package r

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kataras/iris/v12"
)

// This table freezes the published catalog, including duplicate codes and typos.
func TestErrorCatalog(t *testing.T) {
	rows := []struct {
		name               string
		err                *ApiError
		status, code       int
		key, message, info string
	}{
		{"OK", ErrOK, 200, 0, "", "success", "success"},
		{"Continue", ErrContinue, 100, 10100, "err.ErrContinue", "Continue", ""},
		{"SwitchProtocol", ErrSwitchProtocol, 101, 10101, "err.ErrSwitchProtocol", "SwitchProtocol", ""},
		{"Processing", ErrProcessing, 102, 10102, "err.ErrProcessing", "Processing", ""},
		{"Created", ErrCreated, 201, 0, "err.ErrCreated", "Created", ""},
		{"Accepted", ErrAccepted, 202, 0, "err.ErrAccepted", "Accepted", ""},
		{"400", Err400, 400, 10400, "err.Err400", "Bad Request", ""},
		{"401", Err401, 401, 10401, "err.Err401", "Unauthorized", ""},
		{"403", Err403, 403, 10403, "err.Err403", "Forbidden", ""},
		{"404", Err404, 404, 10404, "err.Err404", "Not Found", ""},
		{"405", Err405, 405, 10405, "err.Err405", "Method Not Allowed", ""},
		{"Timeout", ErrTimeout, 408, 10408, "err.ErrTimeout", "Request Timeout", ""},
		{"TooLarge", ErrTooLarge, 413, 10413, "err.ErrTooLarge", "Request Entity Too Large", ""},
		{"Unsupported", ErrUnsupported, 415, 10415, "err.ErrUnsupported", "Unsupported Media Type", ""},
		{"Locked", ErrLocked, 423, 10423, "err.ErrLocked", "Process locked", ""},
		{"Install", ErrInstall, 500, 11001, "err.ErrInstall", "", ""},
		{"InternalServer", ErrInternalServer, 500, 11000, "err.ErrInternalServer", "Internal Server Error", ""},
		{"BadGateway", ErrBadGateway, 502, 10502, "err.ErrBadGateway", "Bad Gateway", ""},
		{"ServiceUnavailable", ErrServiceUnavailable, 503, 10503, "err.ErrServiceUnavailable", "Service Unavailable", ""},
		{"Params", ErrParams, 500, 11002, "err.ErrParams", "Invalid Params", ""},
		{"InvalidToken", ErrInvalidToken, 500, 10401, "err.ErrInvalidToken", "Invalid Token", ""},
		{"ResourceNotFound", ErrResourceNotFound, 405, 11003, "err.ErrResourceNotFound", "Resource NotFound", ""},
		{"ResourceNotMatch", ErrResourceNotMatch, 405, 11004, "err.ErrResourceNotMatch", "Resource NotMatch", ""},
		{"ResourceExisted", ErrResourceExisted, 405, 10001, "err.ErrResourceExisted", "Resource Existed", ""},
		{"Database", ErrDatabase, 500, 10002, "err.ErrDatabase", "Database Error", ""},
		{"DupUser", ErrDupUser, 500, 10003, "err.ErrDupUser", "Duplicate User", ""},
		{"NoUser", ErrNoUser, 500, 10004, "err.ErrNoUser", "No User", ""},
		{"Pass", ErrPass, 500, 10005, "err.ErrPass", "Password Error", ""},
		{"NoUserPass", ErrNoUserPass, 500, 10006, "err.ErrNoUserPass", "No Password", ""},
		{"NoUserChange", ErrNoUserChange, 500, 10007, "err.ErrNoUserChange", "User Not Change", ""},
		{"InvalidUser", ErrInvalidUser, 500, 10008, "err.ErrInvalidUser", "Invalid User", ""},
		{"OpenFile", ErrOpenFile, 500, 10009, "err.ErrOpenFile", "Open File Error", ""},
		{"WriteFile", ErrWriteFile, 500, 10010, "err.ErrWriteFile", "Write File Error", ""},
		{"System", ErrSystem, 500, 10011, "err.ErrSystem", "System Error", ""},
		{"Expired", ErrExpired, 500, 10012, "err.ErrExpired", "Expired", ""},
		{"Permission", ErrPermission, 500, 10013, "err.ErrPermission", "Permission Denied", ""},
		{"GenJwt", ErrGenJwt, 500, 10014, "err.ErrGenJwt", "General Token Error", ""},
		{"ValidJwt", ErrValidJwt, 500, 10012, "err.ErrValidJwt", "Valid Token", ""},
		{"IdData", ErrIdData, 500, 10016, "err.ErrIdData", "ID Error", ""},
		{"DifferentPasswords", ErrDifferentPasswords, 500, 10017, "err.ErrDifferentPasswords", "Password Different Error", ""},
		{"SamePasswords", ErrSamePasswords, 500, 10018, "err.ErrSamePasswords", "Same Password", ""},
		{"AddFail", ErrAddFail, 500, 11000, "err.ErrAddFail", "Add Error", ""},
		{"EditFail", ErrEditFail, 500, 11001, "err.ErrEditFail", "Edit Error", ""},
		{"DelFail", ErrDelFail, 500, 11002, "err.ErrDelFail", "Delete Error", ""},
		{"GrantFail", ErrGrantFail, 500, 12000, "err.ErrGrantFail", "Grant Failed", ""},
		{"CaptchaEmpty", ErrCaptchaEmpty, 500, 13001, "err.ErrCaptchaEmpty", "Captcha Empty", ""},
		{"Captcha", ErrCaptcha, 500, 13002, "err.ErrCaptcha", "Captcha Error", ""},
		{"DupRecord", ErrDupRecord, 500, 13005, "err.ErrDupRecord", "Duplicate Data", ""},
		{"WrongRefreshToken", ErrWrongRefreshToken, 500, 13006, "err.ErrWrongRefreshToken", "Token Refresh Error", ""},
		{"BindDingtalk", ErrBindDingtalk, 500, 13007, "err.ErrBindDingtalk", "Binding DingDing Error", ""},
		{"UnBindDingtalk", ErrUnBindDingtalk, 500, 13008, "err.ErrUnBindDingtalk", "UnBinding DingDing Error", ""},
		{"SendMail", ErrSendMail, 500, 13010, "err.ErrSendMail", "Send Email Error", ""},
		{"Validation", ErrValidation, 500, 13011, "err.ErrValidate", "Validated Error", ""},
		{"NoRecord", ErrNoRecord, 500, 13012, "err.ErrNoRecord", "No Record", ""},
		{"ContainsSubset", ErrContainsSubset, 500, 13013, "err.ErrContainsSubset", "Contains SubSet Data", ""},
		{"UploadAvatar", ErrUploadAvatar, 500, 13014, "err.ErrUploadAvatar", "Upload Avatar Error", ""},
		{"UploadFile", ErrUploadFile, 500, 13014, "err.ErrUploadFile", "Upload Error", ""},
		{"SmsSendCode", ErrSmsSendCode, 500, 13015, "err.ErrSendCode", "Send Sms Error", ""},
		{"LowSecurityLevel", ErrLowSecurityLevel, 500, 13016, "err.ErrLowSecurityLevel", "Low Security", ""},
		{"SendMsg", ErrSendMsg, 500, 13017, "err.ErrSendMsg", "Send Msg Eror", ""},
		{"Binding", ErrBinding, 500, 13007, "err.ErrBinding", "Binding Error", ""},
		{"UnBinding", ErrUnBinding, 500, 13008, "err.ErrUnBinding", "UnBinding DingDing Error", ""},
		{"LdapUser", ErrLdapUser, 500, 10009, "err.ErrLdapUser", "Incorrect Ldap User", ""},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			got, err := json.Marshal(row.err)
			want := fmt.Sprintf(`{"status":%d,"code":%d,"langkey":%q,"msg":%q,"moreinfo":%q,"err":null}`, row.status, row.code, row.key, row.message, row.info)
			if err != nil || string(got) != want || row.err.Error() != row.message {
				t.Fatalf("got %s (%v), want %s", got, err, want)
			}
		})
	}
}

func TestErrorIdentityAndMutation(t *testing.T) {
	for _, row := range []struct {
		combined error
		targets  []error
		text     string
	}{
		{UserNotFoundErr, []error{RecordNotFoundErr, NotFoundHTTPCode}, "user not found: DB: record not found (404)"},
		{OtherResourceUnauthorizedErr, []error{ResourceUnauthorizedErr, UnauthorizedHTTPCode}, "unauthorized call: HTTP client: unauthorized (401)"},
	} {
		if row.combined.Error() != row.text {
			t.Fatal(row.combined)
		}
		for _, target := range row.targets {
			if !errors.Is(row.combined, target) {
				t.Fatalf("lost identity %v", target)
			}
		}
	}
	if AffectedRecordsMismatchErr.Error() != "DB: affected records mismatch" || ResourceNotFoundErr.Error() != "HTTP client: resource not found" {
		t.Fatal("sentinel text changed")
	}
	e := NewApiError(418, 77, "tea", "message", "info", nil)
	cause := errors.New("cause")
	if ProcErr(e, cause) != e || e.Err != cause {
		t.Fatal("legacy mutation/identity changed")
	}
	if ProcErr(e, nil) != e || e.Err != nil {
		t.Fatal("clear cause failed")
	}
}

func TestHTTPResponses(t *testing.T) {
	rows := []struct {
		name    string
		handler iris.Handler
		status  int
		body    string
	}{
		{"ok", OK, 200, `{"code":0,"msg":"success"}`},
		{"nil", func(c iris.Context) { OKWithData(c, nil) }, 200, `{"code":0,"msg":"success"}`},
		{"false", func(c iris.Context) { OKWithData(c, false) }, 200, `{"code":0,"msg":"success","data":false}`},
		{"zero", func(c iris.Context) { OKWithData(c, 0) }, 200, `{"code":0,"msg":"success","data":0}`},
		{"empty", func(c iris.Context) { OKWithData(c, []string{}) }, 200, `{"code":0,"msg":"success","data":[]}`},
		{"typednil", func(c iris.Context) { OKWithData(c, []string(nil)) }, 200, `{"code":0,"msg":"success","data":null}`},
		{"okerror", func(c iris.Context) { OKWithErrMsg(c, Err403, "denied") }, 200, `{"code":10403,"msg":"denied"}`},
		{"okerrornil", func(c iris.Context) { OKWithErrMsg(c, nil, "ignored") }, 200, `{"code":0,"msg":"success"}`},
		{"fail", func(c iris.Context) { Fail(c, Err403) }, 403, `{"code":10403,"msg":"Forbidden"}`},
		{"failmsg", func(c iris.Context) { FailWithMsg(c, Err403, "denied") }, 403, `{"code":10403,"err":"Forbidden","msg":"denied"}`},
		{"pg", func(c iris.Context) { HandlerPgError(c, &pgconn.PgError{Detail: "duplicate"}) }, 500, `{"code":10002,"err":"Database Error","msg":"duplicate"}`},
		{"validationother", func(c iris.Context) { HandlerValidateError(c, errors.New("invalid")) }, 500, `{"code":13011,"err":"Validated Error","msg":"invalid"}`},
		{"validation", func(c iris.Context) {
			type Input struct {
				Name string `validate:"required"`
			}
			HandlerValidateError(c, validator.New().Struct(Input{}))
		}, 500, `{"code":13011,"err":"Validated Error","msg":"Input.Name required"}`},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			app := iris.New()
			app.Get("/", row.handler)
			if err := app.Build(); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if w.Code != row.status || strings.TrimSpace(w.Body.String()) != row.body {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

func TestValidationDetails(t *testing.T) {
	type Input struct {
		Count int `validate:"min=3"`
	}
	err := validator.New().Struct(Input{Count: 1})
	var fields validator.ValidationErrors
	if !errors.As(err, &fields) {
		t.Fatal(err)
	}
	got, err := json.Marshal(wrapValidationErrors(fields))
	if err != nil || string(got) != `[{"tag":"min","namespace":"Input.Count","kind":"int","type":"int","value":"1","param":"3"}]` {
		t.Fatalf("%s %v", got, err)
	}
}
