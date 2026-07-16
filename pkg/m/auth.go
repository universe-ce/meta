package m

import (
	"encoding/json"
	"time"

	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/context"
	"github.com/universe-ce/meta/v3/pkg/r"
	"github.com/universe-ce/meta/v3/pkg/x"
)

// AuthHandler 登陆handler
func AuthHandler(tokenKey, signKey string) iris.Handler {
	return func(ctx *context.Context) {
		// URI放行设置
		var urlItem []string // 此处目前没有需要跳过的uri // "/captcha", "/login"
		urlItem = append(urlItem, "/api/v2/uaa/account/login")
		if !x.InStringArray(ctx.Path(), urlItem) {
			token := x.GetToken(ctx, tokenKey)
			// 字符串替换
			token = x.Replace(token, "Bearer ", "")
			claim, err := x.ParseToken(signKey, token)
			if err != nil {
				log.Errorf("解析token出现错误：%v", err)
				_ = ctx.JSON(r.JsonResult{
					Code: 401,
					Msg:  "Token已过期",
				})
				return
			} else if time.Now().Unix() > claim.ExpiresAt {
				log.Errorf("Token已超时: %v", token)
				_ = ctx.JSON(r.JsonResult{
					Code: 401,
					Msg:  "时间超时",
				})
				return
			}
			if ctx.User() == nil && claim != nil {
				field := map[string]interface{}{
					"name": claim.Name,
				}
				su := &context.SimpleUser{
					ID:       claim.UserId,
					Username: claim.Username,
					Token:    json.RawMessage(token),
					Roles:    claim.Roles,
					Fields:   field,
				}
				_ = ctx.SetUser(su)
			}
		}
		// 前置中间件
		ctx.Next()
	}
}
