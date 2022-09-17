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
	"fmt"

	"github.com/prometheus/common/version"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
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

func (b *Builder) Build() (*trace.TracerProvider, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.provider != nil {
		return b.provider, nil
	}
	if b.resource == nil {
		return nil, fmt.Errorf("otel resource is empty, use the default one or set one")
	}
	b.provider = trace.NewTracerProvider(
		trace.WithBatcher(b.exporter),
		trace.WithResource(b.resource))
	return b.provider, nil
}

func (b *Builder) createDefaultResource(serviceName string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version.Version)))
}
