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
	"time"

	"github.com/perses/common/async"
	"github.com/prometheus/common/version"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
)

type Builder struct {
	resource *resource.Resource
	exporter trace.SpanExporter
	provider *trace.TracerProvider
	err      error
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithDefaultResource(serviceName string) *Builder {
	b.resource, b.err = b.createDefaultResource(serviceName)
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

func (b *Builder) Build() (async.Task, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.provider != nil {
		return &provider{provider: b.provider}, nil
	}
	if b.resource == nil {
		return nil, fmt.Errorf("otel resource is empty, use the default one or set one")
	}
	otelProvider := trace.NewTracerProvider(
		trace.WithBatcher(b.exporter),
		trace.WithResource(b.resource))
	return &provider{provider: otelProvider}, nil
}

func (b *Builder) createDefaultResource(serviceName string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version.Version)))
}

type otelErrHandler func(err error)

func (o otelErrHandler) Handle(err error) {
	o(err)
}

type provider struct {
	async.Task
	provider *trace.TracerProvider
}

func (p *provider) String() string {
	return "otel provider"
}

func (p *provider) Initialize() error {
	return nil
}

func (p *provider) Execute(ctx context.Context, _ context.CancelFunc) error {
	// start provider
	otel.SetTracerProvider(p.provider)
	otel.SetErrorHandler(otelErrHandler(func(err error) {
		logrus.WithError(err).Error("OpenTelemetry handler returned an error")
	}))
	<-ctx.Done()
	return nil
}

func (p *provider) Finalize() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.provider.Shutdown(ctx)
}
