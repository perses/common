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

//go:build unix

package async

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signalListenerHelperEnv is the environment variable telling the test binary to run runSignalListenerHelper.
const signalListenerHelperEnv = "PERSES_TEST_SIGNAL_LISTENER_HELPER"

const (
	firstSignalHandledMessage = "the first signal has been handled"
	stillAliveMessage         = "the process is still alive after the second signal"
)

func TestMain(m *testing.M) {
	if os.Getenv(signalListenerHelperEnv) == "1" {
		runSignalListenerHelper()
		return
	}
	os.Exit(m.Run())
}

// runSignalListenerHelper runs the signal listener in a dedicated process, then sends SIGTERM to itself twice.
// The first one must cancel the context, and the second one must kill the process.
func runSignalListenerHelper() {
	// Until the listener has handled a signal, SIGTERM is caught here too: the signals sent before the listener is
	// ready don't kill the process.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executed := make(chan struct{})
	go func() {
		defer close(executed)
		_ = NewSignalListener(syscall.SIGTERM).Execute(ctx, cancel)
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
	<-executed
	signal.Stop(guard)
	fmt.Println(firstSignalHandledMessage)
	// The listener is done: the second signal is not caught anymore, so it kills the process.
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	fmt.Println(stillAliveMessage)
}

func TestSignalListener_StopCatchingTheSignalsOnceOneIsReceived(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), signalListenerHelperEnv+"=1")
	output, err := cmd.Output()
	assert.Contains(t, string(output), firstSignalHandledMessage)
	assert.NotContains(t, string(output), stillAliveMessage)
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	require.True(t, ok, "the process must be killed by the second signal, got the error %v", err)
	status := exitErr.Sys().(syscall.WaitStatus)
	assert.True(t, status.Signaled() && status.Signal() == syscall.SIGTERM, "the process must be killed by SIGTERM, got %s", exitErr)
}
