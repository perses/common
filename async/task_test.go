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
	assert.Equal(t, 2, complexTask.counter)
}
