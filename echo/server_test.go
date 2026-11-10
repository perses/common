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
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	echoLib "github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
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

	// The errors of the server are logged with logrus (their levels are tested by TestHTTPServerErrorWriter).
	require.NotNil(t, httpServer.ErrorLog)
	httpServer.ErrorLog.Print("http: TLS handshake error from 127.0.0.1:1234: EOF")
	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, "http: TLS handshake error from 127.0.0.1:1234: EOF", entry.Message)
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

	// echo wraps the listener with TLS before serving, so the server is ready once the port is open. A TLS handshake
	// would not work when a client CA is set with TLS 1.2, as the server rejects a client without certificate.
	require.Eventually(t, func() bool {
		conn, dialErr := net.Dial("tcp", address)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
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

func TestServer_TLSClientCA(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertificate(t, dir, 1)
	// The client certificates are self-signed, so they are their own CA.
	clientPEM, clientKeyPEM := generateCertificate(t, 2)
	clientCertificate, err := tls.X509KeyPair(clientPEM, clientKeyPEM)
	require.NoError(t, err)
	otherClientPEM, otherClientKeyPEM := generateCertificate(t, 3)
	otherClientCertificate, err := tls.X509KeyPair(otherClientPEM, otherClientKeyPEM)
	require.NoError(t, err)
	caFile := filepath.Join(dir, "ca.crt")
	replaceFile(t, caFile, clientPEM)

	origClientCAFile := clientCAFile
	clientCAFile = caFile
	defer func() {
		clientCAFile = origClientCAFile
	}()
	address := startTLSServer(t, certFile, keyFile, "", "")

	// An HTTP request is sent, as with TLS 1.3, the client completes the handshake before the server verifies its
	// certificate: a rejected certificate is only reported when the client reads the response.
	get := func(certificates ...tls.Certificate) error {
		client := &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, Certificates: certificates},
				DisableKeepAlives: true,
			},
		}
		resp, reqErr := client.Get("https://" + address)
		if reqErr != nil {
			return reqErr
		}
		return resp.Body.Close()
	}

	// Only the clients presenting a certificate signed by the CA are accepted.
	require.NoError(t, get(clientCertificate))
	assert.ErrorContains(t, get(), "certificate required")
	assert.ErrorContains(t, get(otherClientCertificate), "unknown certificate authority")

	// The CA is rotated: the new connections are verified with the new one.
	replaceFile(t, caFile, otherClientPEM)
	require.NoError(t, get(otherClientCertificate))
	assert.ErrorContains(t, get(clientCertificate), "unknown certificate authority")

	// The new file is invalid: the previous CA is kept.
	replaceFile(t, caFile, []byte("not a certificate"))
	require.NoError(t, get(otherClientCertificate))
	assert.ErrorContains(t, get(clientCertificate), "unknown certificate authority")
}

// A resumed session must not bypass the rotation of the client CA.
func TestServer_TLSClientCAResumption(t *testing.T) {
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			dir := t.TempDir()
			certFile, keyFile := writeCertificate(t, dir, 1)
			clientPEM, clientKeyPEM := generateCertificate(t, 2)
			clientCertificate, err := tls.X509KeyPair(clientPEM, clientKeyPEM)
			require.NoError(t, err)
			otherClientPEM, _ := generateCertificate(t, 3)
			caFile := filepath.Join(dir, "ca.crt")
			replaceFile(t, caFile, clientPEM)

			origClientCAFile := clientCAFile
			clientCAFile = caFile
			defer func() {
				clientCAFile = origClientCAFile
			}()
			address := startTLSServer(t, certFile, keyFile, "", "")

			// Every connection uses the same TLS configuration, so the same session cache.
			clientConfig := &tls.Config{
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{clientCertificate},
				ClientSessionCache: tls.NewLRUClientSessionCache(1),
				MaxVersion:         version,
			}
			get := func() (*tls.ConnectionState, error) {
				client := &http.Client{
					Timeout:   5 * time.Second,
					Transport: &http.Transport{TLSClientConfig: clientConfig, DisableKeepAlives: true},
				}
				resp, reqErr := client.Get("https://" + address)
				if reqErr != nil {
					return nil, reqErr
				}
				return resp.TLS, resp.Body.Close()
			}

			_, err = get()
			require.NoError(t, err)
			state, err := get()
			require.NoError(t, err)
			require.True(t, state.DidResume)

			// The CA is rotated: the session is not resumed, and the full handshake fails.
			replaceFile(t, caFile, otherClientPEM)
			_, err = get()
			assert.ErrorContains(t, err, "unknown certificate authority")
		})
	}
}

func TestBuild_PartialTLSConfiguration(t *testing.T) {
	origHooks := logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	t.Cleanup(func() {
		logrus.StandardLogger().ReplaceHooks(origHooks)
	})
	hook := logrustest.NewLocal(logrus.StandardLogger())

	tests := []struct {
		name            string
		certFile        string
		keyFile         string
		clientCAFile    string
		minVersion      string
		expectedError   string
		expectedWarning bool
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
			name:          "only the certificate is set",
			certFile:      "tls.crt",
			expectedError: "--web.tls-key-file must be set when --web.tls-cert-file is set",
		},
		{
			name:          "only the key is set",
			keyFile:       "tls.key",
			expectedError: "--web.tls-cert-file must be set when --web.tls-key-file is set",
		},
		{
			name:          "only the client CA is set",
			clientCAFile:  "ca.crt",
			expectedError: "--web.tls-cert-file and --web.tls-key-file must be set when --web.tls-client-ca-file is set",
		},
		{
			name:            "TLS option without HTTPS",
			minVersion:      "1.2",
			expectedWarning: true,
		},
		{
			name:       "TLS option with HTTPS",
			certFile:   "tls.crt",
			keyFile:    "tls.key",
			minVersion: "1.2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCert := cert
			origKey := key
			origClientCAFile := clientCAFile
			origMinVersion := tlsMinVersion
			cert = tt.certFile
			key = tt.keyFile
			clientCAFile = tt.clientCAFile
			tlsMinVersion = tt.minVersion
			defer func() {
				cert = origCert
				key = origKey
				clientCAFile = origClientCAFile
				tlsMinVersion = origMinVersion
			}()
			hook.Reset()

			_, err := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true).
				Build()
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}

			warned := false
			for _, entry := range hook.AllEntries() {
				if entry.Level == logrus.WarnLevel && strings.Contains(entry.Message, "have no effect because HTTPS is not enabled") {
					warned = true
				}
			}
			assert.Equal(t, tt.expectedWarning, warned)
		})
	}
}

func TestBuild_PartialTLSConfigurationDoesNotModifyTheBuilder(t *testing.T) {
	origCert := cert
	cert = "tls.crt"
	defer func() {
		cert = origCert
	}()

	builder := NewBuilder().
		APIRegistration(&mockRegister{}).
		MetricNamespace("test").
		PrometheusRegisterer(prometheus.NewRegistry())
	_, err := builder.Build()
	require.Error(t, err)

	// The configuration is rejected before the Builder is modified, so it can be built once the configuration is fixed.
	cert = ""
	_, err = builder.Build()
	require.NoError(t, err)
}

// runServer runs the server built by the builder until the end of the test, when it is stopped and finalized.
func runServer(t *testing.T, builder *Builder) {
	t.Helper()
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
}

// startServer runs the server built by the builder on a random port until the end of the test, and returns its address,
// read from the logs.
func startServer(t *testing.T, hook *logrustest.Hook, builder *Builder) string {
	t.Helper()
	builder.ListenAddress("127.0.0.1:0")
	runServer(t, builder)
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
