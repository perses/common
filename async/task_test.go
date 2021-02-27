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

package async

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type simpleTaskImpl struct {
	SimpleTask
}

func (s *simpleTaskImpl) String() string {
	return "simple task"
}

func (s *simpleTaskImpl) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
	select {
	case <-ctx.Done():
		return nil
	default:
		time.Sleep(10 * time.Second)
		// simulate the whole stop of the application by calling the master cancel func
		cancelFunc()
		return nil
	}
}

type complexTaskImpl struct {
	counter int
	Task
}

func (s *complexTaskImpl) String() string {
	return "complex task"
}

func (s *complexTaskImpl) Initialize() error {
	return nil
}

func (s *complexTaskImpl) Execute(ctx context.Context, _ context.CancelFunc) error {
	select {
	case <-ctx.Done():
		return nil
	default:
		fmt.Printf("'%s' is doing something\n", s.String())
		s.counter++
		return nil
	}
}

func (s *complexTaskImpl) Finalize() error {
	return nil
}

// The goal of this test is:
// * To validate that when the cancelFunc() is called, it is correctly propagated across the different go-routing and properly considered
// * To validate that the JoinAll is effectively waiting for the end of the every given task
func TestJoinAll(t *testing.T) {
	t1, err := NewTaskRunner(&simpleTaskImpl{})
	assert.NoError(t, err)
	complexTask := &complexTaskImpl{}
	t2, err := NewCron(complexTask, 5*time.Second)
	assert.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	// start all runner
	LaunchRunner(ctx, cancel, t1)
	LaunchRunner(ctx, cancel, t2)
	JoinAll(ctx, 30*time.Second, []TaskRunner{t1, t2})
	assert.True(t, complexTask.counter >= 2)
}
