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

package async

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	ErrorThrown = fmt.Errorf("super error")
)

func doneAsync() (int, error) {
	time.Sleep(1 * time.Second)
	return 1, nil
}

func doneWithErrorAsync() (int, error) {
	time.Sleep(1 * time.Second)
	return 2, ErrorThrown
}

func TestNextImpl_Await(t *testing.T) {
	n := Async(doneAsync)
	result, err := n.Await()
	assert.Equal(t, 1, result)
	assert.Equal(t, nil, err)
}

func TestNextImpl_AwaitWithContext(t *testing.T) {
	ctx := context.Background()
	n := Async(doneAsync)
	result, err := n.AwaitWithContext(ctx)
	assert.Equal(t, 1, result)
	assert.Equal(t, nil, err)
}

func TestNextImpl_AwaitWithError(t *testing.T) {
	n := Async(doneWithErrorAsync)
	result, err := n.Await()
	assert.Equal(t, 2, result)
	assert.Equal(t, ErrorThrown, err)
}

func TestNextImpl_AwaitWithErrorAndContext(t *testing.T) {
	ctx := context.Background()
	n := Async(doneWithErrorAsync)
	result, err := n.AwaitWithContext(ctx)
	assert.Equal(t, 2, result)
	assert.Equal(t, ErrorThrown, err)
}
