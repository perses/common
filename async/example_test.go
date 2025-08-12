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

package async_test

import (
	"fmt"

	"github.com/perses/common/async"
)

func fn(s string, err error) func() (string, error) {
	return func() (string, error) {
		return s, err
	}
}

func ExampleAsync() {
	next1 := async.Async(fn("success", nil))
	next2 := async.Async(fn("", fmt.Errorf("error")))

	result1, err1 := next1.Await()
	result2, err2 := next2.Await()
	fmt.Println("async1:", result1, err1, "async2:", result2, err2)
	// Output: async1: success <nil> async2:  error
}
