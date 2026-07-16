package m

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"time"

	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	"github.com/universe-ce/meta/v3/pkg/x"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingHandler 日志tracing handler
func TracingHandler(enable bool, server string, url string) iris.Handler {
	return func(ctx iris.Context) {
		if !enable {
			logRequest(ctx)
			return
		}
		if url == "" {
			log.Warnf("Opentracing url is empty!")
			logRequest(ctx)
			return
		}
		exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
		if err != nil {
			log.Errorf("OpenTracing connect is error")
			logRequest(ctx)
			return
		}
		tp := tracesdk.NewTracerProvider(
			tracesdk.WithBatcher(exp),
			tracesdk.WithResource(resource.NewSchemaless(
				semconv.ServiceNameKey.String(server),
			)),
		)

		otel.SetTracerProvider(tp)
		// otel.SetTextMapPropagator(propagation.TraceContext{})
		b3Propagator := b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader))
		propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}, b3Propagator)
		otel.SetTextMapPropagator(propagator)

		bctx := context.Background()
		var tracer = otel.Tracer(server)
		// 塞入自定义参数
		tracer.Start(bctx, "request", trace.WithAttributes(attribute.String("type", "zce-opentracing")))

		// 继续处理请求，无论成功或者失败都不应该影响主业务流程
		logRequest(ctx)
		return
	}
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
	fields["url"] = ctx.Request().URL.String()
	fields["proto"] = ctx.Request().Proto
	fields["header"] = ctx.Request().Header
	fields["user_agent"] = ctx.Request().UserAgent()
	fields["x_requestid"] = ctx.GetHeader("X-Request-Id")

	// 如果是POST/PUT请求，并且内容类型为JSON，则读取内容体
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		body, err := io.ReadAll(ctx.Request().Body)
		if err == nil {
			defer func(Body io.ReadCloser) {
				_ = Body.Close()
			}(ctx.Request().Body)
			buf := bytes.NewBuffer(body)
			ctx.Request().Body = io.NopCloser(buf)
			fields["content_length"] = ctx.GetContentLength()
			fields["body"] = string(body)
		}
	}

	ctx.Next()

	// 请求处理完毕后执行
	// 请求处理时间
	end := time.Now()
	latency := end.Sub(start)
	fields["latency"] = latency

	//下面是返回日志
	fields["status_code"] = ctx.ResponseWriter().StatusCode()
	if ctx.Values().GetString("out_err") != "" {
		fields["out_err"] = ctx.Values().GetString("out_err")
	}
	fields["res_length"] = ctx.ResponseWriter().Header().Get("size")
	if v := ctx.Values().Get("res_body"); v != nil {
		if b, ok := v.([]byte); ok {
			fields["res_body"] = string(b)
		}
	}

	// 打印日志
	filedJson, _ := json.Marshal(fields)
	log.Infof("请求日志: %s", string(filedJson))
}
