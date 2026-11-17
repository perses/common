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

package app_test

import (
	"context"
	"flag"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/perses/common/app"
)

type nightlyTask struct{}

func (t *nightlyTask) String() string {
	return "nightly task"
}

func (t *nightlyTask) Execute(_ context.Context, _ context.CancelFunc) error {
	return nil
}

type api struct{}

func (a *api) RegisterRoute(e *echo.Echo) {
	e.GET("/api/v1/hello", func(c *echo.Context) error {
		return c.String(200, "hello")
	})
}

// This example is only compiled, because Start blocks until the application is stopped.
func ExampleNewRunner() {
	// register the flags configuring the logs and the HTTP server, then parse them
	app.InitFlag()
	flag.Parse()

	runner := app.NewRunner().
		WithDefaultLogrusBuilder().
		WithDefaultHTTPServer("your_api_name").
		WithTimerTasks(30*time.Second, &nightlyTask{}).
		WithCronTasks("0 2 * * *", &nightlyTask{})
	// register your api
	runner.HTTPServerBuilder().APIRegistration(&api{})
	// start the application
	runner.Start()
}
