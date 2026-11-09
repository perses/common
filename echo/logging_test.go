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
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

	// The errors of the server are logged with logrus. The TLS handshake errors are caused by the clients, so they are warnings.
	require.NotNil(t, httpServer.ErrorLog)
	tests := []struct {
		name          string
		line          string
		expectedLevel logrus.Level
	}{
		{
			name:          "TLS handshake error",
			line:          "http: TLS handshake error from 127.0.0.1:1234: EOF",
			expectedLevel: logrus.WarnLevel,
		},
		{
			name:          "panic not recovered by a middleware",
			line:          "http: panic serving 127.0.0.1:1234: boom",
			expectedLevel: logrus.ErrorLevel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer.ErrorLog.Print(tt.line)
			entry := hook.LastEntry()
			require.NotNil(t, entry)
			assert.Equal(t, tt.expectedLevel, entry.Level)
			assert.Equal(t, tt.line, entry.Message)
		})
	}
}

// startServer starts the server built by the builder on a random port and returns its address, read from the logs.
// The server is stopped at the end of the test.
func startServer(t *testing.T, hook *logrustest.Hook, builder *Builder) string {
	t.Helper()
	builder.ListenAddress("127.0.0.1:0")
	task, err := builder.Build()
	require.NoError(t, err)
	require.NoError(t, task.Initialize())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var executeErr error
	go func() {
		defer close(done)
		executeErr = task.Execute(ctx, cancel)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		assert.NoError(t, executeErr)
		assert.NoError(t, task.Finalize())
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
	s, err := builder.build()
	require.NoError(t, err)
	s.shutdownTimeout = 50 * time.Millisecond
	require.NoError(t, s.Initialize())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	var executeErr error
	go func() {
		defer close(done)
		executeErr = s.Execute(ctx, cancel)
	}()
	address := waitForListenAddress(t, hook)

	// A request is still running when the server is stopped, so the graceful shutdown reaches its timeout.
	requestErr := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, reqErr := client.Get("http://" + address + "/block")
		if reqErr == nil {
			reqErr = resp.Body.Close()
		}
		requestErr <- reqErr
	}()
	select {
	case <-api.started:
	case reqErr := <-requestErr:
		t.Fatalf("the request ended before reaching the handler: %v", reqErr)
	case <-time.After(5 * time.Second):
		t.Fatal("the request didn't reach the handler")
	}
	cancel()
	<-done
	close(api.release)
	// The shutdown timeout doesn't interrupt the request still running: it ends normally once released.
	assert.NoError(t, <-requestErr)

	// The server stopped as requested, but its shutdown failed: the task must report it, like before echo v5.
	assert.NoError(t, executeErr)
	err = s.Finalize()
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorContains(t, err, "server shutdown not properly")
}

func TestServer_FinalizeWithoutShutdownError(t *testing.T) {
	hook := newLogHook(t)
	address := startServer(t, hook, NewBuilder().APIRegistration(&mockRegister{}).OverrideDefaultMiddleware(true))
	assert.NotEmpty(t, address)
	// startServer stops the server and checks Finalize returns no error at the end of the test.
}

func TestLogrusHandler(t *testing.T) {
	testErr := errors.New("boom")
	tests := []struct {
		name           string
		log            func(logger *slog.Logger)
		expectedLevel  logrus.Level
		expectedFields logrus.Fields
	}{
		{
			name:           "debug",
			log:            func(logger *slog.Logger) { logger.Debug("message") },
			expectedLevel:  logrus.DebugLevel,
			expectedFields: logrus.Fields{},
		},
		{
			name:           "info with attributes",
			log:            func(logger *slog.Logger) { logger.Info("message", "address", "127.0.0.1:8080", "count", 2) },
			expectedLevel:  logrus.InfoLevel,
			expectedFields: logrus.Fields{"address": "127.0.0.1:8080", "count": int64(2)},
		},
		{
			name:           "warning",
			log:            func(logger *slog.Logger) { logger.Warn("message") },
			expectedLevel:  logrus.WarnLevel,
			expectedFields: logrus.Fields{},
		},
		{
			name:           "error with the error as attribute",
			log:            func(logger *slog.Logger) { logger.Error("message", "error", testErr) },
			expectedLevel:  logrus.ErrorLevel,
			expectedFields: logrus.Fields{logrus.ErrorKey: testErr},
		},
		{
			// logrus would exit or panic with the levels above error.
			name:           "level above error",
			log:            func(logger *slog.Logger) { logger.Log(context.Background(), slog.LevelError+4, "message") },
			expectedLevel:  logrus.ErrorLevel,
			expectedFields: logrus.Fields{},
		},
		{
			name: "attributes and groups added to the logger",
			log: func(logger *slog.Logger) {
				logger.With("component", "echo").WithGroup("request").With("method", http.MethodGet).
					Info("message", "status", http.StatusOK, slog.Group("user", "name", "admin"))
			},
			expectedLevel: logrus.InfoLevel,
			expectedFields: logrus.Fields{
				"component":         "echo",
				"request.method":    http.MethodGet,
				"request.status":    int64(http.StatusOK),
				"request.user.name": "admin",
			},
		},
		{
			name: "empty attributes are ignored, and the attributes of a group without key are inlined",
			log: func(logger *slog.Logger) {
				logger.WithGroup("").Info("message", slog.Attr{}, slog.Group("", "inlined", true), slog.Group("empty"))
			},
			expectedLevel:  logrus.InfoLevel,
			expectedFields: logrus.Fields{"inlined": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newLogHook(t)
			tt.log(slog.New(logrusHandler{}))
			require.Len(t, hook.AllEntries(), 1)
			entry := hook.LastEntry()
			assert.Equal(t, "message", entry.Message)
			assert.Equal(t, tt.expectedLevel, entry.Level)
			assert.Equal(t, tt.expectedFields, entry.Data)
		})
	}
}

func TestLogrusHandler_Enabled(t *testing.T) {
	hook := newLogHook(t)
	logrus.SetLevel(logrus.WarnLevel)
	logger := slog.New(logrusHandler{})
	assert.False(t, logger.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, logger.Enabled(context.Background(), slog.LevelWarn))
	logger.Info("message")
	assert.Empty(t, hook.AllEntries())
}

type doubleWriteRegister struct{}

func (r *doubleWriteRegister) RegisterRoute(e *echoLib.Echo) {
	e.GET("/double-write", func(c *echoLib.Context) error {
		if err := c.NoContent(http.StatusOK); err != nil {
			return err
		}
		return c.NoContent(http.StatusAccepted)
	})
}

func TestServer_EchoLogsWithLogrus(t *testing.T) {
	hook := newLogHook(t)
	handler, err := NewBuilder().APIRegistration(&doubleWriteRegister{}).OverrideDefaultMiddleware(true).BuildHandler()
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/double-write", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	// echo logs with log/slog that the response was already written: the log is written with logrus.
	entry := findLogEntry(hook, "echo: response already written to client")
	require.NotNil(t, entry)
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
}
