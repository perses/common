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

package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	echoMiddleware "github.com/labstack/echo/v5/middleware"
	"github.com/sirupsen/logrus"
	logrusTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) {
	logger := logrus.StandardLogger()
	origHooks := logger.ReplaceHooks(make(logrus.LevelHooks))
	origLevel := logger.GetLevel()
	origOutput := logger.Out
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(io.Discard)
	t.Cleanup(func() {
		logger.ReplaceHooks(origHooks)
		logger.SetLevel(origLevel)
		logger.SetOutput(origOutput)
	})
	hook := logrusTest.NewLocal(logger)

	e := echo.New()
	// Like the default middlewares, the logger is executed before the middleware Recover, so it logs the panics too.
	e.Use(Logger(), echoMiddleware.Recover())
	handler := func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	}
	e.GET("/api/v1/users", handler)
	e.GET("/api/v1/projects/:project", handler)
	e.GET("/metrics", handler)
	// Like a reverse proxy, this handler modifies the path and the method of the request.
	e.GET("/proxy/*", func(c *echo.Context) error {
		c.Request().URL.Path = "/rewritten"
		c.Request().Method = http.MethodPost
		return c.NoContent(http.StatusOK)
	})
	e.GET("/error", func(*echo.Context) error {
		return errors.New("unexpected failure")
	})
	e.GET("/forbidden", func(*echo.Context) error {
		return echo.ErrForbidden
	})
	e.GET("/panic", func(*echo.Context) error {
		panic("boom")
	})

	tests := []struct {
		name           string
		target         string
		expectedStatus int
		expectedLevel  logrus.Level
		expectedURI    string
	}{
		{
			name:           "regular endpoint",
			target:         "/api/v1/users",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/api/v1/users",
		},
		{
			name:           "blacklisted endpoint",
			target:         "/metrics",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.DebugLevel,
			expectedURI:    "/metrics",
		},
		{
			name:           "the query string cannot change the log level and is not logged",
			target:         "/api/v1/users?token=secret&x=metrics",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/api/v1/users",
		},
		{
			name:           "a path parameter cannot change the log level",
			target:         "/api/v1/projects/metrics",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/api/v1/projects/metrics",
		},
		{
			name:           "an unknown endpoint cannot change the log level",
			target:         "/metrics-hidden",
			expectedStatus: http.StatusNotFound,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/metrics-hidden",
		},
		{
			name:           "the path and the method requested by the client are logged, even if the handler modifies them",
			target:         "/proxy/api/v1/query",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/proxy/api/v1/query",
		},
		{
			name:           "the status of an unexpected error is the one sent by the error handler",
			target:         "/error",
			expectedStatus: http.StatusInternalServerError,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/error",
		},
		{
			name:           "the status of an HTTP error is the one carried by the error",
			target:         "/forbidden",
			expectedStatus: http.StatusForbidden,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/forbidden",
		},
		{
			name:           "a panic is logged",
			target:         "/panic",
			expectedStatus: http.StatusInternalServerError,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/panic",
		},
		{
			name:           "the control characters are escaped",
			target:         "/api/v1/projects/a%0Ab",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/api/v1/projects/a%0Ab",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook.Reset()
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.target, nil))
			require.Equal(t, tt.expectedStatus, rec.Code)

			entry := hook.LastEntry()
			require.NotNil(t, entry)
			assert.Equal(t, tt.expectedLevel, entry.Level)
			assert.Equal(t, tt.expectedURI, entry.Data["uri"])
			assert.Equal(t, http.MethodGet, entry.Data["method"])
			assert.Equal(t, tt.expectedStatus, entry.Data["status"])
		})
	}
}

func TestRequestMethodAndPath(t *testing.T) {
	tests := []struct {
		name           string
		middleware     []echo.MiddlewareFunc
		expectedMethod string
		expectedPath   string
	}{
		{
			name:           "the method and the path logged by the logger are returned, even if the handler modifies them",
			middleware:     []echo.MiddlewareFunc{LoggerWithConfig(LoggerConfig{})},
			expectedMethod: http.MethodGet,
			expectedPath:   "/proxy/api%2Fv1",
		},
		{
			name:           "the current method and path are returned when the logger doesn't handle the request",
			middleware:     []echo.MiddlewareFunc{LoggerWithConfig(LoggerConfig{Skipper: func(*echo.Context) bool { return true }})},
			expectedMethod: http.MethodPost,
			expectedPath:   "/rewritten",
		},
		{
			name:           "the current method and path are returned without the logger",
			expectedMethod: http.MethodPost,
			expectedPath:   "/rewritten",
		},
	}
	origOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	t.Cleanup(func() {
		logrus.SetOutput(origOutput)
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var method, path string
			e := echo.New()
			e.Use(tt.middleware...)
			// Like a reverse proxy, this handler modifies the path and the method of the request.
			e.GET("/proxy/*", func(c *echo.Context) error {
				c.Request().URL.Path = "/rewritten"
				c.Request().Method = http.MethodPost
				method = RequestMethod(c)
				path = RequestPath(c)
				return c.NoContent(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/proxy/api%2Fv1", nil))
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.expectedMethod, method)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}
