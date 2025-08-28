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

// Package taskhelper provides struct and methods to help to synchronize different async.Task together and to simply start the async.Task properly
// This package should mainly used through the package app and not directly by the developer.
package taskhelper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/flc1125/go-cron/v4"
	"github.com/perses/common/async"
	"github.com/sirupsen/logrus"
)

// Helper is an interface that defines a wrapper of the task that would help to execute it.
// Even if this interface is public, you usually don't have to implement it yourself, you just have to implement a Task or a SimpleTask
type Helper interface {
	fmt.Stringer
	Start(ctx context.Context, cancelFunc context.CancelFunc) error
	// Done returns the channel used to wait for the task job to be finalized
	Done() <-chan struct{}
}

func New(task any) (Helper, error) {
	isSimpleTask, err := isSimpleTask(task)
	if err != nil {
		return nil, err
	}
	return &runner{
		interval:     0,
		task:         task,
		isSimpleTask: isSimpleTask,
		done:         make(chan struct{}),
	}, nil
}

// NewTick is returning a Helper that will execute the task periodically.
// The task can be a SimpleTask or a Task. It returns an error if it's something different
func NewTick(task any, interval time.Duration) (Helper, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("interval cannot be negative or equal to 0 when creating a cron")
	}
	isSimpleTask, err := isSimpleTask(task)
	if err != nil {
		return nil, err
	}
	return &runner{
		interval:     interval,
		task:         task,
		isSimpleTask: isSimpleTask,
		done:         make(chan struct{}),
	}, nil
}

// NewCron is returning a Helper that will execute the task according to the schedule.
// cronSchedule is following the standard syntax described here: https://en.wikipedia.org/wiki/Cron.
// It also supports the predefined scheduling definitions:
// - @yearly (or @annually) | Run once a year, midnight, Jan. 1st        | 0 0 0 1 1 *
// - @monthly               | Run once a month, midnight, first of month | 0 0 0 1 * *
// - @weekly                | Run once a week, midnight between Sat/Sun  | 0 0 0 * * 0
// - @daily (or @midnight)  | Run once a day, midnight                   | 0 0 0 * * *
// - @hourly                | Run once an hour, beginning of hour        | 0 0 * * * *
//
// We are directly relying on what the library https://pkg.go.dev/github.com/robfig/cron is supporting.
func NewCron(task any, cronSchedule string) (Helper, error) {
	sch, err := cron.ParseStandard(cronSchedule)
	if err != nil {
		return nil, err
	}
	isSimpleTask, err := isSimpleTask(task)
	if err != nil {
		return nil, err
	}
	return &cronRunner{
		schedule:     sch,
		task:         task,
		isSimpleTask: isSimpleTask,
		done:         make(chan struct{}),
	}, nil
}

// Run is executing in a go-routing the Helper that handles a unique task
func Run(ctx context.Context, cancelFunc context.CancelFunc, t Helper) {
	go func() {
		if err := t.Start(ctx, cancelFunc); err != nil {
			logrus.WithError(err).Errorf("'%s' ended in error", t.String())
		}
	}()
}

// JoinAll is waiting for context to be canceled.
// A task that is ended and should stop the whole application, must have called the master cancelFunc shared by every TaskRunner which will closed the master context.
func JoinAll(ctx context.Context, timeout time.Duration, helpers []Helper) {
	<-ctx.Done()
	WaitAll(timeout, helpers)
}

// WaitAll is waiting for all the helpers to be done or for the timeout to be reached.
func WaitAll(timeout time.Duration, helpers []Helper) {
	waitGroup := &sync.WaitGroup{}
	// set the number of goroutine to wait
	waitGroup.Add(len(helpers))
	for _, helper := range helpers {
		go func(r Helper, t time.Duration) {
			defer waitGroup.Done()
			timeoutTicker := time.NewTicker(t)
			defer timeoutTicker.Stop()
			select {
			case <-timeoutTicker.C:
				logrus.Errorf("'%s' took too much time to stop", r.String())
			case <-r.Done():
				logrus.Debugf("'%s' has ended", r.String())
			}
		}(helper, timeout)
	}
	waitGroup.Wait()
}

func isSimpleTask(task any) (bool, error) {
	result := true
	switch task.(type) {
	case async.Task:
		result = false
	case async.SimpleTask:
	// just here as sanity check
	default:
		return false, fmt.Errorf("task is not a SimpleTask or a Task")
	}
	return result, nil
}
