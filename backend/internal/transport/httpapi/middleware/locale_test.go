// locale_test.go — tests for InjectLocale middleware.
//
// Round-trip and fallback behavior of SetLocale/GetLocale is in
// reqctx/locale_test.go. Here we verify the middleware's wiring:
// parsing Accept-Language correctly and stamping the right Locale.
//
// locale_test.go — InjectLocale 中间件的测试。
//
// SetLocale/GetLocale 的往返和 fallback 行为在 reqctx/locale_test.go 测过。
// 这里验证中间件本身：是否正确解析 Accept-Language 并塞入对应 Locale。
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func TestInjectLocale_ParsesAcceptLanguage(t *testing.T) {
	cases := []struct {
		header string
		want   reqctx.Locale
	}{
		{"", reqctx.LocaleZhCN},                 // no header → default
		{"zh-CN", reqctx.LocaleZhCN},            // exact
		{"zh-CN,en;q=0.9", reqctx.LocaleZhCN},   // zh primary
		{"en", reqctx.LocaleEn},                 // english
		{"en-US", reqctx.LocaleEn},              // english regional
		{"en-US,en;q=0.9,zh;q=0.8", reqctx.LocaleEn},
		{"EN", reqctx.LocaleEn},                 // case-insensitive
		{"fr-FR", reqctx.LocaleZhCN},            // unsupported → default
		{"de", reqctx.LocaleZhCN},               // unsupported → default
	}

	for _, c := range cases {
		t.Run(c.header, func(t *testing.T) {
			var got reqctx.Locale
			handler := InjectLocale(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = reqctx.GetLocale(r.Context())
			}))

			req := httptest.NewRequest("GET", "/x", nil)
			if c.header != "" {
				req.Header.Set("Accept-Language", c.header)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if got != c.want {
				t.Errorf("Accept-Language %q: got %q, want %q", c.header, got, c.want)
			}
		})
	}
}

func TestInjectLocale_DoesNotAffectResponse(t *testing.T) {
	// Middleware must be transparent to the HTTP response.
	// 中间件对 HTTP 响应必须透明。
	handler := InjectLocale(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("brew"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status: got %d, want 418", rec.Code)
	}
	if rec.Body.String() != "brew" {
		t.Errorf("body: got %q, want \"brew\"", rec.Body.String())
	}
}
