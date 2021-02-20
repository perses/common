package echo

import (
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

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
	e.GET("/metrics", echo.WrapHandler(
		promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer, promhttp.HandlerOpts{
					DisableCompression: m.disableCompression,
				},
			),
		),
	))
}
