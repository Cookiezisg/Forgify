// router_test.go — integration tests verifying all 4 middlewares + the
// fallback handler + registered routes all wire up correctly together.
//
// router_test.go — 集成测试，验证 4 个中间件 + 404 fallback + 已注册路由
// 全部正确串联工作。
package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// newTestDeps returns a Deps with a no-op logger so tests are quiet.
//
// newTestDeps 返回带 no-op logger 的 Deps，让测试输出安静。
func newTestDeps() Deps {
	return Deps{Log: zap.NewNop()}
}

func TestRouter_HealthEndpointReturnsEnvelope(t *testing.T) {
	h := New(newTestDeps())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}

	var env struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env.Data.Status != "ok" {
		t.Errorf("status: got %q, want ok", env.Data.Status)
	}
}

func TestRouter_UnknownPathReturnsEnvelope404(t *testing.T) {
	// Regression: before the refactor, unknown paths returned Go's default
	// plain-text "404 page not found". Must be envelope JSON now.
	//
	// 回归：重构前未知路径返回 Go 默认纯文本 "404 page not found"。
	// 现在必须是 envelope JSON。
	h := New(newTestDeps())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/totally-nonexistent", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "404 page not found") {
		t.Errorf("leaked Go's default 404 body: %s", rec.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Errorf("error code: got %q, want NOT_FOUND", env.Error.Code)
	}
}

func TestRouter_CORSPreflightWorks(t *testing.T) {
	// Verify CORS middleware is in the chain: an OPTIONS preflight from an
	// allowed dev origin should return 204 with CORS headers.
	//
	// 验证 CORS 中间件已接入：来自白名单的 dev origin 发 OPTIONS preflight，
	// 应返回 204 + 全套 CORS 头。
	h := New(newTestDeps())
	req := httptest.NewRequest("OPTIONS", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("CORS middleware not wired: missing Allow-Origin")
	}
}

func TestRouter_CORSHeaderPresentOnHealthRequest(t *testing.T) {
	// Non-preflight GET from allowed origin should pass through AND carry
	// the Access-Control-Allow-Origin header.
	//
	// 来自白名单 origin 的普通 GET 应透传，并携带 Allow-Origin 头。
	h := New(newTestDeps())
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("missing Allow-Origin on passed-through request")
	}
}

func TestRouter_UserIDInjectedIntoHandlerContext(t *testing.T) {
	// Handlers reached via applyChain must see DefaultLocalUserID in ctx.
	// If someone removes InjectUserID from the chain, this test fails
	// even though all existing /health tests pass.
	//
	// 通过 applyChain 到达的 handler 必须能从 ctx 读出 DefaultLocalUserID。
	// 如果有人从链里撤掉 InjectUserID，即使 /health 测试都通过，这条也会失败。
	var gotID string
	var gotOK bool
	testHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, gotOK = reqctx.GetUserID(r.Context())
	})

	h := applyChain(testHandler, newTestDeps())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/anything", nil))

	if !gotOK {
		t.Fatalf("GetUserID ok: got false — InjectUserID not wired")
	}
	if gotID != reqctx.DefaultLocalUserID {
		t.Errorf("userID: got %q, want %q", gotID, reqctx.DefaultLocalUserID)
	}
}

func TestRouter_LocaleInjectedIntoHandlerContext(t *testing.T) {
	// Handlers reached via applyChain must see a parsed Locale in ctx.
	// Guards wiring: removing InjectLocale from the chain fails this.
	//
	// 通过 applyChain 到达的 handler 必须能从 ctx 读出解析好的 Locale。
	// 守护接线：从链里撤掉 InjectLocale 会让本测试失败。
	var gotLocale reqctx.Locale
	testHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotLocale = reqctx.GetLocale(r.Context())
	})

	h := applyChain(testHandler, newTestDeps())
	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotLocale != reqctx.LocaleEn {
		t.Errorf("locale: got %q, want %q", gotLocale, reqctx.LocaleEn)
	}
}
