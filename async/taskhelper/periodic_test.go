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
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/perses/common/async"
	"github.com/sirupsen/logrus"
	logrusTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingTask fails the first `failures` times it is executed.
type failingTask struct {
	async.SimpleTask
	failures int32
	calls    atomic.Int32
}

func (f *failingTask) String() string {
	return "failing task"
}

func (f *failingTask) Execute(_ context.Context, _ context.CancelFunc) error {
	if f.calls.Add(1) <= f.failures {
		return errors.New("temporary failure")
	}
	return nil
}

func runUntil(t *testing.T, helper Helper, condition func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Run(ctx, cancel, helper)
	assert.Eventually(t, condition, 10*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-helper.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the task has not been stopped")
	}
}

func TestTick_ExecutedAgainAfterAnError(t *testing.T) {
	task := &failingTask{failures: 2}
	helper, err := NewTick(task, 10*time.Millisecond)
	require.NoError(t, err)
	runUntil(t, helper, func() bool {
		return task.calls.Load() >= 4
	})
}

func TestCron_ExecutedAgainAfterAnError(t *testing.T) {
	task := &failingTask{failures: 1}
	helper, err := NewCron(task, "@every 1s")
	require.NoError(t, err)
	runUntil(t, helper, func() bool {
		return task.calls.Load() >= 2
	})
}

// slowTask takes `duration` to execute, and records when each execution starts.
type slowTask struct {
	async.SimpleTask
	duration time.Duration
	mutex    sync.Mutex
	starts   []time.Time
}

func (s *slowTask) String() string {
	return "slow task"
}

func (s *slowTask) Execute(ctx context.Context, _ context.CancelFunc) error {
	s.mutex.Lock()
	s.starts = append(s.starts, time.Now())
	s.mutex.Unlock()
	select {
	case <-time.After(s.duration):
	case <-ctx.Done():
	}
	return nil
}

func (s *slowTask) executionStarts() []time.Time {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return slices.Clone(s.starts)
}

func TestCron_ExecutionTimeDoesNotDelayTheNextExecution(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		// maxGap is the maximum time expected between the start of two executions.
		// Without taking the execution time into account, the gap would be about the execution time plus one second.
		maxGap time.Duration
	}{
		{
			// The task is executed every second.
			name:     "execution shorter than the schedule",
			duration: 700 * time.Millisecond,
			maxGap:   1300 * time.Millisecond,
		},
		{
			// The next date has already passed when the execution ends, so the task is executed again right away.
			name:     "execution longer than the schedule",
			duration: 1500 * time.Millisecond,
			maxGap:   1800 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &slowTask{duration: tt.duration}
			helper, err := NewCron(task, "@every 1s")
			require.NoError(t, err)
			runUntil(t, helper, func() bool {
				return len(task.executionStarts()) >= 3
			})
			starts := task.executionStarts()
			for i := 1; i < len(starts); i++ {
				assert.Less(t, starts[i].Sub(starts[i-1]), tt.maxGap, "execution %d started too late", i+1)
			}
		})
	}
}

func TestNewCron_Schedule(t *testing.T) {
	testSuite := []struct {
		name     string
		schedule string
		wantErr  string
	}{
		{name: "standard", schedule: "*/5 * * * *"},
		{name: "descriptor", schedule: "@daily"},
		{name: "interval", schedule: "@every 1s"},
		{name: "leap day", schedule: "0 0 29 2 *"},
		{name: "never matches", schedule: "0 0 31 2 *", wantErr: `the cron schedule "0 0 31 2 *" never matches any date`},
	}
	for _, test := range testSuite {
		t.Run(test.name, func(t *testing.T) {
			helper, err := NewCron(&failingTask{}, test.schedule)
			if test.wantErr != "" {
				assert.ErrorContains(t, err, test.wantErr)
				assert.Nil(t, helper)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, helper)
		})
	}
}

func TestNew_ReturnTheExecuteError(t *testing.T) {
	task := &failingTask{failures: 1}
	helper, err := New(task)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.Error(t, helper.Start(ctx, cancel))
	assert.Equal(t, int32(1), task.calls.Load())
}

// cancelingTask stops the application during its n-th execution, and returns the error of the canceled context.
type cancelingTask struct {
	async.SimpleTask
	cancelAt int32
	calls    atomic.Int32
}

func (c *cancelingTask) String() string {
	return "canceling task"
}

func (c *cancelingTask) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
	if c.calls.Add(1) < c.cancelAt {
		return nil
	}
	cancelFunc()
	return ctx.Err()
}

func TestPeriodicTask_StoppedWhenTheErrorIsCausedByTheCancellation(t *testing.T) {
	origHooks := logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	t.Cleanup(func() {
		logrus.StandardLogger().ReplaceHooks(origHooks)
	})
	hook := logrusTest.NewLocal(logrus.StandardLogger())

	tests := []struct {
		name      string
		cancelAt  int32
		newHelper func(task async.SimpleTask) (Helper, error)
	}{
		{
			name:     "tick, first execution",
			cancelAt: 1,
			newHelper: func(task async.SimpleTask) (Helper, error) {
				return NewTick(task, 10*time.Millisecond)
			},
		},
		{
			name:     "tick, next execution",
			cancelAt: 2,
			newHelper: func(task async.SimpleTask) (Helper, error) {
				return NewTick(task, 10*time.Millisecond)
			},
		},
		{
			name:     "cron",
			cancelAt: 1,
			newHelper: func(task async.SimpleTask) (Helper, error) {
				return NewCron(task, "@every 1s")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook.Reset()
			task := &cancelingTask{cancelAt: tt.cancelAt}
			helper, err := tt.newHelper(task)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			Run(ctx, cancel, helper)
			select {
			case <-helper.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the task has not been stopped")
			}
			// The task is not executed again, and the error caused by the cancellation is not reported as a failure.
			assert.Equal(t, tt.cancelAt, task.calls.Load())
			for _, entry := range hook.AllEntries() {
				assert.NotEqual(t, logrus.ErrorLevel, entry.Level, entry.Message)
			}
		})
	}
}
