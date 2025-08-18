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
	task         interface{}
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
	return r.cron(childCtx, cancelFunc)
}

func (r *cronRunner) cron(ctx context.Context, cancelFunc context.CancelFunc) error {
	simpleTask := r.task.(async.SimpleTask)
	now := time.Now()
	next := r.schedule.Next(now)
	for {
		timer := time.NewTimer(next.Sub(now))
		for {
			select {
			case now = <-timer.C:
				// then run the task
				if executeErr := r.task.(async.SimpleTask).Execute(ctx, cancelFunc); executeErr != nil {
					return fmt.Errorf("unable to call the execute method of the task: %w", executeErr)
				}
				next = r.schedule.Next(now)
			case <-ctx.Done():
				logrus.Debugf("task %s has been canceled", simpleTask.String())
				return nil
			}
			break
		}
	}
}
