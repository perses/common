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

package otel

import (
	"context"
	"fmt"

	"github.com/perses/common/async"
	"github.com/prometheus/common/version"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

type Builder struct {
	serviceName string
	resource    *resource.Resource
	exporter    trace.SpanExporter
	provider    *trace.TracerProvider
	err         error
}

func NewBuilder(serviceName string) *Builder {
	return &Builder{
		serviceName: serviceName,
	}
}

func (b *Builder) WithDefaultResource() *Builder {
	b.resource, b.err = b.createDefaultResource()
	return b
}

func (b *Builder) SetResource(r *resource.Resource) *Builder {
	b.resource = r
	return b
}

func (b *Builder) SetExporter(exp trace.SpanExporter) *Builder {
	b.exporter = exp
	return b
}

func (b *Builder) SetProvider(provider *trace.TracerProvider) *Builder {
	b.provider = provider
	return b
}

func (b *Builder) Build() (async.SimpleTask, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.provider != nil {
		return &provider{
			provider: b.provider,
		}, nil
	}
	if b.resource != nil {
		res, err := b.createDefaultResource()
		if err != nil {
			return nil, err
		}
		b.resource = res
	}
	b.provider = trace.NewTracerProvider(
		trace.WithBatcher(b.exporter),
		trace.WithResource(b.resource))
	return &provider{
		provider: b.provider,
	}, nil
}

func (b *Builder) createDefaultResource() (*resource.Resource, error) {
	if len(b.serviceName) == 0 {
		return nil, fmt.Errorf("otel serviceName not set")
	}
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(b.serviceName),
			semconv.ServiceVersionKey.String(version.Version)))
}

type provider struct {
	async.SimpleTask
	provider *trace.TracerProvider
}

func (p *provider) String() string {
	return "otel provider"
}

func (p *provider) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
	// start provider
	providerCtx, providerCancelFunc := context.WithCancel(ctx)
	go func() {
		defer providerCancelFunc()
		if err := p.provider.Shutdown(providerCtx); err != nil {
			logrus.WithError(err).Info("http server stopped")
		}
		logrus.Debug("go routine running the http server has been stopped.")
	}()
	// Wait for the end of the task or cancellation
	select {
	case <-providerCtx.Done():
		// OTeL provider ended unexpectedly
		// In our ecosystem, we don't want to continue if the traces cannot be sent to the server. So we prefer to shut down the whole system.
		// That's why we are calling the parent cancelFunc
		cancelFunc()
		// as it is possible that the serverCtx.Done() is closed because the main cancelFunc() has been called by another go routing,
		// we should try to close properly the http server
		// Note: that's why we don't return any error here.
	case <-ctx.Done():
		// Cancellation requested by the parent context
		logrus.Debug("provider cancellation requested")
	}
	return nil
}
