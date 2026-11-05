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
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	echoLib "github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLogHook captures the logs of the standard logrus logger until the end of the test.
func newLogHook(t *testing.T) *logrustest.Hook {
	t.Helper()
	origHooks := logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	t.Cleanup(func() {
		logrus.StandardLogger().ReplaceHooks(origHooks)
	})
	return logrustest.NewLocal(logrus.StandardLogger())
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
	e.GET("/unexpected", func(_ *echoLib.Context) error {
		return errors.New("unexpected failure")
	})
	e.GET("/unauthorized", func(_ *echoLib.Context) error {
		return echoLib.NewHTTPError(http.StatusUnauthorized, "missing token")
	})
	e.GET("/bad-gateway", func(_ *echoLib.Context) error {
		return echoLib.NewHTTPError(http.StatusBadGateway, "datasource unreachable")
	})
}

func TestServer_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		route         string
		expectedCode  int
		expectedBody  string
		expectedError string
	}{
		{
			name:          "panic",
			path:          "/panic",
			route:         "/panic",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "panic recovered while handling the request",
		},
		{
			name:          "unexpected error",
			path:          "/unexpected",
			route:         "/unexpected",
			expectedCode:  http.StatusInternalServerError,
			expectedBody:  `{"message":"Internal Server Error"}`,
			expectedError: "unexpected error while handling the request",
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
			handler, err := NewBuilder().
				APIRegistration(&errorRegister{}).
				MetricNamespace("test").
				PrometheusRegisterer(registry).
				BuildHandler()
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

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
			labels := map[string]string{"code": strconv.Itoa(tt.expectedCode), "method": http.MethodGet}
			if tt.route != "" {
				labels["handler"] = tt.route
			}
			assert.Equal(t, float64(1), counterValue(t, registry, "test_http_requests_total", labels))

			// Only the errors that don't carry an HTTP status are logged as errors.
			errorEntry := findLogEntry(hook, tt.expectedError)
			if tt.expectedError == "" {
				for _, entry := range hook.AllEntries() {
					assert.NotEqual(t, logrus.ErrorLevel, entry.Level, entry.Message)
				}
				return
			}
			require.NotNil(t, errorEntry)
			assert.Equal(t, logrus.ErrorLevel, errorEntry.Level)
			assert.Equal(t, http.MethodGet, errorEntry.Data["method"])
			assert.Equal(t, tt.path, errorEntry.Data["uri"])
			if tt.name == "panic" {
				assert.EqualError(t, errorEntry.Data[logrus.ErrorKey].(error), "boom")
				assert.Contains(t, errorEntry.Data["stack"], "goroutine")
			}
		})
	}
}
