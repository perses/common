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
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	waitFor = 5 * time.Second
	tick    = 10 * time.Millisecond
	// noEventDelay is the time to wait to check that the callback is not called.
	noEventDelay = 5 * debounceDelay
)

func watchFile(t *testing.T, filename string) *atomic.Int32 {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	calls := &atomic.Int32{}
	require.NoError(t, WatchWithContext(ctx, filename, func() {
		calls.Add(1)
	}))
	return calls
}

func TestWatch_FileChanges(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	calls := watchFile(t, filename)

	// The file is created.
	require.NoError(t, os.WriteFile(filename, []byte("a: 1"), 0600))
	assert.Eventually(t, func() bool { return calls.Load() == 1 }, waitFor, tick)

	// Several writes in a row only call the callback once, when the file is not changing anymore.
	for _, content := range []string{"a: 2", "a: 3", "a: 4"} {
		require.NoError(t, os.WriteFile(filename, []byte(content), 0600))
	}
	assert.Eventually(t, func() bool { return calls.Load() == 2 }, waitFor, tick)
	time.Sleep(noEventDelay)
	assert.Equal(t, int32(2), calls.Load())

	// The other files of the directory are ignored.
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(filename), "other.yaml"), []byte("b: 1"), 0600))
	time.Sleep(noEventDelay)
	assert.Equal(t, int32(2), calls.Load())
}

// TestWatch_SymlinkTargetChanges reproduces how Kubernetes updates a ConfigMap or a Secret mounted in a Pod:
// the file is a symlink to ..data/config.yaml, and ..data is a symlink to a directory that is atomically replaced.
func TestWatch_SymlinkTargetChanges(t *testing.T) {
	if runtime.GOOS != "linux" {
		// kqueue doesn't always report a directory entry replaced by a rename, so the change cannot be reliably detected.
		t.Skip("the update of a symlink by a rename is only reliably reported by inotify")
	}
	dir := t.TempDir()
	for _, version := range []string{"..v1", "..v2"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, version), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, version, "config.yaml"), []byte(version), 0600))
	}
	require.NoError(t, os.Symlink("..v1", filepath.Join(dir, "..data")))
	filename := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.Symlink(filepath.Join("..data", "config.yaml"), filename))
	calls := watchFile(t, filename)

	require.NoError(t, os.Symlink("..v2", filepath.Join(dir, "..data_tmp")))
	require.NoError(t, os.Rename(filepath.Join(dir, "..data_tmp"), filepath.Join(dir, "..data")))
	assert.Eventually(t, func() bool { return calls.Load() == 1 }, waitFor, tick)
}

func TestWatchWithContext_Stop(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	ctx, cancel := context.WithCancel(context.Background())
	calls := &atomic.Int32{}
	require.NoError(t, WatchWithContext(ctx, filename, func() {
		calls.Add(1)
	}))

	cancel()
	require.NoError(t, os.WriteFile(filename, []byte("a: 1"), 0600))
	time.Sleep(noEventDelay)
	assert.Equal(t, int32(0), calls.Load())
}

func TestWatch_DirectoryNotFound(t *testing.T) {
	err := Watch(filepath.Join(t.TempDir(), "missing", "config.yaml"), func() {})
	assert.Error(t, err)
}
