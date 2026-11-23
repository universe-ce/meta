package mw

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	"github.com/universe-ce/meta/v3/pkg/o"
	"go.opentelemetry.io/otel"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTracingRequestLifecycle(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); w.WriteHeader(202) }))
	defer collector.Close()
	previous := otel.GetTracerProvider()
	defer otel.SetTracerProvider(previous)
	previousPropagator := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(previousPropagator)
	app := iris.New()
	app.Use(TracingHandler(true, "foundation-lifecycle", collector.URL))
	var spans []trace.Span
	var providers []trace.TracerProvider
	type contextKey struct{}
	parentID := "4bf92f3577b34da6a3ce929d0e0e4736"
	app.Post("/", func(c iris.Context) {
		span := trace.SpanFromContext(c.Request().Context())
		spans = append(spans, span)
		providers = append(providers, otel.GetTracerProvider())
		if !span.IsRecording() {
			t.Error("downstream request has no recording span")
		}
		if span.SpanContext().TraceID().String() != parentID {
			t.Error("incoming trace parent lost")
		}
		if c.Request().Context().Value(contextKey{}) != "keep" {
			t.Error("request context values lost")
		}
		if c.Request().Context().Err() != context.Canceled {
			t.Error("request cancellation lost")
		}
		b, _ := io.ReadAll(c.Request().Body)
		_, _ = c.Write(b)
	})
	build(t, app)
	for range 2 {
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "keep"))
		cancel()
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{"hello":true}`)).WithContext(ctx)
		req.Header.Set("traceparent", "00-"+parentID+"-00f067aa0ba902b7-01")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		if w.Body.String() != `{"hello":true}` {
			t.Fatal("request body changed")
		}
	}
	if providers[0] != providers[1] {
		t.Error("provider recreated for the same configuration")
	}
	for _, span := range spans {
		if span.IsRecording() {
			t.Error("request span did not end")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ShutdownTracing(ctx); err != nil {
		t.Error(err)
	}
}

func TestTracingPanicEndsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime, err := o.New("panic-test", exporter)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown(context.Background())
	app := iris.New()
	diagnostics := captureTracingDiagnostics(t, app.Logger())
	app.Use(ErrorsHandler(), TracingWithRuntime(runtime))
	app.Get("/", func(c iris.Context) { panic("fixture") })
	build(t, app)
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if err := runtime.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "request" || spans[0].EndTime.Before(spans[0].StartTime) {
		t.Fatalf("panic request spans: %+v", spans)
	}
	if output := diagnostics.String(); strings.Count(output, "[WARN]") != 1 ||
		!strings.Contains(output, "fixture") || strings.Contains(output, "[ERRO]") {
		t.Fatalf("unexpected panic diagnostics: %q", output)
	}
}

func TestTracingHandlersShareConfiguration(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); w.WriteHeader(202) }))
	defer collector.Close()
	previous, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	defer otel.SetTracerProvider(previous)
	defer otel.SetTextMapPropagator(previousPropagator)
	first := TracingHandler(true, "shared-test", collector.URL)
	provider := otel.GetTracerProvider()
	second := TracingHandler(true, "shared-test", collector.URL)
	if provider != otel.GetTracerProvider() {
		t.Fatal("same configuration created another provider")
	}
	app := iris.New()
	app.Get("/one", first, func(c iris.Context) { _, _ = c.WriteString("one") })
	app.Get("/two", second, func(c iris.Context) { _, _ = c.WriteString("two") })
	build(t, app)
	for _, path := range []string{"/one", "/two"} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Body.String() != strings.TrimPrefix(path, "/") {
			t.Fatal(w.Body)
		}
	}
	if err := ShutdownTracing(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ShutdownTracing(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTracingFallbackDoesNotCreateProvider(t *testing.T) {
	diagnostics := captureTracingDiagnostics(t, log.Default)
	previous := otel.GetTracerProvider()
	for _, handler := range []iris.Handler{TracingHandler(false, "disabled", "bad"), TracingHandler(true, "invalid", "://"), TracingWithRuntime(nil)} {
		app := iris.New()
		app.Get("/", handler, func(c iris.Context) { _, _ = c.WriteString("ok") })
		build(t, app)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Body.String() != "ok" || otel.GetTracerProvider() != previous {
			t.Fatal("fallback changed request/provider")
		}
	}
	if output := diagnostics.String(); strings.Count(output, "[ERRO]") != 1 ||
		!strings.Contains(output, "OpenTracing connect is error") || strings.Contains(output, "[WARN]") {
		t.Fatalf("unexpected fallback diagnostics: %q", output)
	}
}

func captureTracingDiagnostics(t *testing.T, logger *log.Logger) *bytes.Buffer {
	t.Helper()
	// These fixtures are serial: preserve all destinations and the enabled level.
	output, levels, level := logger.Printer.Output, logger.LevelOutput, logger.Level
	var captured bytes.Buffer
	logger.LevelOutput = nil
	logger.SetOutput(&captured)
	logger.SetLevel("info")
	t.Cleanup(func() {
		logger.SetOutput(output)
		logger.LevelOutput, logger.Level = levels, level
	})
	return &captured
}

// Compile-time check of the public provider port without exposing an Iris type.
var _ sdk.SpanExporter = (*tracetest.InMemoryExporter)(nil)
