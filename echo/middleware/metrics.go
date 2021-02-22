package middleware

import (
	"fmt"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	labelCode    = "code"
	labelHandler = "handler"
	labelMethod  = "method"
)

// Metrics provides a way to monitor an API with a middleware to use
type Metrics struct {
	totalHTTPRequest    *prometheus.CounterVec
	durationHTTPRequest *prometheus.SummaryVec
}

func NewMetrics(namespace string) (*Metrics, error) {
	if len(namespace) == 0 {
		return nil, fmt.Errorf("namespace cannot be empty")
	}
	return &Metrics{
		totalHTTPRequest: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_request_total",
			Help:      "Total of HTTP requests that received the API",
		}, []string{labelCode, labelHandler, labelMethod}),
		durationHTTPRequest: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "http_request_duration_second",
			Help:      "Http request latencies in second",
		}, []string{labelHandler, labelMethod}),
	}, nil
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	m.totalHTTPRequest.Collect(ch)
	m.durationHTTPRequest.Collect(ch)
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	m.totalHTTPRequest.Describe(ch)
	m.durationHTTPRequest.Describe(ch)
}

// ProcessHTTPRequest is an echo middleware. It will intercept all responses.
// It will increase the metrics that count the number of HTTP request and calculate the time took to respond.
func (m *Metrics) ProcessHTTPRequest(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		start := time.Now()
		if err := next(ctx); err != nil {
			// Note: if this method is called, the code won't go further.
			ctx.Error(err)
		}
		elapsedTime := time.Since(start).Seconds()

		status := strconv.Itoa(ctx.Response().Status)
		counter, err := m.totalHTTPRequest.GetMetricWith(prometheus.Labels{labelCode: status, labelHandler: ctx.Path(), labelMethod: ctx.Request().Method})
		if err != nil {
			logrus.WithError(err).Error("unable to get the counter metrics in the api monitoring")
			// maybe not a really smart choice, but for the moment let's not impact the business if the monitoring somehow failed (which will unlikely happen)
			return nil
		}
		counter.Inc()
		sum, err := m.durationHTTPRequest.GetMetricWith(prometheus.Labels{labelHandler: ctx.Path(), labelMethod: ctx.Request().Method})
		if err != nil {
			logrus.WithError(err).Error("unable to get the summary metrics in the api monitoring")
			// maybe not a really smart choice, but for the moment let's not impact the business if the monitoring somehow failed (which will unlikely happen)
			return nil
		}
		sum.Observe(elapsedTime)
		return nil
	}
}
