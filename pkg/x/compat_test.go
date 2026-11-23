package x

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/x/pagination"
	"golang.org/x/crypto/bcrypt"
)

func panics(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected legacy panic")
		}
	}()
	f()
}

func TestStringContracts(t *testing.T) {
	for _, row := range []struct{ name, got, want string }{
		{"remove-zero", RemoveTopNLines("a\nb\nc", 0), "b\nc"},
		{"remove-one", RemoveTopNLines("a\nb\nc", 1), "c"},
		{"remove-negative", RemoveTopNLines("a\nb", -1), "a\nb"},
		{"remove-all", RemoveTopNLines("a", 9), ""},
		{"wrap", CmdlineFormatWrap(" a\n\tb\r\n"), " a\tb\r"},
		{"format", CmdlineFormat(" a\n\tb\r\n"), "a\tb\r"},
		{"replace-all", Replace("aaa", "a", "b"), "bbb"},
		{"replace-first", Replace("aaa", "a", "b", 1), "baa"},
		{"replace-zero", Replace("aaa", "a", "b", 0), "aaa"},
		{"substring", SubStr("abc", 1, 1), "b"},
		{"negative-start", SubStr("abc", -2, 2), "ab"},
		{"past-end", SubStr("abc", 5), ""},
		{"negative-length", SubStr("abc", 1, -2), "bc"},
		{"long-length", SubStr("abc", 1, 9), "bc"},
		{"byte-substring", SubStr("中文", 0, 1), "\xe4"},
		{"join", Join([]string{"a", "", "b"}, ":"), "a::b"},
		{"title", UcWords("hello world"), "Hello World"},
		{"upper", ToUpper("héllo"), "HÉLLO"},
		{"camel", CaseCamelLower("Hello_world-foo.bar"), "helloWorldFooBar"},
		{"camel-number", CaseCamelLower("version12beta"), "version12Beta"},
		{"camel-unicode", CaseCamelLower("中文"), ""},
		{"camel-empty", CaseCamelLower(""), ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			if row.got != row.want {
				t.Fatalf("%q != %q", row.got, row.want)
			}
		})
	}
	if !Equal("AbC", "aBc") || Equal("a", "b") || !reflect.DeepEqual(Split("a,,b", ","), []string{"a", "", "b"}) {
		t.Fatal("comparison or split changed")
	}
}

func TestCollections(t *testing.T) {
	if !InStringArray("a", []string{"b", "a"}) || InStringArray("A", []string{"a"}) || InStringArray("", nil) {
		t.Fatal("string membership changed")
	}
	for _, target := range []any{[]int{1, 2}, [2]int{1, 2}, map[int]string{1: "one", 2: "two"}} {
		if ok, err := Contains(2, target); !ok || err != nil {
			t.Fatalf("%v %v", ok, err)
		}
		if ok, err := Contains(3, target); ok || err == nil || err.Error() != "not in array" {
			t.Fatalf("%v %v", ok, err)
		}
	}
	if ok, err := Contains(1, "one"); ok || err == nil {
		t.Fatal("unsupported type")
	}
	panics(t, func() { _, _ = Contains(1, nil) })
	panics(t, func() { _, _ = Contains("one", map[int]string{1: "one"}) })
	panics(t, func() { _, _ = Contains([]int{1}, [][]int{{1}}) })
}

func TestPaging(t *testing.T) {
	empty, err := TransformPageInfo[int, string](nil, nil)
	b, _ := json.Marshal(empty)
	if err != nil || string(b) != `{"currentPage":1,"pageSize":0,"totalPages":0,"count":0,"hasNextPage":false,"filter":null,"list":[]}` {
		t.Fatalf("%s %v", b, err)
	}
	list := &pagination.List[int]{CurrentPage: 2, PageSize: 2, TotalPages: 4, TotalItems: 7, HasNextPage: true, Filter: "f", Items: []int{1, 2}}
	out, err := TransformPageInfo(list, func(i int) (int, error) { return i * 2, nil })
	b, _ = json.Marshal(out)
	if err != nil || string(b) != `{"currentPage":2,"pageSize":2,"totalPages":4,"count":7,"hasNextPage":false,"filter":"f","list":[2,4]}` {
		t.Fatalf("%s %v", b, err)
	}
	failure := errors.New("transform")
	calls := 0
	out, err = TransformPageInfo(list, func(i int) (int, error) { calls++; return 0, failure })
	if out != nil || err != failure || calls != 1 {
		t.Fatal("must stop at first conversion error")
	}
	list.Items = nil
	out, err = TransformPageInfo(list, func(i int) (int, error) { t.Fatal("empty should not convert"); return 0, nil })
	if err != nil || out.Items == nil || len(out.Items) != 0 {
		t.Fatal("empty result contract")
	}
}

func TestPasswordAndAESContracts(t *testing.T) {
	hash, err := EncodeHash("correct horse")
	cost, _ := bcrypt.Cost([]byte(hash))
	if err != nil || cost != bcrypt.DefaultCost || !CompareHash(hash, "correct horse") || CompareHash(hash, "wrong") || CompareHash("invalid", "x") {
		t.Fatal("bcrypt contract")
	}
	if _, err = EncodeHash(strings.Repeat("a", 73)); err == nil {
		t.Fatal("bcrypt length error missing")
	}
	key := []byte("0123456789abcdef")
	// Standard CBC encryption is an independent producer of the stored-password format.
	block, _ := aes.NewCipher(key)
	padded := append([]byte("hello"), []byte{11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11}...)
	ciphertext := make([]byte, 16)
	cipher.NewCBCEncrypter(block, key).CryptBlocks(ciphertext, padded)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	if encoded != "MOfLtxzZ0YgS4+5cPylFYw==" {
		t.Fatal("standard CBC vector differs from OpenSSL fixture")
	}
	plain, err := DecryptByAes(encoded, key)
	if err != nil || string(plain) != "hello" {
		t.Fatalf("%q %v", plain, err)
	}
	unpacked, err := DecodeEncryptPwd(encoded + "@" + string(key))
	if err != nil || unpacked != "hello" {
		t.Fatalf("%s %v", unpacked, err)
	}
	// The legacy producer decrypts padded input. Keep its bytes until a versioned migration.
	want := make([]byte, 16)
	cipher.NewCBCDecrypter(block, key).CryptBlocks(want, padded)
	got, err := AesEncrypt([]byte("hello"), key)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%x %v", got, err)
	}
	encodedLegacy, err := EncryptByAes([]byte("hello"), key)
	if err != nil || encodedLegacy != "/dlXw0vAgjSsQ4pAqasg/A==" {
		t.Fatal("base64 producer changed")
	}
	for _, input := range []string{"missing", "a@b@c"} {
		if _, err := DecodeEncryptPwd(input); err == nil || err.Error() != "invalid encrypt passwd string" {
			t.Fatal("delimiter boundary")
		}
	}
	if _, err := DecryptByAes("%%%", key); err == nil {
		t.Fatal("base64 error missing")
	}
	if _, err := AesEncrypt(nil, []byte("short")); err == nil {
		t.Fatal("key error missing")
	}
	if _, err := AesDecrypt(nil, key); err == nil || err.Error() != "加密字符串错误！" {
		t.Fatal("empty ciphertext contract")
	}
	if _, err := AesDecrypt(nil, []byte("short")); err == nil {
		t.Fatal("decrypt key error missing")
	}
	panics(t, func() { _, _ = AesDecrypt([]byte("short"), key) })
	panics(t, func() { _, _ = pkcs7UnPadding([]byte{255}) })
}

func TestJWTContracts(t *testing.T) {
	exp := time.Unix(4102444800, 0)
	out, err := GenerateJwtToken("fixture-key", &JwtUser{Id: "42", Username: "alice", Name: "Alice", Roles: []string{"reader"}}, exp)
	if err != nil || out.TokenType != "jwt" || out.ExpiresIn != exp.Unix() {
		t.Fatalf("%+v %v", out, err)
	}
	// Independently produced with Python stdlib HMAC-SHA256.
	const golden = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySWQiOiI0MiIsIlVzZXJuYW1lIjoiYWxpY2UiLCJOYW1lIjoiIiwiUm9sZXMiOlsicmVhZGVyIl0sImV4cCI6NDEwMjQ0NDgwMCwiaXNzIjoiY2lhcy5jbiJ9.mdSnbakHzqkLR5b_6F55g0yZdDY-zlM8DR1V1Xembxs"
	if out.AccessToken != golden {
		t.Fatalf("JWT wire changed: %s", out.AccessToken)
	}
	parts := strings.Split(out.AccessToken, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if string(payload) != `{"UserId":"42","Username":"alice","Name":"","Roles":["reader"],"exp":4102444800,"iss":"cias.cn"}` {
		t.Fatalf("%s", payload)
	}
	claims, err := ParseToken("fixture-key", out.AccessToken)
	if err != nil || claims.UserId != "42" || claims.Username != "alice" || claims.Name != "" || claims.Issuer != "cias.cn" || !reflect.DeepEqual(claims.Roles, []string{"reader"}) {
		t.Fatalf("%+v %v", claims, err)
	}
	for _, token := range []string{"broken", out.AccessToken + "x"} {
		if c, err := ParseToken("fixture-key", token); c != nil || err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	if c, err := ParseToken("wrong-key", out.AccessToken); c != nil || err == nil {
		t.Fatal("wrong signature accepted")
	}
	expired, _ := GenerateJwtToken("fixture-key", &JwtUser{}, time.Unix(1, 0))
	if c, err := ParseToken("fixture-key", expired.AccessToken); c != nil || err == nil {
		t.Fatal("expired token accepted")
	}
	token, err := GenerateToken("fixture-key", "42", "alice", "ignored")
	if token != "" || err != jwt.ErrInvalidKeyType {
		t.Fatalf("legacy signer changed: %q %v", token, err)
	}
}

func TestTokenLookup(t *testing.T) {
	app := iris.New()
	app.Get("/", func(c iris.Context) { _, _ = c.WriteString(GetToken(c, "X-Token")) })
	if err := app.Build(); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ url, header, want string }{
		{"/?X-Token=query&x-token=lower", "header", "header"},
		{"/?X-Token=%20query%20&x-token=lower", "", "query"},
		{"/?x-token=%20lower%20", "", "lower"},
		{"/", "", ""},
	} {
		req := httptest.NewRequest("GET", row.url, nil)
		req.Header.Set("X-Token", row.header)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		if w.Body.String() != row.want {
			t.Fatalf("%q != %q", w.Body, row.want)
		}
	}
}

func TestIPContracts(t *testing.T) {
	for _, row := range []struct{ forward, real, remote, client, public string }{
		{"10.0.0.1, 8.8.8.8", "1.1.1.1", "127.0.0.1:42", "10.0.0.1", "8.8.8.8"},
		{"", " 1.1.1.1 ", "127.0.0.1:42", "1.1.1.1", "1.1.1.1"},
		{"", "", "[2001:4860::1]:42", "2001:4860::1", "2001:4860::1"},
		{"", "", "127.0.0.1:42", "127.0.0.1", ""},
		{"", "", "bad", "", ""},
		{"bad", "", "127.0.0.1:42", "bad", "bad"},
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", row.forward)
		req.Header.Set("X-Real-IP", row.real)
		req.RemoteAddr = row.remote
		if ClientIP(req) != row.client || ClientPublicIP(req) != row.public {
			t.Fatalf("%+v: %s / %s", row, ClientIP(req), ClientPublicIP(req))
		}
	}
	for _, ip := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "172.31.0.1", "169.254.1.1", "192.168.0.1"} {
		if !HasLocalIPAddr(ip) || !HasLocalIP(net.ParseIP(ip)) {
			t.Fatal(ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "172.32.0.1", "fc00::1", "invalid"} {
		if HasLocalIPAddr(ip) {
			t.Fatal(ip)
		}
	}
}

func TestDeviceContracts(t *testing.T) {
	for _, row := range []struct{ agent, header, value, name, platform string }{
		{"", "", "", "normal", ""},
		{"Desktop", "", "", "normal", "desktop"},
		{"Android", "", "", "tablet", "android"},
		{"Android Mobile", "", "", "mobile", "android"},
		{"iPad", "", "", "tablet", "ipad"},
		{"iPhone", "", "", "mobile", "ios"},
		{"Kindle", "", "", "tablet", "Unknown"},
		{"PostmanRuntime/7", "", "", "normal", "postman"},
		{"noki123", "", "", "mobile", "Unknown"},
		{"BlackBerry", "", "", "mobile", "Unknown"},
		{"Desktop", "Accept", "application/vnd.wap.wml", "mobile", "Unknown"},
		{"iPhone", "Profile", "yes", "mobile", "ios"},
		{"abc", "Profile", "yes", "mobile", "Unknown"},
	} {
		t.Run(row.agent+row.header, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", row.agent)
			if row.header != "" {
				req.Header.Set(row.header, row.value)
			}
			d := ResolveDevice(req)
			if d.DeviceName() != row.name || d.Platform() != row.platform || d.Normal() != (row.name == "normal") || d.Mobile() != (row.name == "mobile") || d.Tablet() != (row.name == "tablet") {
				t.Fatalf("%s / %s", d.DeviceName(), d.Platform())
			}
		})
	}
	if GetOsName("") != "Unknown" || GetOsName("Mozilla/5.0 (Windows NT 10.0; Win64; x64)") != "Windows 10" || GetBrowserName("Chrome/120.0 Safari/537.36") != "Chrome/120.0" || GetBrowserName("") != "Unknown" {
		t.Fatal("OS/browser parsing")
	}
}

func TestAuditWire(t *testing.T) {
	if CREATE != 1 || UPDATE != 2 || QUERY != 3 || DELETE != 4 || OTHER != 5 || SUCCESS != 1 || FAILURE != 2 || UNKNOWN != 3 {
		t.Fatal("enum drift")
	}
	b, err := json.Marshal(AuditLog{XRequestId: "r", UserId: "u", UserName: "n", Module: "m", Action: UPDATE, ObjectType: "type", ObjectName: "name", ObjectId: "42", Result: SUCCESS, Details: "d", URI: "/", Params: "{}", Ip: "i", Device: "normal", Platform: "p", Browser: "b", Os: "o", Timestamp: "2026-11-15 12:00:00"})
	want := `{"x_request_id":"r","userid":"u","username":"n","module":"m","action":2,"object_type":"type","object_name":"name","object_id":"42","result":1,"details":"d","uri":"/","params":"{}","ip":"i","device":"normal","platform":"p","browser":"b","os":"o","timestamp":"2026-11-15 12:00:00"}`
	if err != nil || string(b) != want {
		t.Fatalf("%s %v", b, err)
	}
}
