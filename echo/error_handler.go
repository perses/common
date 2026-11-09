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

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/sirupsen/logrus"

	persesMiddleware "github.com/perses/common/echo/middleware"
)

var defaultHTTPErrorHandler = echo.DefaultHTTPErrorHandler(false)

// contextKeyRequestContext is the key of the echo context in which saveRequestContext saves the context of the request.
const contextKeyRequestContext = "github.com/perses/common/echo.requestContext"

// saveRequestContext is a middleware, executed before every other one, that saves the context of the request as received
// by the server. The middlewares and the handlers can replace the context of the request (like the middleware ContextTimeout,
// which cancels its own context once the request is handled), so only this one tells if the client went away.
func saveRequestContext(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		c.Set(contextKeyRequestContext, c.Request().Context())
		return next(c)
	}
}

// requestContext returns the context of the request as received by the server (see saveRequestContext).
func requestContext(c *echo.Context) context.Context {
	if ctx, ok := c.Get(contextKeyRequestContext).(context.Context); ok {
		return ctx
	}
	return c.Request().Context()
}

// handleError is the central error handler of the server, called with the error returned by the middleware chain.
// The panics recovered by the middleware Recover and the errors that don't carry an HTTP status (unexpected errors)
// are logged, as nothing turned them into a response. Then, like the default echo handler, it writes the error as a JSON
// response without exposing the internal errors.
func handleError(c *echo.Context, err error) {
	// A panic is checked first: its value can carry an HTTP status (like panic(echo.NewHTTPError(...))).
	if panicErr, ok := errors.AsType[*middleware.PanicStackError](err); ok {
		requestLogEntry(c).WithError(panicErr.Err).WithField("stack", string(panicErr.Stack)).Error("panic recovered while handling the request")
	} else if echo.StatusCode(err) == 0 {
		requestLogEntry(c).WithError(err).Log(unexpectedErrorLevel(c, err), "unexpected error while handling the request")
	}
	defaultHTTPErrorHandler(c, err)
}

// requestLogEntry returns a log entry describing the request like the access log: with the method and the escaped path
// of the request as received, as the handlers can rewrite them (see persesMiddleware.RequestMethod and RequestPath).
func requestLogEntry(c *echo.Context) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"method": persesMiddleware.RequestMethod(c),
		"uri":    persesMiddleware.RequestPath(c),
	})
}

// unexpectedErrorLevel returns the level of the log of an unexpected error.
// The error is less severe when it is caused by the client that went away, or when the response is already sent.
func unexpectedErrorLevel(c *echo.Context, err error) logrus.Level {
	if errors.Is(err, context.Canceled) && requestContext(c).Err() != nil {
		// The request is canceled when the client closes the connection, and the error is caused by this cancellation.
		return logrus.DebugLevel
	}
	if response, _ := echo.UnwrapResponse(c.Response()); response != nil && response.Committed {
		// The client already received the status of the response, which the error cannot change anymore.
		return logrus.WarnLevel
	}
	return logrus.ErrorLevel
}
