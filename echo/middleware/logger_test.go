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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
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
	e.Use(Logger())
	handler := func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	}
	e.GET("/api/v1/users", handler)
	e.GET("/api/v1/projects/:project", handler)
	e.GET("/metrics", handler)
	// Like a reverse proxy, this handler modifies the path of the request.
	e.GET("/proxy/*", func(c echo.Context) error {
		c.Request().URL.Path = "/rewritten"
		return c.NoContent(http.StatusOK)
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
			name:           "the path requested by the client is logged, even if the handler modifies it",
			target:         "/proxy/api/v1/query",
			expectedStatus: http.StatusOK,
			expectedLevel:  logrus.InfoLevel,
			expectedURI:    "/proxy/api/v1/query",
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
		})
	}
}
