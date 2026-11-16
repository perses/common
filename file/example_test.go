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

package file_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/perses/common/file"
)

func ExampleWatch() {
	dir, err := os.MkdirTemp("", "watch")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	filename := filepath.Join(dir, "test.txt")

	changed := make(chan struct{}, 1)
	err = file.Watch(filename, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	if err = os.WriteFile(filename, []byte("hello"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	<-changed
	fmt.Println("File created or changed")
	// Output: File created or changed
}
