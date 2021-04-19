// Copyright 2021 Amadeus s.a.s
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

// Package app is exposing a struct to handle the building and the managing of the different task coming from the package async.
// This should be used in the main package only.
//
// A quite straightforward usage of this package is when you are implementing an HTTP API and want to expose it.
// In that case you can use the following example:
//
//   package main
//   import (
//     "github.com/perses/commun/app"
//   )
//   func main() {
//     // create your api
//     api := newAPI()
//     // then use the app package to start it properly
//     runner := app.NewRunner().WithDefaultHTTPServer("your_api_name")
//     runner.HTTPServerBuilder().APIRegistration(api)
//     // start the application
//     runner.Start()
//   }
//
package app

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/perses/common/async"
	"github.com/perses/common/async/taskhelper"
	"github.com/perses/common/echo"
	"github.com/sirupsen/logrus"
)

var (
	// BuildTime is the date when the application has been built.
	BuildTime string
	// Version is the current version of the application
	Version string
	// Commit is the ID of the commit when the application has been built
	Commit string
	// level of the log for logrus only
	logLevel string
	// includes the calling method as a field in the log
	logMethodTrace bool
	// http address listened
	addr string
)

// mainHeader logs the start time and various build information.
func mainHeader() {
	logrus.Infof("Program started at %s", time.Now().UTC().String())
	logrus.Infof("Build time: %s", BuildTime)
	logrus.Infof("Version: %s", Version)
	logrus.Infof("Commit: %s", Commit)
	logrus.Info("------------")
}

func init() {
	flag.StringVar(&logLevel, "log.level", "info", "log level. Possible value: panic, fatal, error, warning, info, debug, trace")
	flag.BoolVar(&logMethodTrace, "log.method-trace", false, "include the calling method as a field in the log. Can be useful to see immediately where the log comes from")
	flag.StringVar(&addr, "web.listen-address", ":8080", "The address to listen on for HTTP requests, web interface and telemetry.")
}

type cron struct {
	task     interface{}
	duration time.Duration
}

type Runner struct {
	// waitTimeout is the amount of time to wait before killing the application once it received a cancellation order.
	waitTimeout time.Duration
	// cronTasks is the different task that are executed periodically.
	cronTasks []cron
	// tasks is the different task that are executed asynchronously only once time.
	// for each task a async.TaskRunner will be created
	tasks []interface{}
	// helpers is the different helper to execute
	helpers       []taskhelper.Helper
	serverBuilder *echo.Builder
	// banner is just a string (ideally the logo of the project) that would be printed when the runner is started
	// If set, then the main header won't be printed.
	banner string
}

func NewRunner() *Runner {
	return &Runner{
		waitTimeout: time.Second * 30,
	}
}

// SetTimeout is setting the time to wait before killing the application once it received a cancellation order.
func (r *Runner) SetTimeout(timeout time.Duration) *Runner {
	if timeout > 0 {
		r.waitTimeout = timeout
	}
	return r
}

// SetBanner is setting  a string (ideally the logo of the project) that would be printed when the runner is started
// Additionally you can also print the Version, the BuildTime and the Commit.
// You just have to add '%s' in your banner where you want to print each information (one '%s' per additional information).
// If set, then the main header won't be printed. The main header is printing the Version, the Commit and the BuildTime.
func (r *Runner) SetBanner(banner string) *Runner {
	r.banner = banner
	return r
}

// WithTasks is the way to add different task that will be executed asynchronously. If a task ended with no error, it won't necessarily stopped the whole application.
// It will mainly depend of how the task is managing the context passed in parameter.
func (r *Runner) WithTasks(t ...interface{}) *Runner {
	r.tasks = append(r.tasks, t...)
	return r
}

// WithCronTasks is the way to add different task that will be executed periodically at the frequency defined with the duration.
func (r *Runner) WithCronTasks(duration time.Duration, t ...interface{}) *Runner {
	for _, ts := range t {
		r.cronTasks = append(r.cronTasks, cron{
			task:     ts,
			duration: duration,
		})
	}
	return r
}

func (r *Runner) WithTaskHelpers(t ...taskhelper.Helper) *Runner {
	r.helpers = append(r.helpers, t...)
	return r
}

func (r *Runner) WithDefaultHTTPServer(metricNamespace string) *Runner {
	r.serverBuilder = echo.NewBuilder(addr).APIRegistration(echo.NewMetricsAPI(true)).MetricNamespace(metricNamespace)
	return r
}

func (r *Runner) HTTPServerBuilder() *echo.Builder {
	if r.serverBuilder == nil {
		r.serverBuilder = echo.NewBuilder(addr)
	}
	return r.serverBuilder
}

// Start will start the application. It is a blocking method and will give back the end once every tasks handled are done.
func (r *Runner) Start() {
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("unable to set the log.level")
	}
	logrus.SetLevel(level)
	logrus.SetReportCaller(logMethodTrace)
	logrus.SetFormatter(&logrus.TextFormatter{
		// Useful when you have a TTY attached.
		// Issue explained here when this field is set to false by default:
		// https://github.com/sirupsen/logrus/issues/896
		FullTimestamp: true,
	})
	// log the server infos or print the banner
	r.printBannerOrMainHeader()
	// start to handle the different task

	// create the http server if defined
	if r.serverBuilder != nil {
		if serverTask, err := r.serverBuilder.Build(); err != nil {
			logrus.WithError(err).Fatal("An error occurred while creating the server task")
		} else {
			r.tasks = append(r.tasks, serverTask)
		}
	}

	// create the signal listener and add it to all others tasks
	signalsListener := async.NewSignalListener(syscall.SIGINT, syscall.SIGTERM)
	r.tasks = append(r.tasks, signalsListener)

	for _, c := range r.cronTasks {
		if taskHelper, err := taskhelper.NewCron(c.task, c.duration); err != nil {
			logrus.WithError(err).Fatalf("unable to create a taskRunner to handle a cron set")
		} else {
			r.helpers = append(r.helpers, taskHelper)
		}
	}

	for _, task := range r.tasks {
		if taskHelper, err := taskhelper.New(task); err != nil {
			logrus.WithError(err).Fatalf("unable to create a taskRunner to handle a task set")
		} else {
			r.helpers = append(r.helpers, taskHelper)
		}
	}

	// create the master context that must be shared by every task
	ctx, cancel := context.WithCancel(context.Background())
	// in any case call the cancel method to release any possible resources.
	defer cancel()
	// launch every runners
	for _, runner := range r.helpers {
		taskhelper.Run(ctx, cancel, runner)
	}
	// Wait for context to be canceled or tasks to be ended and wait for graceful stop
	taskhelper.JoinAll(ctx, r.waitTimeout, r.helpers)
}

func (r *Runner) printBannerOrMainHeader() {
	if len(r.banner) == 0 {
		mainHeader()
		return
	}
	var params []string
	nbParam := strings.Count(r.banner, "%s")
	if nbParam >= 1 {
		params = append(params, Version)
	}
	if nbParam >= 2 {
		params = append(params, Commit)
	}
	if nbParam >= 3 {
		params = append(params, BuildTime)
	}
	fmt.Printf(r.banner, params)
}
