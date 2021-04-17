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

// Package taskhelper provides struct and methods to help to synchronize different async.Task together and to simply start the async.Task properly
// This package should mainly used through the package app and not directly by the developer.
package taskhelper

import (
	"context"
	"fmt"
	"sync"
	"time"

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

func New(task interface{}) (Helper, error) {
	isSimpleTask := true
	switch task.(type) {
	case async.Task:
		isSimpleTask = false
	case async.SimpleTask:
	// just here as sanity check
	default:
		return nil, fmt.Errorf("task is not a SimpleTask or a Task")
	}
	return &runner{
		interval:     0,
		task:         task,
		isSimpleTask: isSimpleTask,
		done:         make(chan struct{}),
	}, nil
}

// NewCron is returning a Helper that will execute the task periodically.
// The task can be a SimpleTask or a Task. It returns an error if it's something different
func NewCron(task interface{}, interval time.Duration) (Helper, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("interval cannot be negative or equal to 0 when creating a cron")
	}
	isSimpleTask := true
	switch task.(type) {
	case async.Task:
		isSimpleTask = false
	case async.SimpleTask:
	// just here as sanity check
	default:
		return nil, fmt.Errorf("task is not a SimpleTask or a Task")
	}
	return &runner{
		interval:     interval,
		task:         task,
		isSimpleTask: isSimpleTask,
		done:         make(chan struct{}),
	}, nil
}

type runner struct {
	Helper
	// interval is used when the runner is used as a Cron
	interval time.Duration
	// task can be a SimpleTask or a Task
	task         interface{}
	isSimpleTask bool
	done         chan struct{}
}

func (r *runner) Done() <-chan struct{} {
	return r.done
}

func (r *runner) String() string {
	return r.task.(async.SimpleTask).String()
}

func (r *runner) Start(ctx context.Context, cancelFunc context.CancelFunc) (err error) {
	// closing this channel will highlight the caller that the task is done.
	defer close(r.done)
	childCtx := ctx
	if !r.isSimpleTask {
		// childCancelFunc will be used to stop any sub go-routing using the childCtx when the current task is stopped.
		// it's just to be sure that every sub go-routing created by the task will be stopped without stopping the whole application.
		var childCancelFunc context.CancelFunc
		childCtx, childCancelFunc = context.WithCancel(ctx)
		t := r.task.(async.Task)
		// then we have to call the finalise method of the task
		defer func() {
			childCancelFunc()
			if finalErr := t.Finalize(); finalErr != nil {
				if err == nil {
					err = finalErr
				} else {
					logrus.WithError(finalErr).Error("error occurred when calling the method Finalize of the task")
				}
			}
		}()

		// and the initialize method
		if initError := t.Initialize(); initError != nil {
			err = fmt.Errorf("unable to call the initialize method of the task: %w", initError)
			return
		}
	}

	// then run the task
	if executeErr := r.task.(async.SimpleTask).Execute(childCtx, cancelFunc); executeErr != nil {
		err = fmt.Errorf("unable to call the execute method of the task: %w", executeErr)
		return
	}

	// in case the runner has an interval properly set, then we can create a ticker and call periodically the method execute of the task
	return r.tick(childCtx, cancelFunc)
}

func (r *runner) tick(ctx context.Context, cancelFunc context.CancelFunc) error {
	simpleTask := r.task.(async.SimpleTask)
	if r.interval <= 0 {
		return nil
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if executeErr := simpleTask.Execute(ctx, cancelFunc); executeErr != nil {
				return fmt.Errorf("unable to call the execute method of the task %s: %w", simpleTask.String(), executeErr)
			}
		case <-ctx.Done():
			logrus.Debugf("task %s has been canceled", simpleTask.String())
			return nil
		}
	}
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
	waitAll(timeout, helpers)
}

func waitAll(timeout time.Duration, helpers []Helper) {
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
