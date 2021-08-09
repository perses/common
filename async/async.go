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

// Package async provide different kind of interface and implementation to manipulate a bit more safely the go routing.
//
// First things  provided by this package is an implementation of the pattern Async/Await you can find in Javascript
// It should be used when you need to run multiple asynchronous task and wait for each of them to finish.
// Example:
//  func doneAsync() int {
//		// asynchronous task
//		time.Sleep(3 * time.Second)
//		return 1
//	}
//
//  func synchronousTask() {
//  	next := async.Async(func() interface{} {
//			return doneAsync()
//  	})
//		// do some other stuff
//  	// then wait for the end of the asynchronous task and get back the result
//  	result := next.Await()
//  	// do something with the result
//  }
//
// It is useful to use this implementation when you want to paralyze quickly some short function like paralyzing multiple HTTP request.
// You definitely won't use this implementation if you want to create a cron or a long task. Instead you should implement the interface SimpleTask or Task for doing that.
//
// As a Task is designed to run in a long term, you will have to take care about a context and a cancel function passed in the parameter of the main method Execute.
// Note that the context and the cancel function are shared by every task. It's important to understand that because it means if a task is calling the cancel function,
// then it will close the context for all Task.
// It means also that all task should listen the context in order to react when the context is closed. Once the context is closed, the task should then perform some action to be stopped properly.
// Of course it will depend what you are implementing. Not all task need to do something when the context is closed.
// Also calling the cancel function in the task must done only if your task is critical for your application and so if whatever reason the task doesn't work anymore, then the whole application must be stopped.
// If you are not in this situation, you shouldn't use the cancel function
//
// As you may notice, a Task is an extension of the SimpleTask. In addition of the method Execute (coming from the SimpleTask), a Task will need to implement the method Initialize and Finalize.
// It can be useful when you want to separate the different phases of the lifecycle of a Task.
// The Initialize method is called before running the Task (i.e. before calling the method Execute)
// The Finalize method is called once the context is closed
//
// The Task are designed to work with the app.Runner which will create the root context and the cancel function. Basically you don't have to take care how to run the Task or how to create and share the same context.
//
// Example:
// 1. How to implement a Task that would run periodically
//	type myPeriodicTask struct {
//		SimpleTask
//	}
//	func (t *myPeriodicTask) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
//		// since my task is quite simple and it will be called periodically, I don't need to listen the context. It is handle by the caller. (For more detail, see taskhelper.Start
//		logrus.Info("I'm alive!")
//		return nil
//	}
//	// like that the method Execute of myPeriodicTask will be called periodically every 30 seconds.
//	app.NewRunner().WithCronTasks(30*time.Second, &myPeriodicTask).Start()
//
// 2. How to implement a Task that would run infinitely
//	type myInfiniteTask struct {
//		SimpleTask
//	}
//	func (t *myInfiniteTask) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
//		for {
//			select {
//			case <-ctx.Done():
//				// the context is canceled, I need to stop my task
//				return nil
//			default:
//				// my business code to run
//		}
//		return nil
//	}
//	app.NewRunner().WithTasks(&myInfiniteTask).Start()
package async

import "context"

type Future interface {
	Await() interface{}
	AwaitWithContext(ctx context.Context) interface{}
}

type next struct {
	await func(ctx context.Context) interface{}
}

func (n *next) Await() interface{} {
	return n.await(context.Background())
}

func (n *next) AwaitWithContext(ctx context.Context) interface{} {
	return n.await(ctx)
}

// Async executes the asynchronous function
func Async(f func() interface{}) Future {
	var result interface{}
	// c is going to be used to catch only the signal when the channel is closed.
	c := make(chan struct{})
	go func() {
		defer close(c)
		result = f()
	}()
	return &next{
		await: func(ctx context.Context) interface{} {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c:
				return result
			}
		},
	}
}
