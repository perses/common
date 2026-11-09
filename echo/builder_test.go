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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type helloRegister struct{}

func (h *helloRegister) RegisterRoute(e *echo.Echo) {
	e.GET("/hello", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
}

func TestBuilder_BuildSeveralTimes(t *testing.T) {
	registry := prometheus.NewRegistry()
	builder := NewBuilder().
		APIRegistration(&helloRegister{}).
		MetricNamespace("test").
		PrometheusRegisterer(registry).
		Middleware(func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		})

	first, err := builder.build()
	require.NoError(t, err)
	second, err := builder.build()
	require.NoError(t, err)
	// The default middlewares are not added again.
	assert.Len(t, second.mdws, len(first.mdws))

	// The servers built from the same Builder share the same metrics.
	handler, err := builder.BuildHandler()
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hello", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, float64(1), requestCount(t, registry))
}

// requestCount returns the total of the HTTP requests counted by the metrics middleware.
func requestCount(t *testing.T, registry *prometheus.Registry) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	total := float64(0)
	for _, family := range families {
		if family.GetName() != "test_http_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}
	return total
}

func TestBuilder_MetricsRegistrationError(t *testing.T) {
	registry := prometheus.NewRegistry()
	// A different metric with the same name is already registered.
	registry.MustRegister(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "test",
		Name:      "http_requests_total",
		Help:      "Another metric",
	}))
	_, err := NewBuilder().
		APIRegistration(&helloRegister{}).
		MetricNamespace("test").
		PrometheusRegisterer(registry).
		Build()
	assert.Error(t, err)
}

type clientRegister struct{}

func (r *clientRegister) RegisterRoute(e *echo.Echo) {
	e.GET("/client", func(c *echo.Context) error {
		return c.String(http.StatusOK, c.Scheme()+" "+c.RealIP())
	})
}

func TestBuilder_IPAndSchemeExtractors(t *testing.T) {
	_, proxyNetwork, err := net.ParseCIDR("203.0.113.0/24")
	require.NoError(t, err)
	tests := []struct {
		name      string
		configure func(b *Builder)
		// expected is the scheme and the IP address of the client, as seen by the handler.
		expected string
	}{
		{
			// The forwarding headers can be spoofed by any client, so they are not trusted from a public address by default.
			name:      "default",
			configure: func(*Builder) {},
			expected:  "http 203.0.113.10",
		},
		{
			name: "trusted proxy",
			configure: func(b *Builder) {
				b.IPExtractor(echo.ExtractIPFromXFFHeader(echo.TrustIPRange(proxyNetwork))).
					SchemeExtractor(echo.ExtractSchemeFromHeaders(echo.TrustIPRange(proxyNetwork)))
			},
			expected: "https 198.51.100.7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().APIRegistration(&clientRegister{}).OverrideDefaultMiddleware(true)
			tt.configure(builder)
			handler, err := builder.BuildHandler()
			require.NoError(t, err)

			// The request comes from a proxy with a public address (like a cloud load balancer).
			req := httptest.NewRequest(http.MethodGet, "/client", nil)
			req.RemoteAddr = "203.0.113.10:1234"
			req.Header.Set(echo.HeaderXForwardedFor, "198.51.100.7")
			req.Header.Set(echo.HeaderXForwardedProto, "https")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.expected, rec.Body.String())
		})
	}
}
