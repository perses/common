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
)

// SimpleTask is the simple way to define an asynchronous task.
// The execution of the task is handled by the package async/taskhelper.
type SimpleTask interface {
	// Stringer is implemented for logging purpose (able to identify the execute or in the logs)
	fmt.Stringer
	// Execute is the method called once or periodically to execute the task.
	// ctx is the parent context, you usually use it to listen if the context is done.
	// cancel will be used to cancel the ctx and to propagate the information to others Task that they have to stop since it will close the channel.
	// Use it if you are going to implement a task that is critical to your application and if it stops then it should stop your whole application.
	// A good example is the signalListener. You want to stop your application in case a sigterm is sent to the application. So in that particular case, the signalListener will call the cancelFunc.
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
