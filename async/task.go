package async

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SimpleTask is the simple way to define an asynchronous task. It will be used by the runner.
// Depending of what you want, the runner will can only once the method Execute or periodically.
type SimpleTask interface {
	// Implements fmt.Stringer for logging purpose (able to identify the execute or in the logs)
	fmt.Stringer
	// Execute is the method called once or periodically to execute the task
	// ctx is the parent context, you usually use it to listen if the context is done.
	// cancel will be used to cancel the ctx and to propagate the information to others Task that they have to stop since it will close the channel
	// Use it if you are going to implement a task that is critical to your application and if it stops then it should stop your whole application.
	// A good example is the signalListener. You want to stop your application in case a sigterm is sent to the application. So in that particular case, the signalListener will call the cancelFunc
	Execute(ctx context.Context, cancelFunc context.CancelFunc) error
}

// Task is a more complete way to define an asynchronous task.
// Use it when you want a finer control of each steps when the task will run.
type Task interface {
	SimpleTask
	// Initialize is called by the runner before the Task is running
	Initialize() error
	// Finalize is called by the runner when it ends (clean-up, wait children, ...)
	Finalize() error
}

// TaskRunner is an interface that defines a wrapper of the task that would help to execute it.
// Even if this interface is public, you usually don't have to implement it yourself, you just have to implement a Task or a SimpleTask
type TaskRunner interface {
	fmt.Stringer
	Start(ctx context.Context, cancelFunc context.CancelFunc) error
	// Done returns the channel used to wait for the task job to be finalized
	Done() <-chan struct{}
}

func NewTaskRunner(task interface{}) (TaskRunner, error) {
	isSimpleTask := true
	switch task.(type) {
	case Task:
		isSimpleTask = false
	case SimpleTask:
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

// NewCron is returning a TaskRunner that will execute the task periodically
// task can be a SimpleTask or a Task. It returns an error if it's something different
func NewCron(task interface{}, interval time.Duration) (TaskRunner, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("interval cannot be negative or equal to 0 when creating a cron")
	}
	isSimpleTask := true
	switch task.(type) {
	case Task:
		isSimpleTask = false
	case SimpleTask:
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
	TaskRunner
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
	return r.task.(SimpleTask).String()
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
		t := r.task.(Task)
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
			return
		}()

		// and the initialize method
		if initError := t.Initialize(); initError != nil {
			err = fmt.Errorf("unable to call the initialize method of the task: %w", initError)
			return
		}
	}

	// then run the task
	if executeErr := r.task.(SimpleTask).Execute(childCtx, cancelFunc); executeErr != nil {
		err = fmt.Errorf("unable to call the execute method of the task: %w", executeErr)
		return
	}

	// in case the runner has an interval properly set, then we can create a ticker and call periodically the method execute of the task
	return r.tick(childCtx, cancelFunc)
}

func (r *runner) tick(ctx context.Context, cancelFunc context.CancelFunc) error {
	simpleTask := r.task.(SimpleTask)
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

// LaunchRunner is executing in a go-routing the TaskRunner that handles a unique task
func LaunchRunner(ctx context.Context, cancelFunc context.CancelFunc, t TaskRunner) {
	go func() {
		if err := t.Start(ctx, cancelFunc); err != nil {
			logrus.WithError(err).Errorf("'%s' ended in error", t.String())
		}
	}()
}

// JoinAll is waiting for context to be canceled
// A task that is ended and should stop the whole application, must have called the master cancelFunc shared by every TaskRunner which will closed the master context.
func JoinAll(ctx context.Context, timeout time.Duration, taskRunners []TaskRunner) {
	<-ctx.Done()
	waitAll(timeout, taskRunners)
}

func waitAll(timeout time.Duration, taskRunners []TaskRunner) {
	waitGroup := &sync.WaitGroup{}
	// set the number of goroutine to wait
	waitGroup.Add(len(taskRunners))
	for _, runner := range taskRunners {
		go func(r TaskRunner, t time.Duration) {
			defer waitGroup.Done()
			timeoutTicker := time.NewTicker(t)
			defer timeoutTicker.Stop()
			select {
			case <-timeoutTicker.C:
				logrus.Errorf("'%s' took too much time to stop", r.String())
			case <-r.Done():
				logrus.Debugf("'%s' has ended", r.String())
			}
		}(runner, timeout)
	}
	waitGroup.Wait()
}
