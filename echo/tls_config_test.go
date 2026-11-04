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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild_PartialTLSConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		certFile    string
		keyFile     string
		expectError bool
	}{
		{
			name: "HTTP",
		},
		{
			name:     "HTTPS",
			certFile: "tls.crt",
			keyFile:  "tls.key",
		},
		{
			name:        "only the certificate is set",
			certFile:    "tls.crt",
			expectError: true,
		},
		{
			name:        "only the key is set",
			keyFile:     "tls.key",
			expectError: true,
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

			_, err := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true).
				Build()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "both --web.tls-cert-file and --web.tls-key-file must be set")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
