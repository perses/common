// Package async provide an implementation of the pattern Async/Await you can find in Javascript
// It should be used when you need to run multiple asynchronous task and wait for each of them to finish.
// Example:
//  func doneAsync() int {
//		// asynchronous task
//		time.Sleep(3 * time.Second)
//		return 1
//	}
//
//  func synchronousTask() {
//  	next := async.Exec(func() interface{} {
//			return doneAsync()
//  	})
//		// do some other stuff
//  	// then wait for the end of the asynchronous task and get back the result
//  	result := next.Await()
//  	// do something with the result
//  }
package async

import "context"

type next struct {
	await func(ctx context.Context) interface{}
}

func (n next) Await() interface{} {
	return n.await(context.Background())
}

func (n next) AwaitWithContext(ctx context.Context) interface{} {
	return n.await(ctx)
}

// Exec executes the async function
func Async(f func() interface{}) next {
	var result interface{}
	// c is going to be used to catch only the signal when the channel is closed.
	c := make(chan struct{})
	go func() {
		defer close(c)
		result = f()
	}()
	return next{
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
