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
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_ReadHeaderTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	builder := NewBuilder().
		APIRegistration(&mockRegister{}).
		OverrideDefaultMiddleware(true).
		ReadHeaderTimeout(200 * time.Millisecond)
	builder.ListenAddress(address)
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

	var conn net.Conn
	require.Eventually(t, func() bool {
		conn, err = net.Dial("tcp", address)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	defer func() {
		_ = conn.Close()
	}()

	// The headers are never completed: the server must close the connection once the timeout is reached.
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\n"))
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	start := time.Now()
	_, err = io.ReadAll(conn)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestBuild_HTTPServerTimeouts(t *testing.T) {
	tests := []struct {
		name                      string
		configure                 func(b *Builder)
		expectedReadHeaderTimeout time.Duration
		expectedReadTimeout       time.Duration
		expectedWriteTimeout      time.Duration
		expectedIdleTimeout       time.Duration
	}{
		{
			name:                      "default timeouts",
			configure:                 func(_ *Builder) {},
			expectedReadHeaderTimeout: defaultReadHeaderTimeout,
			expectedIdleTimeout:       defaultIdleTimeout,
		},
		{
			name: "custom timeouts",
			configure: func(b *Builder) {
				b.ReadHeaderTimeout(5 * time.Second).
					ReadTimeout(time.Minute).
					WriteTimeout(2 * time.Minute).
					IdleTimeout(30 * time.Second)
			},
			expectedReadHeaderTimeout: 5 * time.Second,
			expectedReadTimeout:       time.Minute,
			expectedWriteTimeout:      2 * time.Minute,
			expectedIdleTimeout:       30 * time.Second,
		},
		{
			name: "read timeout shorter than the default read header timeout",
			configure: func(b *Builder) {
				b.ReadTimeout(5 * time.Second)
			},
			expectedReadHeaderTimeout: 5 * time.Second,
			expectedReadTimeout:       5 * time.Second,
			expectedIdleTimeout:       defaultIdleTimeout,
		},
		{
			name: "read timeout longer than the default read header timeout",
			configure: func(b *Builder) {
				b.ReadTimeout(time.Minute)
			},
			expectedReadHeaderTimeout: defaultReadHeaderTimeout,
			expectedReadTimeout:       time.Minute,
			expectedIdleTimeout:       defaultIdleTimeout,
		},
		{
			name: "timeouts disabled",
			configure: func(b *Builder) {
				b.ReadHeaderTimeout(-1).IdleTimeout(-1)
			},
			expectedReadHeaderTimeout: -1,
			expectedIdleTimeout:       -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true)
			tt.configure(builder)
			s, err := builder.build()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedReadHeaderTimeout, s.readHeaderTimeout)
			assert.Equal(t, tt.expectedReadTimeout, s.readTimeout)
			assert.Equal(t, tt.expectedWriteTimeout, s.writeTimeout)
			assert.Equal(t, tt.expectedIdleTimeout, s.idleTimeout)
		})
	}
}
