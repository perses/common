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

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateCertificate returns a self-signed certificate for 127.0.0.1 and its key, both PEM encoded.
// The serial number is used to identify the certificate served.
// It can be used by a server or by a client (mTLS), and trusted directly as the client CA, as it is self-signed.
func generateCertificate(t *testing.T, serialNumber int64) ([]byte, []byte) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serialNumber),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
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

func servedSerialNumber(t *testing.T, r *fileReloader[*tls.Certificate]) int64 {
	t.Helper()
	leaf, err := x509.ParseCertificate(r.get().Certificate[0])
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

	// The files are removed: the previous certificate is kept, and the error is logged only once.
	origHooks := logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	defer logrus.StandardLogger().ReplaceHooks(origHooks)
	hook := logrustest.NewLocal(logrus.StandardLogger())
	require.NoError(t, os.Remove(certFile))
	require.NoError(t, os.Remove(keyFile))
	assert.Equal(t, int64(4), servedSerialNumber(t, r))
	assert.Equal(t, int64(4), servedSerialNumber(t, r))
	assert.Len(t, hook.AllEntries(), 1)

	// The files are back: they are loaded again.
	writeCertificate(t, dir, 5)
	assert.Equal(t, int64(5), servedSerialNumber(t, r))
}

func TestNewClientCAReloader(t *testing.T) {
	dir := t.TempDir()
	notPEMFile := filepath.Join(dir, "ca.txt")
	replaceFile(t, notPEMFile, []byte("not a certificate"))

	_, err := newClientCAReloader(filepath.Join(dir, "missing.crt"))
	assert.ErrorContains(t, err, "no such file or directory")
	_, err = newClientCAReloader(notPEMFile)
	assert.ErrorContains(t, err, "no certificate found in "+notPEMFile)
}
