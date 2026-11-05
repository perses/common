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
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// debounceDelay is the time to wait after the last change before calling the callback.
// Writing a file can generate several events (for example, when it is truncated then written),
// so the callback is called once the file is not changing anymore, instead of reading a partially written file.
const debounceDelay = 100 * time.Millisecond

// Watch watches the given filename and calls the given callback when the file is changed.
// The watcher uses the parent directory as a watchpoint to be notified if the file doesn't
// exist when the watcher is created.
// When the file is a symlink (like a ConfigMap or a Secret mounted in a Kubernetes Pod), a change of its target is detected as well.
// It relies on the events of the parent directory, which are all reported by inotify on Linux, but not always by kqueue (macOS, BSD).
// The file is watched until the end of the process. Use WatchWithContext to stop watching it.
// Example:
//
//		file.Watch("/tmp/test.txt", func() {
//			fmt.Println("File created or changed")
//		}
//	)
func Watch(filename string, callback func()) error {
	return WatchWithContext(context.Background(), filename, callback)
}

// WatchWithContext is like Watch, but it stops watching the file once the context is done.
// The callback is not called anymore once the context is done, but a call in progress is not waited for.
func WatchWithContext(ctx context.Context, filename string, callback func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	// Watch the parent directory of the given filename.
	// Fix a bug with fsnotify if the file does not exist.
	dir := filepath.Dir(filename)
	if err = watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return err
	}
	// A file mounted from a Kubernetes ConfigMap or Secret is a symlink, and it is updated by changing the target of another symlink.
	// No event is received for the file itself in that case, so its target is tracked to detect these changes.
	// It is resolved before returning, so a change made right after is detected.
	target, _ := filepath.EvalSymlinks(filename)
	go watch(ctx, watcher, dir, filename, target, callback)
	return nil
}

func watch(ctx context.Context, watcher *fsnotify.Watcher, dir string, filename string, target string, callback func()) {
	defer func() {
		if err := watcher.Close(); err != nil {
			logrus.WithError(err).Errorf("Unable to close the watcher of the file %s", filename)
		}
	}()
	debounce := time.NewTimer(debounceDelay)
	debounce.Stop()
	defer debounce.Stop()
	// pending is true when the debounce timer is running.
	pending := false
	fileChanged := false
	// rewatch is true when the watched directory has been removed or renamed.
	rewatch := false
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// When the parent directory is a symlink (like the directory of a nested path in a Kubernetes ConfigMap),
			// the directory it was targeting is watched. Once this directory is removed, it is not watched anymore,
			// so the parent directory must be watched again to follow its new target.
			if filepath.Clean(event.Name) == dir && (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) {
				rewatch = true
			}
			// As we are watching the parent directory, we only care
			// about file creation and changes on the given filename.
			if (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) && filepath.Base(event.Name) == filepath.Base(filename) {
				fileChanged = true
				pending = true
				debounce.Reset(debounceDelay)
			} else if !pending {
				// Any other change in the directory can be the update of a symlink targeting the file.
				// The target is checked once the timer expires, as the change can still be in progress when the event is received.
				pending = true
				debounce.Reset(debounceDelay)
			}
		case <-debounce.C:
			if ctx.Err() != nil {
				return
			}
			pending = false
			if rewatch {
				rewatch = false
				if err := watcher.Add(dir); err != nil {
					logrus.WithError(err).Errorf("Unable to watch again the directory of the file %s", filename)
				}
			}
			if currentTarget, err := filepath.EvalSymlinks(filename); err == nil && currentTarget != target {
				target = currentTarget
				fileChanged = true
			}
			if fileChanged {
				fileChanged = false
				callback()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			if err != nil {
				logrus.WithError(err).Errorf("Unable to watch the file %s", filename)
			}
		}
	}
}
