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
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/perses/common/slices"
	"github.com/sirupsen/logrus"
)

type LoggerConfig struct {
	Skipper middleware.Skipper
	// BlackListEndpoint is the list of endpoint that you don't want to log with the info level.
	// An endpoint is matched when the route registered in echo (like /metrics, /debug/pprof/* or /api/v1/projects/:project) contains one of these values.
	// The route is used instead of the URL, as the URL is controlled by the client, who must not be able to change the log level of its requests.
	BlackListEndpoint []string
}

var defaultLoggerConfig = LoggerConfig{
	Skipper: middleware.DefaultSkipper,
	BlackListEndpoint: []string{
		"metrics",
		"favicon",
		"debug/pprof",
	},
}

func Logger() echo.MiddlewareFunc {
	return LoggerWithConfig(defaultLoggerConfig)
}

func LoggerWithConfig(config LoggerConfig) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = defaultLoggerConfig.Skipper
	}
	if len(config.BlackListEndpoint) == 0 {
		config.BlackListEndpoint = defaultLoggerConfig.BlackListEndpoint
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}
			// The request is described before calling the next handler, as it can modify it (like a reverse proxy rewriting the path).
			// Only the escaped path is logged: the query string can contain sensitive data (tokens, OAuth codes, etc.),
			// and the escaped path cannot contain control characters.
			path := c.Request().URL.EscapedPath()
			route := c.Path()
			if err := next(c); err != nil {
				c.Error(err)
			}
			entry := logrus.WithField("method", c.Request().Method).
				WithField("uri", path).
				WithField("status", c.Response().Status)

			if slices.InvertSubContains(config.BlackListEndpoint, route) {
				entry.Debug()
			} else {
				entry.Info()
			}
			return nil
		}
	}
}
