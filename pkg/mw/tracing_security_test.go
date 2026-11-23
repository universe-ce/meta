package mw

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/kataras/golog"
	"github.com/kataras/iris/v12"
)

func TestRequestLogOmitsCredentialsAndPreservesPayload(t *testing.T) {
	const token = "synthetic-token-never-log"
	const password = "synthetic-password-never-log"
	const cookie = "synthetic-cookie-never-log"
	var output bytes.Buffer
	defer log.SetOutput(io.Discard)
	app := iris.New()
	app.Logger().SetOutput(io.Discard)
	app.Use(TracingWithRuntime(nil))
	app.Post("/account/login", func(c iris.Context) {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil || string(body) != `{"password":"`+password+`"}` {
			t.Error("logging changed the request payload")
		}
		c.Values().Set("res_body", []byte(token))
		c.Values().Set("out_err", password)
		_, _ = c.WriteString("ok")
	})
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/account/login?Authorization="+token, strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("X-Private-Token", token)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	log.SetLevel("info")
	log.SetOutput(&output)
	app.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Error("logging changed the response")
	}
	for _, value := range []string{token, password, cookie} {
		if strings.Contains(output.String(), value) {
			t.Error("request log contains a credential")
		}
	}
	if !strings.Contains(output.String(), "/account/login") || !strings.Contains(output.String(), "status_code") {
		t.Error("safe request metadata was not recorded")
	}
}
