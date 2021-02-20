package async

import (
	"context"
	"fmt"
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

// Runner is interface to use if you want to run a Task/SimpleTask
type Runner interface {
	Start(ctx context.Context, cancelFunc context.CancelFunc) error
}

// NewCron is returning a Runner that will execute the task periodically
// task can be a SimpleTask or a Task. It returns an error if it's something different
func NewCron(task interface{}, interval time.Duration) (Runner, error) {
	isSimpleTask := true
	switch _ := task.(type) {
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
	}, nil
}

type runner struct {
	// interval is used when the runner is used as a Cron
	interval time.Duration
	// task can be a SimpleTask or a Task
	task         interface{}
	isSimpleTask bool
}

func (r *runner) Start(ctx context.Context, cancelFunc context.CancelFunc) (err error) {
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
