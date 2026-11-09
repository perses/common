// Copyright The Perses Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package echo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	echoLib "github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLogHook captures the logs of the standard logrus logger, at every level, until the end of the test.
// The logs are not written to the output during the test.
func newLogHook(t *testing.T) *logrustest.Hook {
	t.Helper()
	logger := logrus.StandardLogger()
	origHooks := logger.ReplaceHooks(make(logrus.LevelHooks))
	origLevel := logger.GetLevel()
	origOutput := logger.Out
	logger.SetLevel(logrus.TraceLevel)
	logger.SetOutput(io.Discard)
	t.Cleanup(func() {
		logger.ReplaceHooks(origHooks)
		logger.SetLevel(origLevel)
		logger.SetOutput(origOutput)
	})
	return logrustest.NewLocal(logger)
}

// findLogEntry returns the first entry captured by the hook with the given message.
func findLogEntry(hook *logrustest.Hook, message string) *logrus.Entry {
	for _, entry := range hook.AllEntries() {
		if entry.Message == message {
			return entry
		}
	}
	return nil
}

// counterValue returns the value of the counter with the given name and labels, or 0 if it doesn't exist.
func counterValue(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			matched := 0
			for _, label := range metric.GetLabel() {
				if value, ok := labels[label.GetName()]; ok && value == label.GetValue() {
					matched++
				}
			}
			if matched == len(labels) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

type errorRegister struct{}

func (r *errorRegister) RegisterRoute(e *echoLib.Echo) {
	e.GET("/panic", func(_ *echoLib.Context) error {
		panic("boom")
	})
	e.GET("/panic-http-error", func(_ *echoLib.Context) error {
		panic(echoLib.NewHTTPError(http.StatusServiceUnavailable, "maintenance"))
	})
	// Like a reverse proxy, these handlers modify the path of the request before failing.
	e.GET("/rewrite-then-panic", func(c *echoLib.Context) error {
		c.Request().URL.Path = "/rewritten"
		panic("boom")
	})
	e.GET("/rewrite-then-fail", func(c *echoLib.Context) error {
		c.Request().URL.Path = "/rewritten"
		return errors.New("unexpected failure")
	})
	e.GET("/rewrite-method-then-fail", func(c *echoLib.Context) error {
		c.Request().Method = http.MethodPost
		return errors.New("unexpected failure")
	})
	e.GET("/unexpected", func(_ *echoLib.Context) error {
		return errors.New("unexpected failure")
	})
	e.GET("/fail-after-response", func(c *echoLib.Context) error {
		if err := c.JSON(http.StatusOK, map[string]string{"message": "partial"}); err != nil {
			return err
		}
		return errors.New("unexpected failure")
	})
	// Like a handler calling a datasource, the error is caused by the cancellation of the request when the client went away.
	e.GET("/canceled", func(c *echoLib.Context) error {
		return c.Request().Context().Err()
	})
	// Like a handler aborting its own work, the handler cancels the context of the request, while the client is still there.
	e.GET("/canceled-by-the-server", func(c *echoLib.Context) error {
		ctx, cancel := context.WithCancel(c.Request().Context())
		c.SetRequest(c.Request().WithContext(ctx))
		cancel()
		return ctx.Err()
	})
	e.GET("/unauthorized", func(_ *echoLib.Context) error {
		return echoLib.NewHTTPError(http.StatusUnauthorized, "missing token")
	})
	e.GET("/bad-gateway", func(_ *echoLib.Context) error {
		return echoLib.NewHTTPError(http.StatusBadGateway, "datasource unreachable")
	})
}

// goroutineHeader matches the first line of the stack of a goroutine (like "goroutine 7 [running]:").
var goroutineHeader = regexp.MustCompile(`(?m)^goroutine \d+ \[`)

func TestServer_ErrorHandling(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		route string
		// canceled cancels the request before it is handled, like when the client went away.
		canceled bool
		// middlewares are added after the default ones.
		middlewares  []echoLib.MiddlewareFunc
		expectedCode int
		expectedBody string
		// expectedError is the message of the error logged (if any) with the level expectedLevel.
		expectedError string
		expectedLevel logrus.Level
		// expectedPanic is the error of the panic logged with its stack trace.
		expectedPanic string
	}{
		{
			name:          "panic",
			path:          "/panic",
			route:         "/panic",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "panic recovered while handling the request",
			expectedLevel: logrus.ErrorLevel,
			expectedPanic: "boom",
		},
		{
			// The value of the panic carries an HTTP status, it must be logged as a panic anyway.
			name:          "panic with an HTTP error",
			path:          "/panic-http-error",
			route:         "/panic-http-error",
			expectedCode:  http.StatusServiceUnavailable,
			expectedBody:  `{"message":"maintenance"}`,
			expectedError: "panic recovered while handling the request",
			expectedLevel: logrus.ErrorLevel,
			expectedPanic: echoLib.NewHTTPError(http.StatusServiceUnavailable, "maintenance").Error(),
		},
		{
			// The error log and the access log use the path requested by the client.
			name:          "panic after the path is rewritten",
			path:          "/rewrite-then-panic",
			route:         "/rewrite-then-panic",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "panic recovered while handling the request",
			expectedLevel: logrus.ErrorLevel,
			expectedPanic: "boom",
		},
		{
			name:          "unexpected error after the path is rewritten",
			path:          "/rewrite-then-fail",
			route:         "/rewrite-then-fail",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			// Like a proxy, the handler rewrites the method: the logs and the metrics use the method received.
			name:          "unexpected error after the method is rewritten",
			path:          "/rewrite-method-then-fail",
			route:         "/rewrite-method-then-fail",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			name:          "unexpected error",
			path:          "/unexpected",
			route:         "/unexpected",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			// The middleware ContextTimeout cancels its own context once the request is handled: the client didn't go away.
			name:          "unexpected error with a middleware canceling its context",
			path:          "/unexpected",
			route:         "/unexpected",
			middlewares:   []echoLib.MiddlewareFunc{middleware.ContextTimeout(time.Minute)},
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			// The error is caused by a cancellation, but the client didn't go away: the server canceled its own context.
			name:          "cancellation by the server",
			path:          "/canceled-by-the-server",
			route:         "/canceled-by-the-server",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			// The client went away, but the error is not caused by the cancellation of the request.
			name:          "unexpected error not caused by the client that went away",
			path:          "/unexpected",
			route:         "/unexpected",
			canceled:      true,
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			// The client already received the status of the response, which the error cannot change.
			name:          "unexpected error after the response is sent",
			path:          "/fail-after-response",
			route:         "/fail-after-response",
			expectedCode:  http.StatusOK,
			expectedBody:  `{"message":"partial"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.WarnLevel,
		},
		{
			// The error is caused by the client that went away.
			name:          "unexpected error after the client went away",
			path:          "/canceled",
			route:         "/canceled",
			canceled:      true,
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
			expectedLevel: logrus.DebugLevel,
		},
		{
			name:         "HTTP error",
			path:         "/unauthorized",
			route:        "/unauthorized",
			expectedCode: http.StatusUnauthorized,
			expectedBody: `{"message":"missing token"}`,
		},
		{
			name:         "HTTP error with a 5xx status",
			path:         "/bad-gateway",
			route:        "/bad-gateway",
			expectedCode: http.StatusBadGateway,
			expectedBody: `{"message":"datasource unreachable"}`,
		},
		{
			name:         "route not found",
			path:         "/does-not-exist",
			expectedCode: http.StatusNotFound,
			expectedBody: `{"message":"Not Found"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newLogHook(t)
			registry := prometheus.NewRegistry()
			builder := NewBuilder().
				APIRegistration(&errorRegister{}).
				MetricNamespace("test").
				PrometheusRegisterer(registry)
			for _, mdw := range tt.middlewares {
				builder.Middleware(mdw)
			}
			handler, err := builder.BuildHandler()
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.canceled {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedCode, rec.Code)
			assert.JSONEq(t, tt.expectedBody, rec.Body.String())

			// The access log and the metrics report the status of the response sent.
			var accessLog *logrus.Entry
			for _, entry := range hook.AllEntries() {
				if entry.Data["uri"] == tt.path && entry.Data["status"] != nil {
					accessLog = entry
				}
			}
			require.NotNil(t, accessLog)
			assert.Equal(t, tt.expectedCode, accessLog.Data["status"])
			assert.Equal(t, http.MethodGet, accessLog.Data["method"])
			labels := map[string]string{"code": strconv.Itoa(tt.expectedCode), "method": http.MethodGet}
			if tt.route != "" {
				labels["handler"] = tt.route
			}
			assert.Equal(t, float64(1), counterValue(t, registry, "test_http_requests_total", labels))

			// Only the errors that don't carry an HTTP status are logged.
			if tt.expectedError == "" {
				for _, entry := range hook.AllEntries() {
					assert.NotContains(t, []logrus.Level{logrus.WarnLevel, logrus.ErrorLevel}, entry.Level, entry.Message)
				}
				return
			}
			errorEntry := findLogEntry(hook, tt.expectedError)
			require.NotNil(t, errorEntry)
			assert.Equal(t, tt.expectedLevel, errorEntry.Level)
			assert.Equal(t, http.MethodGet, errorEntry.Data["method"])
			assert.Equal(t, tt.path, errorEntry.Data["uri"])
			if tt.expectedPanic != "" {
				loggedErr, ok := errorEntry.Data[logrus.ErrorKey].(error)
				require.True(t, ok, "the value of the panic must be logged as an error")
				assert.EqualError(t, loggedErr, tt.expectedPanic)
				// Only the stack of the goroutine that panicked is logged.
				stack, ok := errorEntry.Data["stack"].(string)
				require.True(t, ok, "the stack must be logged")
				assert.Len(t, goroutineHeader.FindAllString(stack, -1), 1, stack)
			}
		})
	}
}
