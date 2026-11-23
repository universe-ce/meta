// Package o owns OpenTelemetry tracing resources independently of any HTTP framework.
package o

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

type Runtime struct {
	provider     *sdk.TracerProvider
	tracer       trace.Tracer
	propagator   propagation.TextMapPropagator
	shutdownOnce sync.Once
	shutdownDone chan struct{}
	shutdownErr  error
}

// New takes ownership of exporter. Construct once in the composition root and
// call Shutdown after stopping ingress and draining requests.
// It does not change the process-wide OpenTelemetry globals.
func New(service string, exporter sdk.SpanExporter) (*Runtime, error) {
	if exporter == nil {
		return nil, errors.New("telemetry: exporter is required")
	}
	provider := sdk.NewTracerProvider(
		sdk.WithBatcher(exporter),
		sdk.WithResource(resource.NewSchemaless(semconv.ServiceNameKey.String(service))),
	)
	return &Runtime{
		provider:     provider,
		tracer:       provider.Tracer(service),
		shutdownDone: make(chan struct{}),
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
			b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader)),
		),
	}, nil
}

// NewJaeger retains the existing collector protocol while making ownership explicit.
func NewJaeger(service, endpoint string) (*Runtime, error) {
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("telemetry: invalid collector endpoint")
	}
	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)))
	if err != nil {
		return nil, err
	}
	return New(service, exporter)
}

func (r *Runtime) Start(ctx context.Context, carrier propagation.TextMapCarrier) (context.Context, trace.Span) {
	parent := r.propagator.Extract(ctx, carrier)
	return r.tracer.Start(parent, "request", trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("type", "zce-opentracing")))
}

func (r *Runtime) TracerProvider() trace.TracerProvider      { return r.provider }
func (r *Runtime) Propagator() propagation.TextMapPropagator { return r.propagator }
func (r *Runtime) ForceFlush(ctx context.Context) error      { return r.provider.ForceFlush(ctx) }

// Shutdown starts cleanup once and waits for its actual completion. A caller's
// deadline bounds its wait, not the shared cleanup; later callers can keep
// waiting and receive the same final result. The SDK must not receive a cancelled
// caller context: it marks itself closed before completing exporter cleanup.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.shutdownOnce.Do(func() {
		go func() {
			r.shutdownErr = r.provider.Shutdown(context.Background())
			close(r.shutdownDone)
		}()
	})
	select {
	case <-r.shutdownDone:
		return r.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
