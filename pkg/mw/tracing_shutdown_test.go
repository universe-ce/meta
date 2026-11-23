package mw

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/core/host"
	"github.com/universe-ce/meta/v3/pkg/o"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type shutdownExporter struct{ started, release chan struct{} }

func (*shutdownExporter) ExportSpans(context.Context, []sdk.ReadOnlySpan) error { return nil }
func (e *shutdownExporter) Shutdown(context.Context) error                      { close(e.started); <-e.release; return nil }

func TestLegacyShutdownDeadlineWhileAnotherCallerWaits(t *testing.T) {
	e := &shutdownExporter{started: make(chan struct{}), release: make(chan struct{})}
	r, err := o.New("slow", e)
	if err != nil {
		t.Fatal(err)
	}
	key := tracingKey{service: "slow", endpoint: "test"}
	legacyTracing.Lock()
	legacyTracing.runtimes[key] = r
	legacyTracing.Unlock()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(e.release) }) }
	defer release()
	first := make(chan error, 1)
	go func() { first <- ShutdownTracing(context.Background()) }()
	select {
	case <-e.started:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() { second <- ShutdownTracing(ctx) }()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("concurrent shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("shutdown lock ignored caller deadline")
	}
	release()
	if err := <-first; err != nil {
		t.Error(err)
	}
	if err := ShutdownTracing(context.Background()); err != nil {
		t.Error(err)
	}
}

type testInterrupt struct{ callbacks []func() }

func (i *testInterrupt) Register(f func()) { i.callbacks = append(i.callbacks, f) }
func (i *testInterrupt) FireNow() {
	for _, f := range i.callbacks {
		f()
	}
}

type observedListener struct {
	net.Listener
	stopped chan struct{}
	once    sync.Once
}

func (l *observedListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.stopped) })
	return err
}

func TestInterruptDrainsHTTPBeforeTracing(t *testing.T) {
	testInterruptDrainsHTTP(t, false)
}

func TestInterruptDrainsFirstRequestBeforeTracingRegistration(t *testing.T) {
	testInterruptDrainsHTTP(t, true)
}

func testInterruptDrainsHTTP(t *testing.T, beforeTracing bool) {
	t.Helper()
	// Use the real Iris NewHost registration sequence, but never send an OS signal.
	interrupt := &testInterrupt{}
	previousInterrupt := host.Interrupt
	host.Interrupt = interrupt
	legacyTracingInterrupt = sync.Once{}
	defer func() { host.Interrupt = previousInterrupt; legacyTracingInterrupt = sync.Once{} }()
	previous, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	defer otel.SetTracerProvider(previous)
	defer otel.SetTextMapPropagator(previousPropagator)
	var exports atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		exports.Add(1)
		w.WriteHeader(202)
	}))
	defer collector.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	app := iris.New()
	if beforeTracing {
		app.Use(func(c iris.Context) { close(entered); <-release; c.Next() })
	}
	app.Use(TracingHandlerFor(app, true, "interrupt-test", collector.URL))
	app.Get("/", func(c iris.Context) {
		if !beforeTracing {
			close(entered)
			<-release
		}
		_, _ = c.WriteString("ok")
	})
	build(t, app)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedListener{Listener: listener, stopped: make(chan struct{})}
	server := app.NewHost(&http.Server{Handler: app})
	served := make(chan error, 1)
	go func() { served <- server.Serve(observed) }()
	clientDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String() + "/")
		if err == nil {
			b, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			err = readErr
			if string(b) != "ok" {
				err = errors.New("in-flight response changed")
			}
		}
		clientDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not received")
	}
	fired := make(chan struct{})
	go func() { interrupt.FireNow(); close(fired) }()
	select {
	case <-observed.stopped:
	case <-time.After(3 * time.Second):
		t.Error("interrupt did not stop ingress")
	}
	unblock()
	select {
	case err := <-clientDone:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not drain")
	}
	select {
	case <-fired:
	case <-time.After(3 * time.Second):
		t.Fatal("interrupt did not complete")
	}
	if exports.Load() != 1 {
		t.Errorf("in-flight request span lost: collector exports=%d", exports.Load())
	}
	<-served
}

func TestAutomaticShutdownLeavesManualRuntimeOwnedByCaller(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(202)
	}))
	defer collector.Close()
	previous, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	defer otel.SetTracerProvider(previous)
	defer otel.SetTextMapPropagator(previousPropagator)
	defer ShutdownTracing(context.Background())
	_ = TracingHandler(true, "ownership", collector.URL)
	manual := otel.GetTracerProvider()
	app := iris.New()
	_ = TracingHandlerFor(app, true, "ownership", collector.URL)
	automatic := otel.GetTracerProvider()
	_ = TracingHandlerFor(app, true, "ownership", collector.URL)
	if automatic != otel.GetTracerProvider() {
		t.Fatal("same application/configuration was not reused")
	}
	if manual == automatic {
		t.Fatal("manual and automatic lifecycle owners were merged")
	}
	if err := shutdownTracingApplications(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, span := manual.Tracer("manual").Start(context.Background(), "still-owned")
	if !span.IsRecording() {
		t.Error("automatic shutdown closed caller-owned runtime")
	}
	span.End()
	_, closed := automatic.Tracer("automatic").Start(context.Background(), "closed")
	if closed.IsRecording() {
		t.Error("bound runtime remained open")
	}
	closed.End()
}

func TestTracingDistinctApplicationsKeepInFlightSpanIndependent(t *testing.T) {
	isolateTracingApplicationFixture(t)
	collector, exports := tracingBoundaryCollector(t)
	first := startBlockedTracingApplication(t, "same-config", collector.URL)
	second := startBlockedTracingApplication(t, "same-config", collector.URL)
	if first.runtime == second.runtime {
		t.Fatal("distinct applications shared their lifecycle runtime")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	firstDrained := make(chan error, 1)
	go func() { firstDrained <- first.app.Shutdown(ctx) }()
	waitTracingBoundary(t, first.listener.stopped, "first application did not stop ingress")
	first.finishRequest(t)
	if err := <-firstDrained; err != nil {
		t.Fatal(err)
	}
	if err := shutdownTracingRuntimes(ctx, map[tracingKey]*o.Runtime{first.key: first.runtime}); err != nil {
		t.Fatal(err)
	}
	if exports.Load() != 1 {
		t.Fatalf("first application did not export its request: %d", exports.Load())
	}
	if !second.span.IsRecording() {
		t.Fatal("closing the first application ended the second in-flight span")
	}
	if err := second.runtime.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}
	if exports.Load() != 1 {
		t.Fatal("blocked second request exported before being released")
	}

	second.finishRequest(t)
	if err := second.app.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := shutdownTracingRuntimes(ctx, map[tracingKey]*o.Runtime{second.key: second.runtime}); err != nil {
		t.Fatal(err)
	}
	if exports.Load() != 2 {
		t.Fatalf("second application lost its request after first shutdown: %d", exports.Load())
	}
}

func TestTracingFailedApplicationDrainPreservesProvider(t *testing.T) {
	isolateTracingApplicationFixture(t)
	collector, exports := tracingBoundaryCollector(t)
	running := startBlockedTracingApplication(t, "failed-drain", collector.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	drained := make(chan error, 1)
	go func() { drained <- shutdownTracingApplications(ctx) }()
	waitTracingBoundary(t, running.listener.stopped, "shutdown did not stop ingress")
	select {
	case err := <-drained:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked drain must return deadline, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("blocked drain ignored deadline")
	}
	legacyTracing.Lock()
	retained := legacyTracing.runtimes[running.key] == running.runtime
	legacyTracing.Unlock()
	if !retained || exports.Load() != 0 || !running.span.IsRecording() {
		t.Fatal("failed drain removed runtime or ended/exported the blocked request")
	}
	// A new recording span proves the provider was not shut down on the error path.
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer flushCancel()
	_, probe := running.runtime.Start(flushCtx, propagation.MapCarrier{})
	recording := probe.IsRecording()
	probe.End()
	if !recording {
		t.Fatal("failed HTTP drain prematurely shut down the provider")
	}
	if err := running.runtime.ForceFlush(flushCtx); err != nil {
		t.Fatal(err)
	}
	if exports.Load() != 1 {
		t.Fatal("provider could not export after failed drain")
	}

	running.finishRequest(t)
	if err := running.app.Shutdown(flushCtx); err != nil {
		t.Fatal(err)
	}
	if err := ShutdownTracing(flushCtx); err != nil {
		t.Fatal(err)
	}
	if exports.Load() != 2 {
		t.Fatalf("request ending after failed drain lost its span: %d", exports.Load())
	}
}

func isolateTracingApplicationFixture(t *testing.T) {
	t.Helper()
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	previousInterrupt := host.Interrupt
	host.Interrupt = &testInterrupt{}
	legacyTracingInterrupt = sync.Once{}
	legacyTracing.Lock()
	previousRegistry := legacyTracing.runtimes
	legacyTracing.runtimes = make(map[tracingKey]*o.Runtime)
	legacyTracing.Unlock()
	t.Cleanup(func() {
		legacyTracing.Lock()
		if len(legacyTracing.runtimes) != 0 {
			t.Error("boundary fixture leaked registered runtimes")
		}
		legacyTracing.runtimes = previousRegistry
		legacyTracing.Unlock()
		host.Interrupt = previousInterrupt
		legacyTracingInterrupt = sync.Once{}
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
}

func tracingBoundaryCollector(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	exports := &atomic.Int32{}
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil || n == 0 {
			t.Error("collector received an empty or unreadable export")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		exports.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(collector.Close)
	return collector, exports
}

type blockedTracingApplication struct {
	app         *iris.Application
	listener    *observedListener
	key         tracingKey
	runtime     *o.Runtime
	span        trace.Span
	release     func()
	requestDone chan struct{}
	requestErr  error
}

func startBlockedTracingApplication(t *testing.T, service, endpoint string) *blockedTracingApplication {
	t.Helper()
	app := iris.New()
	app.Use(TracingHandlerFor(app, true, service, endpoint))
	key := tracingKey{service: service, endpoint: endpoint, application: app}
	legacyTracing.Lock()
	runtime := legacyTracing.runtimes[key]
	legacyTracing.Unlock()
	if runtime == nil {
		t.Fatal("bound runtime was not registered at startup")
	}
	release, entered := make(chan struct{}), make(chan trace.Span, 1)
	var once sync.Once
	running := &blockedTracingApplication{
		app: app, key: key, runtime: runtime, requestDone: make(chan struct{}),
		release: func() { once.Do(func() { close(release) }) },
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := shutdownTracingRuntimes(ctx, map[tracingKey]*o.Runtime{key: runtime}); err != nil {
			t.Error(err)
		}
	})
	app.Get("/", func(c iris.Context) {
		entered <- trace.SpanFromContext(c.Request().Context())
		<-release
		_, _ = c.WriteString("ok")
	})
	build(t, app)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	running.listener = &observedListener{Listener: listener, stopped: make(chan struct{})}
	server := app.NewHost(&http.Server{Handler: app})
	served := make(chan error, 1)
	go func() { served <- server.Serve(running.listener) }()
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() {
		running.release()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := app.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		_ = server.Server.Close()
		_ = listener.Close()
		client.CloseIdleConnections()
		select {
		case <-running.requestDone:
		case <-time.After(3 * time.Second):
			t.Error("fixture request did not exit")
		}
		select {
		case err := <-served:
			if !errors.Is(err, http.ErrServerClosed) {
				t.Errorf("fixture server exit: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("fixture server did not exit")
		}
	})
	go func() {
		response, err := client.Get("http://" + listener.Addr().String() + "/")
		if err == nil {
			var body []byte
			body, err = io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err == nil && (response.StatusCode != http.StatusOK || string(body) != "ok") {
				err = errors.New("in-flight response changed")
			}
		}
		running.requestErr = err
		close(running.requestDone)
	}()
	select {
	case running.span = <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("fixture request did not enter")
	}
	if !running.span.IsRecording() {
		t.Fatal("fixture request has no recording span")
	}
	return running
}

func (running *blockedTracingApplication) finishRequest(t *testing.T) {
	t.Helper()
	running.release()
	waitTracingBoundary(t, running.requestDone, "released request did not finish")
	if running.requestErr != nil {
		t.Fatal(running.requestErr)
	}
	if running.span.IsRecording() {
		t.Fatal("completed request span is still recording")
	}
}

func waitTracingBoundary(t *testing.T, done <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(message)
	}
}
