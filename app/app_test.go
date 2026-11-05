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

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/perses/common/async"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// stopperTask stops the whole application and returns the given error.
type stopperTask struct {
	async.SimpleTask
	delay time.Duration
	err   error
}

func (s *stopperTask) String() string {
	return "stopper task"
}

func (s *stopperTask) Execute(_ context.Context, cancelFunc context.CancelFunc) error {
	time.Sleep(s.delay)
	cancelFunc()
	return s.err
}

// failingTask fails without stopping the application.
type failingTask struct {
	async.SimpleTask
}

func (f *failingTask) String() string {
	return "failing task"
}

func (f *failingTask) Execute(_ context.Context, _ context.CancelFunc) error {
	return errors.New("non critical failure")
}

func TestRunner_ExitCode(t *testing.T) {
	tests := []struct {
		name         string
		tasks        []any
		expectedExit bool
	}{
		{
			name:         "stopped by a task that failed",
			tasks:        []any{&stopperTask{err: errors.New("unable to listen")}},
			expectedExit: true,
		},
		{
			name:  "stopped by a task that succeeded",
			tasks: []any{&stopperTask{}},
		},
		{
			name:  "a task failed without stopping the application",
			tasks: []any{&failingTask{}, &stopperTask{delay: 100 * time.Millisecond}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := -1
			logger := logrus.StandardLogger()
			previousExitFunc := logger.ExitFunc
			logger.ExitFunc = func(code int) {
				exitCode = code
			}
			defer func() {
				logger.ExitFunc = previousExitFunc
			}()

			NewRunner().SetTimeout(5 * time.Second).WithTasks(tt.tasks...).Start()

			if tt.expectedExit {
				assert.Equal(t, 1, exitCode)
			} else {
				assert.Equal(t, -1, exitCode)
			}
		})
	}
}
