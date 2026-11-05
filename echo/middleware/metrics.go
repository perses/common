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

package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
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
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests received by the API.",
		}, []string{labelCode, labelHandler, labelMethod}),
		durationHTTPRequest: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latencies in seconds.",
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
// The error returned by the next handler is returned as is, to be handled by the central error handler of echo.
// The status code counted is the one of the response that will be sent, resolved from this error.
func (m *Metrics) ProcessHTTPRequest(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx *echo.Context) error {
		start := time.Now()
		err := next(ctx)
		elapsedTime := time.Since(start).Seconds()
		method := ctx.Request().Method
		if method != http.MethodPost &&
			method != http.MethodGet &&
			method != http.MethodHead &&
			method != http.MethodPatch &&
			method != http.MethodPut &&
			method != http.MethodConnect &&
			method != http.MethodDelete &&
			method != http.MethodOptions &&
			method != http.MethodTrace {
			logrus.Tracef("weird http method used %q", method)
			method = "not_standard"
		}

		_, code := echo.ResolveResponseStatus(ctx.Response(), err)
		status := strconv.Itoa(code)
		counter, metricErr := m.totalHTTPRequest.GetMetricWith(prometheus.Labels{labelCode: status, labelHandler: ctx.Path(), labelMethod: method})
		if metricErr != nil {
			logrus.WithError(metricErr).Error("unable to get the counter metrics in the api monitoring")
			// maybe not a really smart choice, but for the moment let's not impact the business if the monitoring somehow failed (which will unlikely happen)
			return err
		}
		counter.Inc()
		sum, metricErr := m.durationHTTPRequest.GetMetricWith(prometheus.Labels{labelHandler: ctx.Path(), labelMethod: method})
		if metricErr != nil {
			logrus.WithError(metricErr).Error("unable to get the summary metrics in the api monitoring")
			// maybe not a really smart choice, but for the moment let's not impact the business if the monitoring somehow failed (which will unlikely happen)
			return err
		}
		sum.Observe(elapsedTime)
		return err
	}
}
