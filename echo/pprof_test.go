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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var registerCustomHandlerOnce sync.Once

func TestPprof(t *testing.T) {
	// A handler registered on http.DefaultServeMux must never be exposed through the pprof endpoints.
	registerCustomHandlerOnce.Do(func() {
		http.HandleFunc("GET /debug/pprof/custom-handler", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	tests := []struct {
		name     string
		activate *bool
		path     string
		expected int
	}{
		{name: "disabled by default", path: "/debug/pprof", expected: http.StatusNotFound},
		{name: "disabled by default, named profile", path: "/debug/pprof/heap", expected: http.StatusNotFound},
		{name: "disabled by default, cmdline", path: "/debug/pprof/cmdline", expected: http.StatusNotFound},
		{name: "index", activate: new(true), path: "/debug/pprof", expected: http.StatusOK},
		{name: "named profile", activate: new(true), path: "/debug/pprof/heap", expected: http.StatusOK},
		{name: "cmdline", activate: new(true), path: "/debug/pprof/cmdline", expected: http.StatusOK},
		{name: "symbol", activate: new(true), path: "/debug/pprof/symbol", expected: http.StatusOK},
		{name: "unknown profile", activate: new(true), path: "/debug/pprof/unknown", expected: http.StatusNotFound},
		{name: "handler registered on http.DefaultServeMux", activate: new(true), path: "/debug/pprof/custom-handler", expected: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().
				APIRegistration(&mockRegister{}).
				OverrideDefaultMiddleware(true)
			if tt.activate != nil {
				builder.ActivatePprof(*tt.activate)
			}
			handler, err := builder.BuildHandler()
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			assert.Equal(t, tt.expected, rec.Code)
		})
	}
}
