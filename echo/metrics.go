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
	"flag"

	"github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultTelemetryPath = "/metrics"

var (
	// http path for telemetry exposition
	telemetryPath string
)

func InitTelemetryPathFlag() {
	flag.StringVar(&telemetryPath, "web.telemetry-path", defaultTelemetryPath, "Path under which to expose metrics.")
}

// NewMetricsAPI returns the API exposing the metrics collected by the gatherer, under the path set by the flag
// --web.telemetry-path ("/metrics" by default, see InitTelemetryPathFlag).
// The metrics about the requests of this endpoint are registered with the registerer (the default Prometheus registerer
// if nil). Set disableCompression to true when the responses are already compressed by a middleware, like the gzip one
// used by default by the Builder.
func NewMetricsAPI(disableCompression bool, r prometheus.Registerer, gatherer prometheus.Gatherer) Register {
	return &metrics{
		disableCompression: disableCompression,
		promRegisterer:     r,
		promGatherer:       gatherer}
}

// metrics is a struct than handles the endpoint /metrics
// It should be used through the Builder like that: Builder.APIRegistration(NewMetricsAPI(true, registerer, gatherer))
type metrics struct {
	Register
	// disableCompression should be used if you are using the gzip middleware at a higher level (meaning that the endpoint /metrics is going to be compressed by the middleware).
	// The following issues give a bit more context:
	// * https://github.com/prometheus/prometheus/issues/5085
	// * https://github.com/prometheus/client_golang/issues/622g
	disableCompression bool
	promRegisterer     prometheus.Registerer
	promGatherer       prometheus.Gatherer
}

func (m *metrics) RegisterRoute(e *echo.Echo) {
	if m.promRegisterer == nil {
		m.promRegisterer = prometheus.DefaultRegisterer
	}
	if len(telemetryPath) == 0 {
		telemetryPath = defaultTelemetryPath
	}
	e.GET(telemetryPath, echo.WrapHandler(
		promhttp.InstrumentMetricHandler(
			m.promRegisterer, promhttp.HandlerFor(
				m.promGatherer, promhttp.HandlerOpts{
					DisableCompression: m.disableCompression,
				},
			),
		),
	))
}
