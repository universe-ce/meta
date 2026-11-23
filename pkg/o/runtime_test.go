package o

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdk "go.opentelemetry.io/otel/sdk/trace"
)

type exporter struct {
	mu        sync.Mutex
	spans     []sdk.ReadOnlySpan
	shutdowns int
	exportErr error
}

func (e *exporter) ExportSpans(_ context.Context, spans []sdk.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return e.exportErr
}
func (e *exporter) Shutdown(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdowns++
	return nil
}

func TestRuntimePropagationAndOwnership(t *testing.T) {
	global := otel.GetTracerProvider()
	e := &exporter{}
	r, err := New("foundation-test", e)
	if err != nil {
		t.Fatal(err)
	}
	for _, headers := range []propagation.MapCarrier{
		{"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{"x-b3-traceid": "4bf92f3577b34da6a3ce929d0e0e4736", "x-b3-spanid": "00f067aa0ba902b7", "x-b3-sampled": "1"},
	} {
		ctx, span := r.Start(context.Background(), headers)
		_, child := r.TracerProvider().Tracer("child").Start(ctx, "child")
		child.End()
		span.End()
	}
	if err := r.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	if len(e.spans) != 4 {
		t.Fatalf("%d spans", len(e.spans))
	}
	for _, span := range e.spans {
		if span.SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Error("parent trace lost")
		}
		if span.Name() == "request" && span.Parent().SpanID().String() != "00f067aa0ba902b7" {
			t.Error("remote parent lost")
		}
	}
	e.mu.Unlock()
	if otel.GetTracerProvider() != global {
		t.Error("explicit runtime mutated global provider")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(r.Shutdown(ctx), context.Canceled) {
		t.Error("cancelled shutdown must report failure and allow retry")
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.shutdowns != 1 {
		t.Fatalf("exporter shutdowns %d", e.shutdowns)
	}
}

func TestRuntimeConcurrentRequestsAndShutdown(t *testing.T) {
	e := &exporter{}
	r, err := New("concurrent", e)
	if err != nil {
		t.Fatal(err)
	}
	const count = 80
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			_, span := r.Start(context.Background(), propagation.MapCarrier{})
			span.End()
		}()
	}
	group.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for range 5 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := r.Shutdown(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.spans) != count || e.shutdowns != 1 {
		t.Fatalf("spans=%d shutdowns=%d", len(e.spans), e.shutdowns)
	}
}

func TestRuntimeInvalidConfiguration(t *testing.T) {
	if _, err := New("invalid", nil); err == nil {
		t.Error("nil exporter accepted")
	}
	for _, endpoint := range []string{"", "://", "file:///tmp/trace", "relative/path", "http://"} {
		if _, err := NewJaeger("invalid", endpoint); err == nil {
			t.Errorf("invalid endpoint %q accepted", endpoint)
		}
	}
}

type slowShutdownExporter struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (*slowShutdownExporter) ExportSpans(context.Context, []sdk.ReadOnlySpan) error { return nil }
func (e *slowShutdownExporter) Shutdown(context.Context) error {
	close(e.started)
	<-e.release
	return e.err
}

func TestShutdownRetryWaitsForActualCompletion(t *testing.T) {
	failure := errors.New("exporter shutdown failed")
	e := &slowShutdownExporter{started: make(chan struct{}), release: make(chan struct{}), err: failure}
	r, err := New("slow", e)
	if err != nil {
		t.Fatal(err)
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(e.release) }) }
	defer release()
	firstCtx, firstCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer firstCancel()
	first := make(chan error, 1)
	go func() { first <- r.Shutdown(firstCtx) }()
	select {
	case <-e.started:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown not started")
	}
	if err := <-first; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("first wait: %v", err)
	}
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer secondCancel()
	if err := r.Shutdown(secondCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("retry returned before exporter finished: %v", err)
	}
	release()
	if err := r.Shutdown(context.Background()); !errors.Is(err, failure) {
		t.Errorf("actual shutdown result lost: %v", err)
	}
	if err := r.Shutdown(context.Background()); !errors.Is(err, failure) {
		t.Errorf("repeated shutdown result lost: %v", err)
	}
}
