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

package file

import (
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"path/filepath"
)

func Watch(filename string, callback func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write && event.Name == filename {
					callback()
				}
			case err := <-watcher.Errors:
				if err != nil {
					logrus.WithError(err).Errorf("Unable to watch the file %s", filename)
				}
			}
		}
	}()
	err = watcher.Add(filepath.Dir(filename))
	return err
}
