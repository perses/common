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

package taskhelper_test

import (
	"context"
	"fmt"

	"github.com/perses/common/async/taskhelper"
)

type cleanupTask struct{}

func (t *cleanupTask) String() string {
	return "cleanup task"
}

func (t *cleanupTask) Execute(_ context.Context, _ context.CancelFunc) error {
	return nil
}

func ExampleNewCron() {
	// run the task every day at 2am. A predefined schedule like "@daily" or "@every 1h30m" is also supported.
	helper, err := taskhelper.NewCron(&cleanupTask{}, "0 2 * * *")
	if err != nil {
		fmt.Println("invalid schedule:", err)
		return
	}
	fmt.Println(helper)

	// the cron library only accepts 5 fields
	_, err = taskhelper.NewCron(&cleanupTask{}, "0 0 0 1 1 *")
	fmt.Println(err != nil)
	// Output:
	// cleanup task
	// true
}
