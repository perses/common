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
	"crypto/x509"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/sirupsen/logrus"
)

// fileReloader provides a value loaded from files, like the certificate used by the HTTPS server.
// On every TLS handshake, it checks if the files changed and loads them again if it is the case.
// Like that, a rotated certificate is used for the new connections without restarting the server.
type fileReloader[T any] struct {
	// name describes the value in the logs (e.g., "TLS certificate").
	name  string
	files []string
	load  func() (T, error)
	mutex sync.Mutex
	value T
	infos []os.FileInfo
	// statFailed is true when the files could not be checked during the previous handshake.
	// It is used to log the error only once instead of on every handshake.
	statFailed bool
}

// newFileReloader loads the value a first time.
// It returns an error if it cannot be loaded, so the server doesn't start with an invalid configuration.
func newFileReloader[T any](name string, load func() (T, error), files ...string) (*fileReloader[T], error) {
	r := &fileReloader[T]{name: name, files: files, load: load}
	infos, err := r.stat()
	if err != nil {
		return nil, err
	}
	r.infos = infos
	if r.value, err = load(); err != nil {
		return nil, err
	}
	return r, nil
}

// get returns the value, loaded again if the files changed.
// When the files cannot be loaded again, the error is logged and the previous value is kept.
func (r *fileReloader[T]) get() T {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	infos, err := r.stat()
	if err != nil {
		if !r.statFailed {
			logrus.WithError(err).Errorf("unable to check if the %s changed, the previous one is used", r.name)
			r.statFailed = true
		}
		return r.value
	}
	r.statFailed = false
	if slices.EqualFunc(r.infos, infos, isSameFile) {
		return r.value
	}
	// The files are tracked even if they are invalid, so they are loaded again only once they change.
	// For example, it happens when the certificate has been replaced but not yet the key.
	r.infos = infos
	value, err := r.load()
	if err != nil {
		logrus.WithError(err).Errorf("unable to reload the %s, the previous one is used", r.name)
		return r.value
	}
	r.value = value
	logrus.Infof("%s reloaded", r.name)
	return r.value
}

func (r *fileReloader[T]) stat() ([]os.FileInfo, error) {
	infos := make([]os.FileInfo, 0, len(r.files))
	for _, file := range r.files {
		info, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// newCertificateReloader provides the certificate used by the HTTPS server, reloaded when the files change.
func newCertificateReloader(certFile, keyFile string) (*fileReloader[*tls.Certificate], error) {
	return newFileReloader("TLS certificate", func() (*tls.Certificate, error) {
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		return &certificate, nil
	}, certFile, keyFile)
}

// newClientCAReloader provides the CA certificates used to verify the client certificates (mTLS), reloaded when the
// file changes.
func newClientCAReloader(caFile string) (*fileReloader[*x509.CertPool], error) {
	return newFileReloader("TLS client CA", func() (*x509.CertPool, error) {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		clientCAs := x509.NewCertPool()
		if !clientCAs.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("no certificate found in %s", caFile)
		}
		return clientCAs, nil
	}, caFile)
}

// isSameFile tells if the file described by newInfo is the one described by oldInfo and if it was not modified since.
// A file replaced by another one is detected even if both have the same modification time and size,
// for example when a Kubernetes Secret volume is updated.
func isSameFile(oldInfo, newInfo os.FileInfo) bool {
	return os.SameFile(oldInfo, newInfo) && oldInfo.ModTime().Equal(newInfo.ModTime()) && oldInfo.Size() == newInfo.Size()
}
