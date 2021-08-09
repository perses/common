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
	// BlackListEndpoint is the list of endpoint that you don't want to log with the info level
	BlackListEndpoint []string
}

var defaultLoggerConfig = LoggerConfig{
	Skipper: middleware.DefaultSkipper,
	BlackListEndpoint: []string{
		"metrics",
		"favicon",
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
			if err := next(c); err != nil {
				c.Error(err)
			}
			entry := logrus.WithField("method", c.Request().Method).
				WithField("uri", c.Request().RequestURI).
				WithField("status", c.Response().Status)

			if slices.InvertSubContains(config.BlackListEndpoint, c.Request().RequestURI) {
				entry.Debug()
			} else {
				entry.Info()
			}
			return nil
		}
	}
}
