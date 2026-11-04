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
