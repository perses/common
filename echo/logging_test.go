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

package echo

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	echoLib "github.com/labstack/echo/v5"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_ConfigureHTTPServer(t *testing.T) {
	hook := newLogHook(t)
	s, err := NewBuilder().
		APIRegistration(&mockRegister{}).
		OverrideDefaultMiddleware(true).
		build()
	require.NoError(t, err)

	// echo creates the http.Server with a ReadTimeout of 30s, which must not be kept, as there is no read timeout by default.
	httpServer := &http.Server{ReadTimeout: 30 * time.Second}
	s.configureHTTPServer(httpServer)
	assert.Equal(t, defaultReadHeaderTimeout, httpServer.ReadHeaderTimeout)
	assert.Equal(t, time.Duration(0), httpServer.ReadTimeout)
	assert.Equal(t, time.Duration(0), httpServer.WriteTimeout)
	assert.Equal(t, defaultIdleTimeout, httpServer.IdleTimeout)

	// The errors of the server are logged with logrus.
	require.NotNil(t, httpServer.ErrorLog)
	httpServer.ErrorLog.Printf("http: TLS handshake error from %s: EOF", "127.0.0.1:1234")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
	assert.Equal(t, "http: TLS handshake error from 127.0.0.1:1234: EOF", entry.Message)
}

// startServer starts the server built by the builder on a random port and returns its address, read from the logs.
// The server is stopped at the end of the test.
func startServer(t *testing.T, hook *logrustest.Hook, builder *Builder) string {
	t.Helper()
	builder.ListenAddress("127.0.0.1:0")
	task, err := builder.Build()
	require.NoError(t, err)
	require.NoError(t, task.Initialize())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = task.Execute(ctx, cancel)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		require.NoError(t, task.Finalize())
	})
	return waitForListenAddress(t, hook)
}

// waitForListenAddress waits for the server to log the address it listens on, and returns it.
func waitForListenAddress(t *testing.T, hook *logrustest.Hook) string {
	t.Helper()
	var address string
	require.Eventually(t, func() bool {
		for _, entry := range hook.AllEntries() {
			if after, ok := strings.CutPrefix(entry.Message, "http server started on "); ok {
				address = after
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	return address
}

func TestServer_LogsTheListenAddress(t *testing.T) {
	hook := newLogHook(t)
	address := startServer(t, hook, NewBuilder().APIRegistration(&mockRegister{}).OverrideDefaultMiddleware(true))
	assert.True(t, strings.HasPrefix(address, "127.0.0.1:"))
	assert.NotEqual(t, "127.0.0.1:0", address)
}

func TestServer_HidesTheListenAddress(t *testing.T) {
	origHidePort := hidePort
	hidePort = true
	t.Cleanup(func() {
		hidePort = origHidePort
	})
	hook := newLogHook(t)
	s, err := NewBuilder().APIRegistration(&mockRegister{}).OverrideDefaultMiddleware(true).build()
	require.NoError(t, err)
	s.logListenerAddr(nil)
	assert.Empty(t, hook.AllEntries())
}

type blockingRegister struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingRegister) RegisterRoute(e *echoLib.Echo) {
	e.GET("/block", func(c *echoLib.Context) error {
		close(r.started)
		<-r.release
		return c.NoContent(http.StatusOK)
	})
}

func TestServer_FinalizeReturnsTheShutdownTimeout(t *testing.T) {
	hook := newLogHook(t)
	api := &blockingRegister{started: make(chan struct{}), release: make(chan struct{})}
	builder := NewBuilder().APIRegistration(api).OverrideDefaultMiddleware(true)
	builder.ListenAddress("127.0.0.1:0")
	task, err := builder.Build()
	require.NoError(t, err)
	task.(*server).shutdownTimeout = 50 * time.Millisecond
	require.NoError(t, task.Initialize())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var executeErr error
	go func() {
		defer close(done)
		executeErr = task.Execute(ctx, cancel)
	}()
	address := waitForListenAddress(t, hook)

	// A request is still running when the server is stopped, so the graceful shutdown reaches its timeout.
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, reqErr := http.Get("http://" + address + "/block")
		if reqErr == nil {
			_ = resp.Body.Close()
		}
	}()
	<-api.started
	cancel()
	<-done
	close(api.release)
	<-requestDone

	// The server stopped as requested, but its shutdown failed: the task must report it, like before echo v5.
	assert.NoError(t, executeErr)
	err = task.Finalize()
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "server shutdown not properly")
}

func TestServer_FinalizeWithoutShutdownError(t *testing.T) {
	hook := newLogHook(t)
	address := startServer(t, hook, NewBuilder().APIRegistration(&mockRegister{}).OverrideDefaultMiddleware(true))
	assert.NotEmpty(t, address)
	// startServer stops the server and checks Finalize returns no error at the end of the test.
}
