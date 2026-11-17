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

package taskhelper

import (
	"context"
	"fmt"
	"time"

	"github.com/flc1125/go-cron/v4"
	"github.com/perses/common/async"
	"github.com/sirupsen/logrus"
)

type cronRunner struct {
	Helper
	// schedule is used to now when calling the task
	schedule cron.Schedule
	// task can be a SimpleTask or a Task
	task         any
	isSimpleTask bool
	done         chan struct{}
}

func (r *cronRunner) Done() <-chan struct{} {
	return r.done
}

func (r *cronRunner) String() string {
	return r.task.(async.SimpleTask).String()
}

func (r *cronRunner) Start(ctx context.Context, cancelFunc context.CancelFunc) (err error) {
	// closing this channel will highlight the caller that the task is done.
	defer close(r.done)
	childCtx := ctx
	if !r.isSimpleTask {
		// childCancelFunc will be used to stop any sub go-routing using the childCtx when the current task is stopped.
		// it's just to be sure that every sub go-routing created by the task will be stopped without stopping the whole application.
		var childCancelFunc context.CancelFunc
		childCtx, childCancelFunc = context.WithCancel(ctx)
		t := r.task.(async.Task)
		// then we have to call the finalize method of the task
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
	r.cron(childCtx, cancelFunc)
	return nil
}

func (r *cronRunner) cron(ctx context.Context, cancelFunc context.CancelFunc) {
	simpleTask := r.task.(async.SimpleTask)
	now := time.Now()
	next := r.schedule.Next(now)
	for {
		// Next returns the zero time when no date matches within five years.
		// This can happen after a firing even though NewCron rejected schedules
		// that never match at creation time (e.g. a leap-day schedule whose next
		// match is more than five years away because 2100 is not a leap year).
		// Arming a timer with a negative duration would execute the task in a
		// busy loop, so stop the runner instead.
		if next.IsZero() {
			logrus.Errorf("task %s has no next execution date for its cron schedule, stopping the runner", simpleTask.String())
			return
		}
		// After an execution, next is computed from the moment it started, so the timer is armed with the time left until this date.
		// That way, the time spent executing the task doesn't delay the next execution.
		// If the execution took longer than that, the next date has already passed, and the task is executed again right away.
		timer := time.NewTimer(time.Until(next))
		for {
			select {
			case now = <-timer.C:
				// then run the task
				// A failure must not stop the task: it is executed again at the next schedule.
				if executeErr := simpleTask.Execute(ctx, cancelFunc); executeErr != nil && !handleExecuteError(ctx, simpleTask.String(), "schedule", executeErr) {
					return
				}
				next = r.schedule.Next(now)
			case <-ctx.Done():
				logrus.Debugf("task %s has been canceled", simpleTask.String())
				return
			}
			break
		}
	}
}
