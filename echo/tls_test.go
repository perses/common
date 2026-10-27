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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateCertificate returns a self-signed certificate for 127.0.0.1 and its key, both PEM encoded.
// The serial number is used to identify the certificate served.
func generateCertificate(t *testing.T, serialNumber int64) ([]byte, []byte) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serialNumber),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

// replaceFile replaces the file by a new one, like it is done when a certificate is rotated.
func replaceFile(t *testing.T, path string, content []byte) {
	t.Helper()
	tmpPath := path + ".tmp"
	require.NoError(t, os.WriteFile(tmpPath, content, 0600))
	require.NoError(t, os.Rename(tmpPath, path))
}

// overwriteFile writes the new content in the existing file and changes its modification time.
func overwriteFile(t *testing.T, path string, content []byte, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0600))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}

func writeCertificate(t *testing.T, dir string, serialNumber int64) (string, string) {
	t.Helper()
	certPEM, keyPEM := generateCertificate(t, serialNumber)
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	replaceFile(t, certFile, certPEM)
	replaceFile(t, keyFile, keyPEM)
	return certFile, keyFile
}

func servedSerialNumber(t *testing.T, r *certificateReloader) int64 {
	t.Helper()
	certificate, err := r.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	require.NoError(t, err)
	return leaf.SerialNumber.Int64()
}

func TestNewCertificateReloader(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertificate(t, dir, 1)
	_, otherKeyPEM := generateCertificate(t, 2)
	otherKeyFile := filepath.Join(dir, "other.key")
	replaceFile(t, otherKeyFile, otherKeyPEM)

	tests := []struct {
		name     string
		certFile string
		keyFile  string
		errorMsg string
	}{
		{
			name:     "missing certificate",
			certFile: filepath.Join(dir, "missing.crt"),
			keyFile:  keyFile,
			errorMsg: "no such file or directory",
		},
		{
			name:     "missing key",
			certFile: certFile,
			keyFile:  filepath.Join(dir, "missing.key"),
			errorMsg: "no such file or directory",
		},
		{
			name:     "key not matching the certificate",
			certFile: certFile,
			keyFile:  otherKeyFile,
			errorMsg: "private key does not match public key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newCertificateReloader(tt.certFile, tt.keyFile)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestCertificateReloader_GetCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertificate(t, dir, 1)
	r, err := newCertificateReloader(certFile, keyFile)
	require.NoError(t, err)
	assert.Equal(t, int64(1), servedSerialNumber(t, r))

	// The files are replaced by new ones.
	writeCertificate(t, dir, 2)
	assert.Equal(t, int64(2), servedSerialNumber(t, r))

	// The content of the files is modified.
	certPEM, keyPEM := generateCertificate(t, 3)
	modTime := time.Now().Add(time.Minute)
	overwriteFile(t, certFile, certPEM, modTime)
	overwriteFile(t, keyFile, keyPEM, modTime)
	assert.Equal(t, int64(3), servedSerialNumber(t, r))

	// The certificate is replaced, but not yet the key: the previous certificate is kept.
	certPEM, keyPEM = generateCertificate(t, 4)
	replaceFile(t, certFile, certPEM)
	assert.Equal(t, int64(3), servedSerialNumber(t, r))
	// Then the key is replaced as well.
	replaceFile(t, keyFile, keyPEM)
	assert.Equal(t, int64(4), servedSerialNumber(t, r))

	// The files are removed: the previous certificate is kept.
	require.NoError(t, os.Remove(certFile))
	require.NoError(t, os.Remove(keyFile))
	assert.Equal(t, int64(4), servedSerialNumber(t, r))
}

func TestServer_TLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeCertificate(t, dir, 1)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	origMinVersion := tlsMinVersion
	origCert := cert
	origKey := key
	tlsMinVersion = "1.3"
	cert = certFile
	key = keyFile
	defer func() {
		tlsMinVersion = origMinVersion
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
	defer func() {
		cancel()
		<-done
		require.NoError(t, task.Finalize())
	}()

	dialServer := func(maxVersion uint16) (tls.ConnectionState, error) {
		conn, dialErr := tls.Dial("tcp", address, &tls.Config{
			// The certificates are self-signed, and the test only checks which one is served.
			InsecureSkipVerify: true,
			MaxVersion:         maxVersion,
			NextProtos:         []string{"h2", "http/1.1"},
		})
		if dialErr != nil {
			return tls.ConnectionState{}, dialErr
		}
		defer func() {
			_ = conn.Close()
		}()
		return conn.ConnectionState(), nil
	}

	require.Eventually(t, func() bool {
		_, dialErr := dialServer(0)
		return dialErr == nil
	}, 5*time.Second, 10*time.Millisecond)

	state, err := dialServer(0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), state.PeerCertificates[0].SerialNumber.Int64())
	assert.Equal(t, "h2", state.NegotiatedProtocol)

	// The TLS min version is applied.
	_, err = dialServer(tls.VersionTLS12)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocol version not supported")

	// The certificate is rotated: the new connections get the new one.
	writeCertificate(t, dir, 2)
	state, err = dialServer(0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), state.PeerCertificates[0].SerialNumber.Int64())
}
