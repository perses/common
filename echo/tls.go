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
	"crypto/tls"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// certificateReloader provides the certificate used by the HTTPS server.
// On every TLS handshake, it checks if the certificate or the key file changed and loads them again if it is the case.
// Like that, a rotated certificate is used for the new connections without restarting the server.
type certificateReloader struct {
	certFile    string
	keyFile     string
	mutex       sync.Mutex
	certificate *tls.Certificate
	certInfo    os.FileInfo
	keyInfo     os.FileInfo
}

// newCertificateReloader loads the certificate and the key a first time.
// It returns an error if they cannot be loaded, so the server doesn't start with an invalid certificate.
func newCertificateReloader(certFile, keyFile string) (*certificateReloader, error) {
	r := &certificateReloader{
		certFile: certFile,
		keyFile:  keyFile,
	}
	certInfo, keyInfo, err := r.stat()
	if err != nil {
		return nil, err
	}
	r.certInfo = certInfo
	r.keyInfo = keyInfo
	if err = r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate is used as tls.Config.GetCertificate.
// When the certificate or the key cannot be loaded again, the error is logged and the previous certificate is kept.
func (r *certificateReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	certInfo, keyInfo, err := r.stat()
	if err != nil {
		logrus.WithError(err).Error("unable to check if the TLS certificate changed, the previous one is used")
		return r.certificate, nil
	}
	if isSameFile(r.certInfo, certInfo) && isSameFile(r.keyInfo, keyInfo) {
		return r.certificate, nil
	}
	// The files are tracked even if they are invalid, so they are loaded again only once they change.
	// For example, it happens when the certificate has been replaced but not yet the key.
	r.certInfo = certInfo
	r.keyInfo = keyInfo
	if err = r.load(); err != nil {
		logrus.WithError(err).Error("unable to reload the TLS certificate, the previous one is used")
		return r.certificate, nil
	}
	logrus.Info("TLS certificate reloaded")
	return r.certificate, nil
}

func (r *certificateReloader) stat() (os.FileInfo, os.FileInfo, error) {
	certInfo, err := os.Stat(r.certFile)
	if err != nil {
		return nil, nil, err
	}
	keyInfo, err := os.Stat(r.keyFile)
	if err != nil {
		return nil, nil, err
	}
	return certInfo, keyInfo, nil
}

func (r *certificateReloader) load() error {
	certificate, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return err
	}
	r.certificate = &certificate
	return nil
}

// isSameFile tells if the file described by newInfo is the one described by oldInfo and if it was not modified since.
// A file replaced by another one is detected even if both have the same modification time and size,
// for example when a Kubernetes Secret volume is updated.
func isSameFile(oldInfo, newInfo os.FileInfo) bool {
	return os.SameFile(oldInfo, newInfo) && oldInfo.ModTime().Equal(newInfo.ModTime()) && oldInfo.Size() == newInfo.Size()
}
