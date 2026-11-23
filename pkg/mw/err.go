package mw

import (
	"fmt"
	"runtime"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
	"github.com/universe-ce/meta/v3/pkg/r"
)

func ErrorsHandler() iris.Handler {
	return func(ctx iris.Context) {
		defer func() {
			if err := recover(); err != nil {
				if ctx.IsStopped() {
					return
				}
				var stacktrace string
				for i := 1; ; i++ {
					_, f, l, got := runtime.Caller(i)
					if !got {
						break
					}
					stacktrace += fmt.Sprintf("%s:%d\n", f, l)
				}
				errMsg := fmt.Sprintf("错误信息: %s", err)
				// when stack finishes
				logMessage := fmt.Sprintf("从错误中回复：('%s')\n", ctx.HandlerName())
				logMessage += errMsg + "\n"
				logMessage += fmt.Sprintf("\n%s", stacktrace)
				// 打印错误日志
				ctx.Application().Logger().Warn(logMessage)
				_ = ctx.JSON(
					r.JsonResult{
						Code: httptest.StatusInternalServerError,
						Msg:  "InternalServerError",
					},
				)
				ctx.StatusCode(httptest.StatusInternalServerError)
				ctx.StopExecution()
			}
		}()
		ctx.Next()
	}
}
