package mw

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	"github.com/swaggo/swag"
	mm "github.com/universe-ce/meta/v3/pkg/m"
	"github.com/universe-ce/meta/v3/pkg/t"
	"github.com/universe-ce/meta/v3/pkg/x"
)

// AuditHandler 审计操作
// msgSender 消息发送函数
// isExcludedPath 设置需要排除的接口
// moduleName 审计模块名称
func AuditHandler(msgSender func(msg *x.AuditLog), isExcludedPath func(path string) bool, moduleName string) iris.Handler {
	return func(ctx iris.Context) {
		if msgSender == nil || isExcludedPath == nil {
			ctx.Next()
			return
		}
		if ctx.Method() == "GET" || isExcludedPath(ctx.Path()) {
			// 查询接口和其他指定接口不审计
			ctx.Next()
			return
		}
		uid, _ := ctx.User().GetID()
		username, _ := ctx.User().GetUsername()
		ctx.RecordRequestBody(true)
		// 获取请求body
		requestBody, _ := ctx.GetBody()
		// 创建一个新的 ResponseRecorder 来捕获响应
		recorder := ctx.Recorder()

		// 继续处理请求
		ctx.Next()

		// 审计操作
		objectType, objectName := getTargetObjectName(recoverSwaggerPath(ctx.Path()), strings.ToLower(ctx.Method()))
		// todo 审计panic事件
		go msgSender(&x.AuditLog{
			XRequestId: ctx.GetHeader("X-Request-Id"),
			UserId:     uid,
			UserName:   username,
			Module:     moduleName,
			Action:     getTargetAction(ctx.Method()),
			ObjectId:   getObjectId(ctx.Method(), requestBody, ctx),
			ObjectType: objectType,
			ObjectName: objectName,
			Result:     getResult(recorder.StatusCode()),
			URI:        ctx.Path(),
			Params:     string(requestBody),
			Ip:         x.GetClientIp(ctx),
			Device:     x.ResolveDevice(ctx.Request()).DeviceName(),
			Platform:   x.ResolveDevice(ctx.Request()).Platform(),
			Browser:    x.GetBrowserName(ctx.GetHeader("User-Agent")),
			Os:         x.GetOsName(ctx.GetHeader("User-Agent")),
			Timestamp:  time.Now().Format(t.DatetimeFormat),
		})
	}
}

func getObjectId(method string, requestBody []byte, ctx iris.Context) string {
	switch method {
	case "PUT":
		var req mm.NormalEditReq
		if err := json.Unmarshal(requestBody, &req); err != nil {
			log.Error(err)
			return ""
		}
		return req.Id.Value
	case "DELETE":
		id := ctx.URLParam("id")
		return id
	default:
		return ""
	}
}

func getTargetAction(method string) x.OperateType {
	switch method {
	case "GET":
		return x.QUERY
	case "POST":
		return x.CREATE
	case "PUT":
		return x.UPDATE
	case "DELETE":
		return x.DELETE
	default:
		return x.OTHER
	}
}

func getResult(code int) x.OperateResult {
	switch code {
	case 200:
		return x.SUCCESS
	default:
		return x.FAILURE
	}
}

func getTargetObjectName(path, method string) (string, string) {
	doc, err := swag.ReadDoc()
	if err != nil {
		log.Error(err)
		return path, method
	}
	// 根据路由获取对象类型和对象名称
	var swaggerDoc mm.SwaggerDoc
	if err = json.Unmarshal([]byte(doc), &swaggerDoc); err != nil {
		log.Error(err)
		return path, method
	}

	for p, methods := range swaggerDoc.Paths {
		if p == path {
			for m, details := range methods {
				if m == method {
					var tag string
					if len(details.Tags) > 0 {
						tag = strings.TrimRight(details.Tags[0], "接口")
					}
					return tag, details.Summary
				}
			}
		}
	}
	return path, method
}

func recoverSwaggerPath(path string) string {
	reg := regexp.MustCompile(`/project/([0-9]+)`)
	return reg.ReplaceAllString(path, "/project/{projectId:string}")
}
