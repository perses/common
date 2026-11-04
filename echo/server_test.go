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
	"crypto/tls"
	"net"
	"testing"
	"time"

	echoLib "github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    uint16
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty string returns 0",
			input:       "",
			expected:    0,
			expectError: false,
		},
		{
			name:        "TLS 1.0",
			input:       "1.0",
			expected:    tls.VersionTLS10,
			expectError: false,
		},
		{
			name:        "TLS 1.1",
			input:       "1.1",
			expected:    tls.VersionTLS11,
			expectError: false,
		},
		{
			name:        "TLS 1.2",
			input:       "1.2",
			expected:    tls.VersionTLS12,
			expectError: false,
		},
		{
			name:        "TLS 1.3",
			input:       "1.3",
			expected:    tls.VersionTLS13,
			expectError: false,
		},
		{
			name:        "invalid version 1.5",
			input:       "1.5",
			expected:    0,
			expectError: true,
			errorMsg:    "unknown TLS version",
		},
		{
			name:        "invalid version abc",
			input:       "abc",
			expected:    0,
			expectError: true,
			errorMsg:    "unknown TLS version",
		},
		{
			name:        "invalid version TLS1.2",
			input:       "TLS1.2",
			expected:    0,
			expectError: true,
			errorMsg:    "unknown TLS version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTLSVersion(tt.input)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestTLSVersionToString(t *testing.T) {
	tests := []struct {
		name     string
		input    uint16
		expected string
	}{
		{
			name:     "TLS 1.0",
			input:    tls.VersionTLS10,
			expected: "1.0",
		},
		{
			name:     "TLS 1.1",
			input:    tls.VersionTLS11,
			expected: "1.1",
		},
		{
			name:     "TLS 1.2",
			input:    tls.VersionTLS12,
			expected: "1.2",
		},
		{
			name:     "TLS 1.3",
			input:    tls.VersionTLS13,
			expected: "1.3",
		},
		{
			name:     "unknown version",
			input:    0,
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tlsVersionToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []uint16
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty string returns nil",
			input:       "",
			expected:    nil,
			expectError: false,
		},
		{
			name:        "single valid cipher suite",
			input:       "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			expected:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			expectError: false,
		},
		{
			name:        "multiple valid cipher suites",
			input:       "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			expected:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
			expectError: false,
		},
		{
			name:        "cipher suites with whitespace",
			input:       "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 , TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			expected:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
			expectError: false,
		},
		{
			name:        "cipher suites with extra commas",
			input:       "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,",
			expected:    []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384},
			expectError: false,
		},
		{
			name:        "TLS 1.3 cipher suite",
			input:       "TLS_AES_128_GCM_SHA256",
			expected:    []uint16{tls.TLS_AES_128_GCM_SHA256},
			expectError: false,
		},
		{
			name:        "unknown cipher suite",
			input:       "UNKNOWN_CIPHER_SUITE",
			expected:    nil,
			expectError: true,
			errorMsg:    "unknown cipher suite",
		},
		{
			name:        "one valid one invalid cipher suite",
			input:       "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,INVALID_CIPHER",
			expected:    nil,
			expectError: true,
			errorMsg:    "unknown cipher suite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCipherSuites(tt.input)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// mockRegister is a simple implementation of Register for testing
type mockRegister struct{}

func (m *mockRegister) RegisterRoute(e *echoLib.Echo) {}

func TestBuild_TLSVersionValidation(t *testing.T) {
	tests := []struct {
		name        string
		minVersion  string
		maxVersion  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "no TLS versions set",
			minVersion:  "",
			maxVersion:  "",
			expectError: false,
		},
		{
			name:        "only min version set",
			minVersion:  "1.2",
			maxVersion:  "",
			expectError: false,
		},
		{
			name:        "only max version set",
			minVersion:  "",
			maxVersion:  "1.3",
			expectError: false,
		},
		{
			name:        "min equals max",
			minVersion:  "1.2",
			maxVersion:  "1.2",
			expectError: false,
		},
		{
			name:        "min less than max",
			minVersion:  "1.2",
			maxVersion:  "1.3",
			expectError: false,
		},
		{
			name:        "min greater than max - error",
			minVersion:  "1.3",
			maxVersion:  "1.2",
			expectError: true,
			errorMsg:    "TLS min version (1.3) cannot be greater than max version (1.2)",
		},
		{
			name:        "invalid min version",
			minVersion:  "invalid",
			maxVersion:  "1.3",
			expectError: true,
			errorMsg:    "invalid TLS min version",
		},
		{
			name:        "invalid max version",
			minVersion:  "1.2",
			maxVersion:  "invalid",
			expectError: true,
			errorMsg:    "invalid TLS max version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and set flag values
			origMinVersion := tlsMinVersion
			origMaxVersion := tlsMaxVersion
			tlsMinVersion = tt.minVersion
			tlsMaxVersion = tt.maxVersion
			defer func() {
				tlsMinVersion = origMinVersion
				tlsMaxVersion = origMaxVersion
			}()

			builder := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true)

			_, err := builder.Build()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuild_TLSCipherSuitesValidation(t *testing.T) {
	tests := []struct {
		name        string
		suites      string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty cipher suites",
			suites:      "",
			expectError: false,
		},
		{
			name:        "valid cipher suites",
			suites:      "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			expectError: false,
		},
		{
			name:        "invalid cipher suites",
			suites:      "INVALID_CIPHER",
			expectError: true,
			errorMsg:    "invalid TLS cipher suites",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and set flag values
			origCipherSuites := tlsCipherSuites
			tlsCipherSuites = tt.suites
			defer func() {
				tlsCipherSuites = origCipherSuites
			}()

			builder := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true)

			_, err := builder.Build()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuild_TLSFlagValues(t *testing.T) {
	// Save original flag values
	origMinVersion := tlsMinVersion
	origMaxVersion := tlsMaxVersion
	origCipherSuites := tlsCipherSuites
	origCert := cert
	origKey := key

	// Set flag values
	tlsMinVersion = "1.2"
	tlsMaxVersion = "1.3"
	tlsCipherSuites = "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	cert = "/path/cert.pem"
	key = "/path/key.pem"

	// Restore original values after test
	defer func() {
		tlsMinVersion = origMinVersion
		tlsMaxVersion = origMaxVersion
		tlsCipherSuites = origCipherSuites
		cert = origCert
		key = origKey
	}()

	// Create a fresh registry to avoid duplicate registration errors
	reg := prometheus.NewRegistry()

	builder := NewBuilder().
		APIRegistration(&mockRegister{}).
		PrometheusRegisterer(reg)

	task, err := builder.Build()
	require.NoError(t, err)

	// Access the internal server to verify TLS settings
	s, ok := task.(*server)
	require.True(t, ok)

	// Verify flag values are used
	assert.Equal(t, uint16(tls.VersionTLS12), s.tlsMinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), s.tlsMaxVersion)
	assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, s.tlsCipherSuites)
	assert.Equal(t, "/path/cert.pem", s.cert)
	assert.Equal(t, "/path/key.pem", s.key)
}

func TestIsHTTP2Compatible(t *testing.T) {
	tests := []struct {
		name         string
		minVersion   uint16
		cipherSuites []uint16
		expected     bool
	}{
		{
			name:     "default cipher suites",
			expected: true,
		},
		{
			name:         "with TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			minVersion:   tls.VersionTLS12,
			cipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			expected:     true,
		},
		{
			name:         "with TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
			cipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
			expected:     true,
		},
		{
			name:         "without a cipher suite required by HTTP/2",
			minVersion:   tls.VersionTLS12,
			cipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384},
			expected:     false,
		},
		{
			name:         "without a cipher suite required by HTTP/2 and the default min version",
			cipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384},
			expected:     false,
		},
		{
			name:         "without a cipher suite required by HTTP/2 but only TLS 1.3",
			minVersion:   tls.VersionTLS13,
			cipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isHTTP2Compatible(tt.minVersion, tt.cipherSuites))
		})
	}
}

// startTLSServer starts the HTTPS server with the given TLS options and returns its address.
// The server is stopped at the end of the test.
func startTLSServer(t *testing.T, certFile, keyFile, minVersion, cipherSuites string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	origMinVersion := tlsMinVersion
	origCipherSuites := tlsCipherSuites
	origCert := cert
	origKey := key
	tlsMinVersion = minVersion
	tlsCipherSuites = cipherSuites
	cert = certFile
	key = keyFile
	defer func() {
		tlsMinVersion = origMinVersion
		tlsCipherSuites = origCipherSuites
		cert = origCert
		key = origKey
	}()

	builder := NewBuilder().
		APIRegistration(&mockRegister{}).
		PrometheusRegisterer(prometheus.NewRegistry())
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

	require.Eventually(t, func() bool {
		_, dialErr := dialTLSServer(address, &tls.Config{})
		return dialErr == nil
	}, 5*time.Second, 10*time.Millisecond)
	return address
}

func dialTLSServer(address string, config *tls.Config) (tls.ConnectionState, error) {
	// The certificates are self-signed, and the tests only check which one is served.
	config.InsecureSkipVerify = true
	config.NextProtos = []string{"h2", "http/1.1"}
	conn, err := tls.Dial("tcp", address, config)
	if err != nil {
		return tls.ConnectionState{}, err
	}
	defer func() {
		_ = conn.Close()
	}()
	return conn.ConnectionState(), nil
}

func TestServer_TLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertificate(t, dir, 1)
	address := startTLSServer(t, certFile, keyFile, "1.3", "")

	state, err := dialTLSServer(address, &tls.Config{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), state.PeerCertificates[0].SerialNumber.Int64())
	assert.Equal(t, "h2", state.NegotiatedProtocol)

	// The TLS min version is applied.
	_, err = dialTLSServer(address, &tls.Config{MaxVersion: tls.VersionTLS12})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocol version not supported")

	// The certificate is rotated: the new connections get the new one.
	writeCertificate(t, dir, 2)
	state, err = dialTLSServer(address, &tls.Config{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), state.PeerCertificates[0].SerialNumber.Int64())
}

func TestServer_TLSCipherSuitesWithoutHTTP2(t *testing.T) {
	certFile, keyFile := writeCertificate(t, t.TempDir(), 1)
	address := startTLSServer(t, certFile, keyFile, "1.2", "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384")

	// The server starts, and the configured cipher suite is used with HTTP/1.1.
	state, err := dialTLSServer(address, &tls.Config{MaxVersion: tls.VersionTLS12})
	require.NoError(t, err)
	assert.Equal(t, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, state.CipherSuite)
	assert.Empty(t, state.NegotiatedProtocol)

	// The other cipher suites are rejected.
	_, err = dialTLSServer(address, &tls.Config{
		MaxVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
	})
	require.Error(t, err)
}
