package mw

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kataras/iris/v12"
	irisctx "github.com/kataras/iris/v12/context"
	"github.com/swaggo/swag"
	"github.com/universe-ce/meta/v3/pkg/m"
	"github.com/universe-ce/meta/v3/pkg/x"
)

func build(t *testing.T, app *iris.Application) {
	t.Helper()
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthCompatibility(t *testing.T) {
	valid, _ := x.GenerateJwtToken("key", &x.JwtUser{Id: "42", Username: "alice", Roles: []string{"reader"}}, time.Unix(4102444800, 0))
	expired, _ := x.GenerateJwtToken("key", &x.JwtUser{}, time.Unix(1, 0))
	for _, row := range []struct {
		name, path, token string
		existing, allowed bool
	}{
		{"login", "/api/v2/uaa/account/login", "", false, true},
		{"valid", "/private", "Bearer " + valid.AccessToken, false, true},
		{"query", "/private?Token=" + valid.AccessToken, "", false, true},
		{"existing", "/private", valid.AccessToken, true, true},
		{"missing", "/private", "", false, false},
		{"expired", "/private", expired.AccessToken, false, false},
		{"invalid", "/private", "broken", false, false},
		{"login-suffix", "/api/v2/uaa/account/login/extra", "", false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			app := iris.New()
			calls := 0
			app.Use(func(c iris.Context) {
				if row.existing {
					_ = c.SetUser(&irisctx.SimpleUser{ID: "old", Username: "existing"})
				}
				c.Next()
			}, AuthHandler("Token", "key"))
			handler := func(c iris.Context) {
				calls++
				if c.Path() != "/api/v2/uaa/account/login" {
					id, _ := c.User().GetID()
					name, _ := c.User().GetUsername()
					wantID, wantName := "42", "alice"
					if row.existing {
						wantID, wantName = "old", "existing"
					}
					if id != wantID || name != wantName {
						t.Errorf("%s / %s", id, name)
					}
					if !row.existing {
						roles, _ := c.User().GetRoles()
						if len(roles) != 1 || roles[0] != "reader" {
							t.Error(roles)
						}
					}
				}
				_, _ = c.WriteString("next")
			}
			app.Get("/private", handler)
			app.Get("/api/v2/uaa/account/login", handler)
			app.Get("/api/v2/uaa/account/login/extra", handler)
			build(t, app)
			req := httptest.NewRequest("GET", row.path, nil)
			req.Header.Set("Token", row.token)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			want := `{"code":401,"msg":"Token已过期"}`
			if row.allowed {
				want = "next"
			}
			if w.Code != 200 || strings.TrimSpace(w.Body.String()) != want || (calls == 1) != row.allowed {
				t.Fatalf("%d %s calls=%d", w.Code, w.Body, calls)
			}
		})
	}
}

func TestErrorsMiddleware(t *testing.T) {
	for _, row := range []struct {
		name    string
		handler iris.Handler
		status  int
		body    string
	}{
		{"normal", func(c iris.Context) { _, _ = c.WriteString("ok") }, 200, "ok"},
		{"panic", func(c iris.Context) { panic("boom") }, 200, `{"code":500,"msg":"InternalServerError"}`},
		{"stopped", func(c iris.Context) { c.StopWithText(409, "stopped"); panic("boom") }, 409, "stopped"},
	} {
		t.Run(row.name, func(t *testing.T) {
			app := iris.New()
			app.Use(ErrorsHandler())
			app.Get("/", row.handler)
			build(t, app)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if w.Code != row.status || strings.TrimSpace(w.Body.String()) != row.body {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
}

func TestTracingCompatibility(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "PATCH"} {
		for _, enable := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", method, enable), func(t *testing.T) {
				app := iris.New()
				calls := 0
				app.Use(TracingHandler(enable, "test", ""))
				app.Handle(method, "/", func(c iris.Context) {
					calls++
					b, _ := io.ReadAll(c.Request().Body)
					_, _ = c.Write(b)
				})
				build(t, app)
				w := httptest.NewRecorder()
				app.ServeHTTP(w, httptest.NewRequest(method, "/?v=1", strings.NewReader(`{"value":false}`)))
				if calls != 1 || w.Body.String() != `{"value":false}` {
					t.Fatalf("%d %s", calls, w.Body)
				}
			})
		}
	}
}

func TestAuditIDAndEnums(t *testing.T) {
	for _, row := range []struct {
		raw, want string
		valid     bool
	}{
		{`"42"`, "42", true}, {`42`, "42", true}, {`42.9`, "42", true}, {`-2`, "-2", true},
		{`"001"`, "001", true}, {`null`, "", false}, {`true`, "", false}, {`{}`, "", false}, {`broken`, "", false},
	} {
		var id m.InterfaceID
		err := json.Unmarshal([]byte(row.raw), &id)
		if (err == nil) != row.valid || id.Value != row.want {
			t.Fatalf("%s => %q, %v", row.raw, id.Value, err)
		}
	}
	for _, row := range []struct {
		method string
		want   x.OperateType
	}{
		{"GET", x.QUERY}, {"POST", x.CREATE}, {"PUT", x.UPDATE}, {"DELETE", x.DELETE}, {"PATCH", x.OTHER},
	} {
		if getTargetAction(row.method) != row.want {
			t.Fatal(row.method)
		}
	}
	if getResult(200) != x.SUCCESS || getResult(201) != x.FAILURE || getResult(500) != x.FAILURE {
		t.Fatal("result contract")
	}
	if recoverSwaggerPath("/project/123/item/42") != "/project/{projectId:string}/item/42" {
		t.Fatal("path contract")
	}
}

type testSwagger struct{ raw string }

func (s *testSwagger) ReadDoc() string { return s.raw }

var swaggerOnce sync.Once
var swaggerFixture = &testSwagger{raw: `{}`}

func TestAuditSwagger(t *testing.T) {
	// Register once; other cases exercise fallback without modifying the registry.
	swaggerOnce.Do(func() { swag.Register(swag.Name, swaggerFixture) })
	doc := swaggerFixture
	previous := doc.raw
	t.Cleanup(func() { doc.raw = previous })
	doc.raw = `{"paths":{"/project/{projectId:string}/items":{"put":{"summary":"修改条目","tags":["资源接口"]},"post":{"summary":"创建","tags":[]}}}}`
	if kind, name := getTargetObjectName("/project/{projectId:string}/items", "put"); kind != "资源" || name != "修改条目" {
		t.Fatalf("%s %s", kind, name)
	}
	if kind, name := getTargetObjectName("/project/{projectId:string}/items", "post"); kind != "" || name != "创建" {
		t.Fatalf("%s %s", kind, name)
	}
	if kind, name := getTargetObjectName("/missing", "patch"); kind != "/missing" || name != "patch" {
		t.Fatalf("%s %s", kind, name)
	}
	doc.raw = "invalid"
	if kind, name := getTargetObjectName("/broken", "post"); kind != "/broken" || name != "post" {
		t.Fatalf("%s %s", kind, name)
	}
	doc.raw = `{}`
}

func TestAuditHTTPAndSnapshot(t *testing.T) {
	messages := make(chan *x.AuditLog, 20)
	app := iris.New()
	app.Use(func(c iris.Context) { _ = c.SetUser(&irisctx.SimpleUser{ID: "42", Username: "alice"}); c.Next() },
		AuditHandler(func(msg *x.AuditLog) { messages <- msg }, func(p string) bool { return p == "/skip" }, "fixture"))
	handler := func(c iris.Context) {
		b, _ := io.ReadAll(c.Request().Body)
		if string(b) != `{"id":123}` {
			t.Errorf("body consumed: %s", b)
		}
		c.StatusCode(200)
		_, _ = c.WriteString("ok")
	}
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
		app.Handle(method, "/items", handler)
	}
	app.Post("/skip", handler)
	app.Post("/created", func(c iris.Context) { c.StatusCode(201) })
	build(t, app)
	for i, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "POST"} {
		path := "/items?id=987"
		if i == 5 {
			path = "/skip"
		}
		req := httptest.NewRequest(method, path, strings.NewReader(`{"id":123}`))
		req.Header.Set("X-Request-Id", fmt.Sprint(i))
		req.Header.Set("User-Agent", "PostmanRuntime/7")
		req.Header.Set("X-Real-IP", "8.8.8.8")
		app.ServeHTTP(httptest.NewRecorder(), req)
	}
	seen := map[string]bool{}
	for range 4 {
		select {
		case msg := <-messages:
			seen[msg.XRequestId] = true
			wantID := ""
			if msg.Action == x.UPDATE {
				wantID = "123"
			}
			if msg.Action == x.DELETE {
				wantID = "987"
			}
			if msg.UserId != "42" || msg.UserName != "alice" || msg.Module != "fixture" || msg.ObjectId != wantID || msg.Params != `{"id":123}` || msg.URI != "/items" || msg.Result != x.SUCCESS || msg.Ip != "8.8.8.8" || msg.Device != "normal" || msg.Platform != "postman" {
				t.Fatalf("%+v", msg)
			}
			if _, err := time.Parse("2006-01-02 15:04:05", msg.Timestamp); err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("audit message missing")
		}
	}
	if len(seen) != 4 || seen["0"] || seen["5"] {
		t.Fatal(seen)
	}
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/created", nil))
	select {
	case msg := <-messages:
		if msg.Result != x.FAILURE {
			t.Fatal(msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("created audit missing")
	}
	select {
	case msg := <-messages:
		t.Fatalf("unexpected message %+v", msg)
	default:
	}
}

func TestAuditConcurrentRequests(t *testing.T) {
	const n = 30
	messages := make(chan *x.AuditLog, n)
	release := make(chan struct{})
	app := iris.New()
	app.Use(func(c iris.Context) { _ = c.SetUser(&irisctx.SimpleUser{ID: c.GetHeader("X-Request-Id")}); c.Next() },
		AuditHandler(func(msg *x.AuditLog) { <-release; messages <- msg }, func(string) bool { return false }, "test"))
	app.Put("/", func(c iris.Context) { c.StatusCode(http.StatusOK) })
	build(t, app)
	var group sync.WaitGroup
	for i := range n {
		group.Add(1)
		go func() {
			defer group.Done()
			id := fmt.Sprint(i)
			req := httptest.NewRequest("PUT", "/", strings.NewReader(`{"id":"`+id+`"}`))
			req.Header.Set("X-Request-Id", id)
			app.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	group.Wait()
	close(release)
	seen := map[string]bool{}
	for range n {
		select {
		case msg := <-messages:
			if msg.UserId != msg.XRequestId || msg.ObjectId != msg.XRequestId || seen[msg.XRequestId] {
				t.Fatalf("%+v", msg)
			}
			seen[msg.XRequestId] = true
		case <-time.After(3 * time.Second):
			t.Fatal("missing audit")
		}
	}
}

func TestAuditDisabled(t *testing.T) {
	for _, middleware := range []iris.Handler{AuditHandler(nil, func(string) bool { return false }, "test"), AuditHandler(func(*x.AuditLog) { t.Error("should not send") }, nil, "test")} {
		app := iris.New()
		app.Use(middleware)
		app.Post("/", func(c iris.Context) { _, _ = c.WriteString("ok") })
		build(t, app)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
		if w.Body.String() != "ok" {
			t.Fatal(w.Body)
		}
	}
}
