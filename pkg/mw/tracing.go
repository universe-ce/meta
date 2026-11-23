package mw

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"sync"
	"time"

	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	irisctx "github.com/kataras/iris/v12/context"
	"github.com/universe-ce/meta/v3/pkg/o"
	"github.com/universe-ce/meta/v3/pkg/x"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type tracingKey struct {
	service, endpoint string
	application       irisctx.Application
}

var legacyTracing = struct {
	sync.Mutex
	runtimes map[tracingKey]*o.Runtime
}{
	runtimes: make(map[tracingKey]*o.Runtime),
}

var legacyTracingInterrupt sync.Once

// TracingHandler preserves the legacy entry point and global OTel integration.
// A runtime is shared by handlers with the same configuration. Its caller owns
// HTTP shutdown and must call ShutdownTracing after draining all requests.
// Use TracingHandlerFor to bind an Iris application before it starts listening,
// or own a o.Runtime and use TracingWithRuntime in a new service.
func TracingHandler(enable bool, server string, url string) iris.Handler {
	return TracingHandlerFor(nil, enable, server, url)
}

// TracingHandlerFor binds the application at startup. Its interrupt callback
// drains that application before closing the runtime; it never infers ownership
// from the first request or the last globally created Iris application.
// A nil application keeps the manual lifecycle of TracingHandler.
func TracingHandlerFor(app irisctx.Application, enable bool, server string, url string) iris.Handler {
	if !enable {
		return logRequest
	}
	if url == "" {
		log.Warnf("Opentracing url is empty!")
		return logRequest
	}
	legacyTracing.Lock()
	defer legacyTracing.Unlock()
	key := tracingKey{server, url, app}
	runtime, ok := legacyTracing.runtimes[key]
	if !ok {
		var err error
		runtime, err = o.NewJaeger(server, url)
		if err != nil {
			log.Errorf("OpenTracing connect is error")
			return logRequest
		}
		legacyTracing.runtimes[key] = runtime
		otel.SetTracerProvider(runtime.TracerProvider())
		otel.SetTextMapPropagator(runtime.Propagator())
	}
	if app != nil {
		legacyTracingInterrupt.Do(func() {
			iris.RegisterOnInterrupt(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := shutdownTracingApplications(ctx); err != nil {
					log.Errorf("OpenTracing shutdown: %v", err)
				}
			})
		})
	}
	return TracingWithRuntime(runtime)
}

func shutdownTracingApplications(ctx context.Context) error {
	legacyTracing.Lock()
	applications := make(map[irisctx.Application]struct{})
	runtimes := make(map[tracingKey]*o.Runtime)
	for key, runtime := range legacyTracing.runtimes {
		if key.application != nil {
			applications[key.application] = struct{}{}
			runtimes[key] = runtime
		}
	}
	legacyTracing.Unlock()
	for app := range applications {
		if err := app.Shutdown(ctx); err != nil {
			// Leave telemetry open if HTTP requests could still end spans.
			return err
		}
	}
	return shutdownTracingRuntimes(ctx, runtimes)
}

// TracingWithRuntime borrows a runtime; its owner must call Shutdown after
// draining HTTP requests. Each request keeps cancellation, baggage and parentage.
func TracingWithRuntime(runtime *o.Runtime) iris.Handler {
	if runtime == nil {
		return logRequest
	}
	return func(ctx iris.Context) {
		request := ctx.Request()
		traceContext, span := runtime.Start(request.Context(), propagation.HeaderCarrier(request.Header))
		defer span.End()
		ctx.ResetRequest(request.WithContext(traceContext))
		logRequest(ctx)
	}
}

// ShutdownTracing drains the legacy runtimes. Call after the server has stopped
// accepting requests; it also runs on the existing Iris interrupt lifecycle.
func ShutdownTracing(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	legacyTracing.Lock()
	runtimes := make(map[tracingKey]*o.Runtime, len(legacyTracing.runtimes))
	for key, runtime := range legacyTracing.runtimes {
		runtimes[key] = runtime
	}
	legacyTracing.Unlock()
	return shutdownTracingRuntimes(ctx, runtimes)
}

func shutdownTracingRuntimes(ctx context.Context, runtimes map[tracingKey]*o.Runtime) error {
	var result error
	for key, runtime := range runtimes {
		if err := runtime.Shutdown(ctx); err != nil {
			result = errors.Join(result, err)
			continue
		}
		legacyTracing.Lock()
		if legacyTracing.runtimes[key] == runtime {
			delete(legacyTracing.runtimes, key)
		}
		legacyTracing.Unlock()
	}
	return result
}

func logRequest(ctx iris.Context) {
	start := time.Now()

	reqPath := ctx.RequestPath(true)
	method := ctx.Request().Method
	fields := make(map[string]interface{})
	fields["title"] = "访问日志"
	fields["fun_name"] = path.Join(method, reqPath)
	fields["ip"] = x.GetClientIp(ctx) //ctx.Request().RemoteAddr
	fields["method"] = method
	fields["url"] = ctx.Request().URL.Path
	fields["proto"] = ctx.Request().Proto
	fields["content_type"] = ctx.Request().Header.Get("Content-Type")
	fields["user_agent"] = ctx.Request().UserAgent()
	fields["x_requestid"] = ctx.GetHeader("X-Request-Id")

	// Technical request logs contain metadata only. Tokens may occur in any
	// header, query, body, response or business error; those values stay out.
	// In particular, logging must not consume or buffer an incoming stream.
	fields["content_length"] = ctx.GetContentLength()

	ctx.Next()

	// 请求处理完毕后执行
	// 请求处理时间
	end := time.Now()
	latency := end.Sub(start)
	fields["latency"] = latency

	//下面是返回日志
	fields["status_code"] = ctx.ResponseWriter().StatusCode()
	fields["res_length"] = ctx.ResponseWriter().Header().Get("size")

	// 打印日志
	filedJson, _ := json.Marshal(fields)
	log.Infof("请求日志: %s", string(filedJson))
}
