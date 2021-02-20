package async

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func doneAsync() int {
	time.Sleep(1 * time.Second)
	return 1
}

func TestNextImpl_Await(t *testing.T) {
	next := Async(func() interface{} {
		return doneAsync()
	})
	result := next.Await()
	assert.Equal(t, 1, result)
}

func TestNextImpl_AwaitWithContext(t *testing.T) {
	ctx := context.Background()
	next := Async(func() interface{} {
		return doneAsync()
	})
	result := next.AwaitWithContext(ctx)
	assert.Equal(t, 1, result)
}
