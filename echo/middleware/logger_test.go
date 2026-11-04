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
	previousLevel := logrus.GetLevel()
	logrus.SetLevel(logrus.DebugLevel)
	hook := logrusTest.NewGlobal()
	defer func() {
		logrus.SetLevel(previousLevel)
		logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	}()

	e := echo.New()
	e.Use(Logger())
	handler := func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	}
	e.GET("/api/v1/users", handler)
	e.GET("/metrics", handler)

	tests := []struct {
		name          string
		target        string
		expectedLevel logrus.Level
		expectedURI   string
	}{
		{
			name:          "regular endpoint",
			target:        "/api/v1/users",
			expectedLevel: logrus.InfoLevel,
			expectedURI:   "/api/v1/users",
		},
		{
			name:          "blacklisted endpoint",
			target:        "/metrics",
			expectedLevel: logrus.DebugLevel,
			expectedURI:   "/metrics",
		},
		{
			name:          "the query string cannot change the log level and is not logged",
			target:        "/api/v1/users?token=secret&x=metrics",
			expectedLevel: logrus.InfoLevel,
			expectedURI:   "/api/v1/users",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook.Reset()
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.target, nil))
			require.Equal(t, http.StatusOK, rec.Code)

			entry := hook.LastEntry()
			require.NotNil(t, entry)
			assert.Equal(t, tt.expectedLevel, entry.Level)
			assert.Equal(t, tt.expectedURI, entry.Data["uri"])
		})
	}
}
