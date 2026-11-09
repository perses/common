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
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
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

// Logger returns a middleware that logs every request (its method, escaped path and status) with logrus, once it is handled.
// It must be executed before the middleware Recover, so the panics are logged too.
// The error returned by the next handler is not handled here: it is returned, to be handled once by the central error handler of echo.
// The status logged is the one of the response that the error handler will send.
func Logger() echo.MiddlewareFunc {
	return LoggerWithConfig(defaultLoggerConfig)
}

// LoggerWithConfig returns a Logger middleware (see Logger) with the given configuration.
func LoggerWithConfig(config LoggerConfig) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = defaultLoggerConfig.Skipper
	}
	if len(config.BlackListEndpoint) == 0 {
		config.BlackListEndpoint = defaultLoggerConfig.BlackListEndpoint
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}
			// The request is described before calling the next handler, as it can modify it (like a reverse proxy rewriting the path or the method).
			// Only the escaped path is logged: the query string can contain sensitive data (tokens, OAuth codes, etc.),
			// and the escaped path cannot contain control characters.
			request := loggedRequest{method: c.Request().Method, path: c.Request().URL.EscapedPath()}
			// The request is saved, so the other logs about it (like the one of the central error handler) describe it the same way.
			c.Set(contextKeyLoggedRequest, request)
			route := c.Path()
			// The error is not handled here, it is returned to be handled by the central error handler of echo.
			// The status logged is the one of the response that will be sent, resolved from this error.
			err := next(c)
			_, status := echo.ResolveResponseStatus(c.Response(), err)
			entry := logrus.WithFields(logrus.Fields{
				"method": request.method,
				"uri":    request.path,
				"status": status,
			})

			if slices.InvertSubContains(config.BlackListEndpoint, route) {
				entry.Debug()
			} else {
				entry.Info()
			}
			return err
		}
	}
}

// loggedRequest describes the request as logged by the Logger middleware.
type loggedRequest struct {
	method string
	path   string
}

// contextKeyLoggedRequest is the key of the echo context in which the Logger middleware saves the request it logs.
const contextKeyLoggedRequest = "github.com/perses/common/echo/middleware.loggedRequest"

// RequestMethod returns the method of the request as logged by the Logger middleware, before the next handlers
// possibly rewrote it (like a reverse proxy). Like that, every log about the same request uses the same method.
// It returns the current method of the request when the Logger middleware didn't handle it.
func RequestMethod(c *echo.Context) string {
	if request, ok := c.Get(contextKeyLoggedRequest).(loggedRequest); ok {
		return request.method
	}
	return c.Request().Method
}

// RequestPath returns the escaped path of the request as logged by the Logger middleware, before the next handlers
// possibly rewrote it (like a reverse proxy). Like that, every log about the same request uses the same path.
// It returns the current escaped path of the request when the Logger middleware didn't handle it.
func RequestPath(c *echo.Context) string {
	if request, ok := c.Get(contextKeyLoggedRequest).(loggedRequest); ok {
		return request.path
	}
	return c.Request().URL.EscapedPath()
}
