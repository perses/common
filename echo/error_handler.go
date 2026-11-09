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

// handleError is the central error handler of the server, called with the error returned by the middleware chain.
// The panics recovered by the middleware Recover and the errors that don't carry an HTTP status (unexpected errors)
// are logged, as nothing turned them into a response. Then, like the default echo handler, it writes the error as a JSON
// response without exposing the internal errors.
func handleError(c *echo.Context, err error) {
	// The path logged by the access logger is used, as the handlers can rewrite the path of the request.
	entry := logrus.WithField("method", c.Request().Method).WithField("uri", persesMiddleware.RequestPath(c))
	// A panic is checked first: its value can carry an HTTP status (like panic(echo.NewHTTPError(...))).
	if panicErr, ok := errors.AsType[*middleware.PanicStackError](err); ok {
		entry.WithError(panicErr.Err).WithField("stack", string(panicErr.Stack)).Error("panic recovered while handling the request")
	} else if echo.StatusCode(err) == 0 {
		entry.WithError(err).Log(unexpectedErrorLevel(c), "unexpected error while handling the request")
	}
	defaultHTTPErrorHandler(c, err)
}

// unexpectedErrorLevel returns the level of the log of an unexpected error.
// The error is less severe when the client went away, or when the response is already sent.
func unexpectedErrorLevel(c *echo.Context) logrus.Level {
	if errors.Is(c.Request().Context().Err(), context.Canceled) {
		// The request is canceled when the client closes the connection, so the error is likely caused by this cancellation.
		return logrus.DebugLevel
	}
	if response, _ := echo.UnwrapResponse(c.Response()); response != nil && response.Committed {
		// The client already received the status of the response, which the error cannot change anymore.
		return logrus.WarnLevel
	}
	return logrus.ErrorLevel
}
