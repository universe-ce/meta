package r

import (
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
	"github.com/universe-ce/meta/v3/pkg/m"
	"net/http"
)

type JsonResult = m.JsonResult

type ApiError = m.ApiError

func NewApiError(status int, code int, langKey string, message string, info string, err error) *ApiError {
	return m.NewApiError(status, code, langKey, message, info, err)
}

type ValidationError = m.ValidationError

var (
	ErrOK = NewApiError(
		http.StatusOK, 0, "", "success", "success", nil,
		//Status:   http.StatusOK,
		//Code:     0,
		//Langkey:  "",
		//Message:  "success",
		//Moreinfo: "success",
		//Err:      nil,
	) // 正常响应
	ErrContinue           = NewApiError(100, 10100, "err.ErrContinue", "Continue", "", nil)                  //
	ErrSwitchProtocol     = NewApiError(101, 10101, "err.ErrSwitchProtocol", "SwitchProtocol", "", nil)      //
	ErrProcessing         = NewApiError(102, 10102, "err.ErrProcessing", "Processing", "", nil)              //
	ErrCreated            = NewApiError(201, 0, "err.ErrCreated", "Created", "", nil)                        // 20x错误
	ErrAccepted           = NewApiError(202, 0, "err.ErrAccepted", "Accepted", "", nil)                      // 20x错误
	Err400                = NewApiError(400, 10400, "err.Err400", "Bad Request", "", nil)                    // 400错误
	Err401                = NewApiError(401, 10401, "err.Err401", "Unauthorized", "", nil)                   // 401错误
	Err403                = NewApiError(403, 10403, "err.Err403", "Forbidden", "", nil)                      // 403错误
	Err404                = NewApiError(404, 10404, "err.Err404", "Not Found", "", nil)                      // 404错误
	Err405                = NewApiError(405, 10405, "err.Err405", "Method Not Allowed", "", nil)             // 405错误
	ErrTimeout            = NewApiError(408, 10408, "err.ErrTimeout", "Request Timeout", "", nil)            // 405错误
	ErrTooLarge           = NewApiError(413, 10413, "err.ErrTooLarge", "Request Entity Too Large", "", nil)  // 413错误
	ErrUnsupported        = NewApiError(415, 10415, "err.ErrUnsupported", "Unsupported Media Type", "", nil) // 415错误
	ErrLocked             = NewApiError(423, 10423, "err.ErrLocked", "Process locked", "", nil)
	ErrInstall            = NewApiError(500, 11001, "err.ErrInstall", "", "", nil)                                    // 安装错误
	ErrInternalServer     = NewApiError(500, 11000, "err.ErrInternalServer", "Internal Server Error", "", nil)        // 服务内部错误
	ErrBadGateway         = NewApiError(502, 10502, "err.ErrBadGateway", "Bad Gateway", "", nil)                      // 服务内部错误
	ErrServiceUnavailable = NewApiError(503, 10503, "err.ErrServiceUnavailable", "Service Unavailable", "", nil)      // 参数错误
	ErrParams             = NewApiError(500, 11002, "err.ErrParams", "Invalid Params", "", nil)                       // 参数错误
	ErrInvalidToken       = NewApiError(500, 10401, "err.ErrInvalidToken", "Invalid Token", "", nil)                  // Token解析错误
	ErrResourceNotFound   = NewApiError(405, 11003, "err.ErrResourceNotFound", "Resource NotFound", "", nil)          // 资源找不到错误
	ErrResourceNotMatch   = NewApiError(405, 11004, "err.ErrResourceNotMatch", "Resource NotMatch", "", nil)          // 资源不匹配错误
	ErrResourceExisted    = NewApiError(405, 10001, "err.ErrResourceExisted", "Resource Existed", "", nil)            // 资源已存在
	ErrDatabase           = NewApiError(500, 10002, "err.ErrDatabase", "Database Error", "", nil)                     // 数据库错误
	ErrDupUser            = NewApiError(500, 10003, "err.ErrDupUser", "Duplicate User", "", nil)                      // 用户重复错误
	ErrNoUser             = NewApiError(500, 10004, "err.ErrNoUser", "No User", "", nil)                              // 用户未登陆错误
	ErrPass               = NewApiError(500, 10005, "err.ErrPass", "Password Error", "", nil)                         // 用户通过错误
	ErrNoUserPass         = NewApiError(500, 10006, "err.ErrNoUserPass", "No Password", "", nil)                      // 没有输入用户密码错误
	ErrNoUserChange       = NewApiError(500, 10007, "err.ErrNoUserChange", "User Not Change", "", nil)                // 用户未改动错误
	ErrInvalidUser        = NewApiError(500, 10008, "err.ErrInvalidUser", "Invalid User", "", nil)                    // 异常用户错误
	ErrOpenFile           = NewApiError(500, 10009, "err.ErrOpenFile", "Open File Error", "", nil)                    // 文件打开错误
	ErrWriteFile          = NewApiError(500, 10010, "err.ErrWriteFile", "Write File Error", "", nil)                  // 文件写入错误
	ErrSystem             = NewApiError(500, 10011, "err.ErrSystem", "System Error", "", nil)                         // 系统错误
	ErrExpired            = NewApiError(500, 10012, "err.ErrExpired", "Expired", "", nil)                             // 过期错误
	ErrPermission         = NewApiError(500, 10013, "err.ErrPermission", "Permission Denied", "", nil)                // 没有权限错误
	ErrGenJwt             = NewApiError(500, 10014, "err.ErrGenJwt", "General Token Error", "", nil)                  // JWT Token生成错误
	ErrValidJwt           = NewApiError(500, 10012, "err.ErrValidJwt", "Valid Token", "", nil)                        // JWT Token校验失败错误
	ErrIdData             = NewApiError(500, 10016, "err.ErrIdData", "ID Error", "", nil)                             // ID错误
	ErrDifferentPasswords = NewApiError(500, 10017, "err.ErrDifferentPasswords", "Password Different Error", "", nil) // 密码不匹配错误
	ErrSamePasswords      = NewApiError(500, 10018, "err.ErrSamePasswords", "Same Password", "", nil)                 // 密码相同错误
	ErrAddFail            = NewApiError(500, 11000, "err.ErrAddFail", "Add Error", "", nil)                           // 添加错误
	ErrEditFail           = NewApiError(500, 11001, "err.ErrEditFail", "Edit Error", "", nil)                         // 编辑错误
	ErrDelFail            = NewApiError(500, 11002, "err.ErrDelFail", "Delete Error", "", nil)                        // 删除错误
	ErrGrantFail          = NewApiError(500, 12000, "err.ErrGrantFail", "Grant Failed", "", nil)                      // 角色没有权限
	ErrCaptchaEmpty       = NewApiError(500, 13001, "err.ErrCaptchaEmpty", "Captcha Empty", "", nil)                  //验证码为空错误
	ErrCaptcha            = NewApiError(500, 13002, "err.ErrCaptcha", "Captcha Error", "", nil)                       //验证码错误
	ErrDupRecord          = NewApiError(500, 13005, "err.ErrDupRecord", "Duplicate Data", "", nil)                    // 重复的结果集错误
	ErrWrongRefreshToken  = NewApiError(500, 13006, "err.ErrWrongRefreshToken", "Token Refresh Error", "", nil)       // RefreshToken刷新错误
	ErrBindDingtalk       = NewApiError(500, 13007, "err.ErrBindDingtalk", "Binding DingDing Error", "", nil)         // 绑定钉钉错误
	ErrUnBindDingtalk     = NewApiError(500, 13008, "err.ErrUnBindDingtalk", "UnBinding DingDing Error", "", nil)     // 钉钉未绑定错误
	ErrSendMail           = NewApiError(500, 13010, "err.ErrSendMail", "Send Email Error", "", nil)                   // 邮件发送错误
	ErrValidation         = NewApiError(500, 13011, "err.ErrValidate", "Validated Error", "", nil)                    // 校验错误
	ErrNoRecord           = NewApiError(500, 13012, "err.ErrNoRecord", "No Record", "", nil)                          // 无结果错误
	ErrContainsSubset     = NewApiError(500, 13013, "err.ErrContainsSubset", "Contains SubSet Data", "", nil)         // 含有子集错误
	ErrUploadAvatar       = NewApiError(500, 13014, "err.ErrUploadAvatar", "Upload Avatar Error", "", nil)            // 头像上传失败错误
	ErrUploadFile         = NewApiError(500, 13014, "err.ErrUploadFile", "Upload Error", "", nil)                     // 头像上传失败错误
	ErrSmsSendCode        = NewApiError(500, 13015, "err.ErrSendCode", "Send Sms Error", "", nil)                     // 短信发送错误
	ErrLowSecurityLevel   = NewApiError(500, 13016, "err.ErrLowSecurityLevel", "Low Security", "", nil)               // 安全等级过低
	ErrSendMsg            = NewApiError(500, 13017, "err.ErrSendMsg", "Send Msg Eror", "", nil)                       // 安全等级过低
	ErrBinding            = NewApiError(500, 13007, "err.ErrBinding", "Binding Error", "", nil)                       // 绑定资源错误
	ErrUnBinding          = NewApiError(500, 13008, "err.ErrUnBinding", "UnBinding DingDing Error", "", nil)          // 解除绑定资源错误
	ErrLdapUser           = NewApiError(500, 10009, "err.ErrLdapUser", "Incorrect Ldap User", "", nil)
)

func ProcErr(e *ApiError, err error) *ApiError {
	e.Err = err
	return e
}

// HandlerPgError 处理pg错误
func HandlerPgError(c iris.Context, err error) {
	if pgError := err.(*pgconn.PgError); pgError != nil {
		FailWithMsg(c, ErrDatabase, pgError.Detail)
		return
	}
	FailWithMsg(c, ErrDatabase, err.Error())
}

// HandlerInternalError 处理内部错误
//func HandlerInternalError(ctx iris.Context, apiErr *ApiError) {
//	ctx.StatusCode(apiErr.Status)
//	if err := ctx.JSON(JsonResult{
//		Code: apiErr.Code,
//		Msg:  apiErr.Message,
//	}); err != nil {
//		log.Errorf("JSON error: %v", err)
//	}
//	return
//}

// OK 请求成功
func OK(ctx iris.Context) {
	ctx.StatusCode(ErrOK.Status)
	if err := ctx.JSON(JsonResult{
		Code: ErrOK.Code,
		Msg:  ErrOK.Message,
	}); err != nil {
		log.Errorf("JSON error: %v", err)
	}
	return
}

// OKWithData 请求成功并携带数据
func OKWithData(ctx iris.Context, data interface{}) {
	ctx.StatusCode(ErrOK.Status)
	if data != nil {
		if err := ctx.JSON(JsonResult{
			Code: ErrOK.Code,
			Msg:  ErrOK.Message,
			Data: data,
		}); err != nil {
			log.Errorf("JSON error: %v", err)
		}
	} else {
		if err := ctx.JSON(JsonResult{
			Code: ErrOK.Code,
			Msg:  ErrOK.Message,
		}); err != nil {
			log.Errorf("JSON error: %v", err)
		}
	}
	return
}

// OKWithErrMsg 请求成功，但是接口报错
func OKWithErrMsg(ctx iris.Context, apiErr *ApiError, msg string) {
	ctx.StatusCode(ErrOK.Status)
	if apiErr != nil {
		if err := ctx.JSON(JsonResult{
			Code: apiErr.Code,
			Msg:  msg,
		}); err != nil {
			log.Errorf("JSON error: %v", err)
		}
	} else {
		if err := ctx.JSON(JsonResult{
			Code: ErrOK.Code,
			Msg:  ErrOK.Message,
		}); err != nil {
			log.Errorf("JSON error: %v", err)
		}
	}
	return
}

// OKWithPage 请求成功并携带数据
//func OKWithPage(ctx iris.Context, list interface{}, total int64) {
//	ctx.StatusCode(ErrOK.Status)
//	if err := ctx.JSON(JsonResult{
//		Code: ErrOK.Code,
//		Msg:  ErrOK.Message,
//		Data: map[string]interface{}{
//			"list":  list,
//			"total": total,
//		},
//	}); err != nil {
//		log.Errorf("JSON error: %v", err)
//	}
//	return
//}

// Fail 请求失败
func Fail(ctx iris.Context, apiErr *ApiError) {
	ctx.StatusCode(apiErr.Status)
	if err := ctx.JSON(JsonResult{
		Code: apiErr.Code,
		Msg:  apiErr.Message,
	}); err != nil {
		log.Errorf("JSON error: %v", err)
	}
	return
}

// FailWithMsg 请求失败并携带消息
func FailWithMsg(ctx iris.Context, apiErr *ApiError, errmsg string) {
	ctx.StatusCode(apiErr.Status)
	if err := ctx.JSON(JsonResult{
		Code: apiErr.Code,
		Msg:  errmsg,
		Err:  apiErr.Message,
	}); err != nil {
		log.Errorf("JSON error: %v", err)
	}
	return
}

func wrapValidationErrors(errs validator.ValidationErrors) []ValidationError {
	validationErrors := make([]ValidationError, 0, len(errs))
	for _, validationErr := range errs {
		validationErrors = append(validationErrors, ValidationError{
			ActualTag: validationErr.ActualTag(),
			Namespace: validationErr.Namespace(),
			Kind:      validationErr.Kind().String(),
			Type:      validationErr.Type().String(),
			Value:     fmt.Sprintf("%v", validationErr.Value()),
			Param:     validationErr.Param(),
		})
	}
	return validationErrors
}

func HandlerValidateError(c iris.Context, err error) {
	var validErrs []ValidationError
	var errs validator.ValidationErrors
	if errors.As(err, &errs) {
		validErrs = wrapValidationErrors(errs)
		FailWithMsg(c, ErrValidation, fmt.Sprintf("%s %s", validErrs[0].Namespace, validErrs[0].ActualTag))
		return
	}
	FailWithMsg(c, ErrValidation, err.Error())
	return
}
