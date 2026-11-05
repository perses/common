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
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_ExecuteReturnsTheStartupError(t *testing.T) {
	// The address is already used, so the server cannot listen on it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()

	tests := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{
			name: "HTTP server, address already in use",
		},
		{
			name:     "HTTPS server, missing certificate",
			certFile: filepath.Join(t.TempDir(), "tls.crt"),
			keyFile:  filepath.Join(t.TempDir(), "tls.key"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCert := cert
			origKey := key
			cert = tt.certFile
			key = tt.keyFile
			defer func() {
				cert = origCert
				key = origKey
			}()

			builder := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true)
			builder.ListenAddress(listener.Addr().String())
			task, err := builder.Build()
			require.NoError(t, err)
			require.NoError(t, task.Initialize())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err = task.Execute(ctx, cancel)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "http server stopped unexpectedly")
			// The whole application must be stopped.
			assert.Error(t, ctx.Err())
			assert.NoError(t, task.Finalize())
		})
	}
}
