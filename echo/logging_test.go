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
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	echoLib "github.com/labstack/echo/v5"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			// slog has no level above error: the levels above it are logged as errors.
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

func TestLogrusHandler_Caller(t *testing.T) {
	hook := newLogHook(t)
	logger := slog.New(logrusHandler{})

	// The caller is not set when logrus doesn't report it, as logrus writes it as soon as it is set.
	logger.Info("message")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Nil(t, entry.Caller)

	// When logrus reports the caller, it is the code calling slog, not the handler.
	origReportCaller := logrus.StandardLogger().ReportCaller
	logrus.SetReportCaller(true)
	t.Cleanup(func() {
		logrus.SetReportCaller(origReportCaller)
	})
	logger.Info("message")
	entry = hook.LastEntry()
	require.NotNil(t, entry.Caller)
	assert.Equal(t, "github.com/perses/common/echo.TestLogrusHandler_Caller", entry.Caller.Function)
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

func TestHTTPServerErrorWriter(t *testing.T) {
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
			name:          "HTTP/2 preface sent by the client",
			line:          `http2: server: error reading preface from client 127.0.0.1:1234: bogus greeting "GET / HTTP/1.1"`,
			expectedLevel: logrus.WarnLevel,
		},
		{
			name:          "HTTP/2 protocol error of the client",
			line:          "http2: server connection error from 127.0.0.1:1234: connection error: PROTOCOL_ERROR",
			expectedLevel: logrus.WarnLevel,
		},
		{
			name:          "panic not recovered by a middleware",
			line:          "http: panic serving 127.0.0.1:1234: boom\ngoroutine 7 [running]:",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			name:          "HTTP/2 panic not recovered by a middleware",
			line:          "http2: panic serving 127.0.0.1:1234: boom\ngoroutine 7 [running]:",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			name:          "handler writing the header twice",
			line:          "http: superfluous response.WriteHeader call from main.handler (main.go:12)",
			expectedLevel: logrus.ErrorLevel,
		},
		{
			name:          "connections not accepted",
			line:          "http: Accept error: accept tcp 127.0.0.1:8080: accept: too many open files; retrying in 5ms",
			expectedLevel: logrus.ErrorLevel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := newLogHook(t)
			log.New(httpServerErrorWriter{}, "", 0).Print(tt.line)
			entry := hook.LastEntry()
			require.NotNil(t, entry)
			assert.Equal(t, tt.expectedLevel, entry.Level)
			assert.Equal(t, tt.line, entry.Message)
		})
	}
}

func TestServer_LogListenerAddr(t *testing.T) {
	tests := []struct {
		name           string
		hidePort       bool
		expectedLogged bool
	}{
		{
			name:           "the address is logged",
			expectedLogged: true,
		},
		{
			name:     "the address is hidden with --web.hide-port",
			hidePort: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origHidePort := hidePort
			hidePort = tt.hidePort
			t.Cleanup(func() {
				hidePort = origHidePort
			})
			hook := newLogHook(t)
			address := freeAddress(t)
			builder := NewBuilder().APIRegistration(&helloRegister{}).OverrideDefaultMiddleware(true)
			builder.ListenAddress(address)
			runServer(t, builder)

			// Once the server answers, it has logged that it started (or not).
			client := &http.Client{Timeout: time.Second}
			require.Eventually(t, func() bool {
				resp, err := client.Get("http://" + address + "/hello")
				if err != nil {
					return false
				}
				_ = resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}, 5*time.Second, 10*time.Millisecond)
			logged := slices.ContainsFunc(hook.AllEntries(), func(entry *logrus.Entry) bool {
				return strings.Contains(fmt.Sprint(entry.Message, entry.Data), address)
			})
			assert.Equal(t, tt.expectedLogged, logged)
		})
	}
}

// freeAddress returns a local address with a port that is free when the function returns.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}
