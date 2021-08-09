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

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// http path for telemetry exposition
	telemetryPath string
)

func init() {
	flag.StringVar(&telemetryPath, "web.telemetry-path", "/metrics", "Path under which to expose metrics.")
}

func NewMetricsAPI(disableCompression bool) Register {
	return &metrics{disableCompression: disableCompression}
}

// metrics is a struct than handles the endpoint /metrics
// It should be used through the Builder like that: Builder.APIRegistration(NewMetricsAPI(true))
type metrics struct {
	Register
	// disableCompression should be used if you are using the gzip middleware at a higher level (meaning that the endpoint /metrics is going to be compression by the middleware).
	// The following issues give a bit more context:
	// * https://github.com/prometheus/prometheus/issues/5085
	// * https://github.com/prometheus/client_golang/issues/622g
	disableCompression bool
}

func (m *metrics) RegisterRoute(e *echo.Echo) {
	e.GET(telemetryPath, echo.WrapHandler(
		promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer, promhttp.HandlerOpts{
					DisableCompression: m.disableCompression,
				},
			),
		),
	))
}
