// cors_test.go — unit tests for the CORS middleware.
//
// cors_test.go — CORS 中间件的单元测试。
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// testCORSConfig is a small fixed config used by all cases for determinism.
//
// testCORSConfig 是所有用例共用的固定配置，保证测试确定性。
func testCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"http://allowed.example"},
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         1 * time.Hour,
	}
}

// okHandler is a trivial downstream handler used to verify pass-through.
//
// okHandler 是简单的下游 handler，用于验证透传。
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestCORS_AllowedOriginAddsHeader(t *testing.T) {
	h := CORS(testCORSConfig())(okHandler())

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://allowed.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.example" {
		t.Errorf("Allow-Origin: got %q, want http://allowed.example", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary: got %q, want Origin", got)
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("request did not pass through: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCORS_DisallowedOriginOmitsHeader(t *testing.T) {
	h := CORS(testCORSConfig())(okHandler())

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin should be absent for disallowed origin, got %q", got)
	}
	// Still passes through — browser, not server, enforces.
	//
	// 仍然透传——由浏览器而非服务器强制。
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("request should pass through: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCORS_NoOriginHeaderSkipsCORS(t *testing.T) {
	// Same-origin requests, curl, Electron IPC — no Origin header.
	// Middleware should be a no-op.
	//
	// 同源请求、curl、Electron IPC 都没 Origin 头。中间件应透明透传。
	h := CORS(testCORSConfig())(okHandler())

	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin should be absent, got %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
}

func TestCORS_PreflightReturns204WithFullHeaders(t *testing.T) {
	h := CORS(testCORSConfig())(okHandler())

	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://allowed.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.example" {
		t.Errorf("Allow-Origin: got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PATCH, DELETE, OPTIONS" {
		t.Errorf("Allow-Methods: got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("Allow-Headers: got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("Max-Age: got %q, want 3600", got)
	}
	// Downstream must NOT be called for preflight.
	//
	// 下游 handler 不应该被 preflight 调用。
	if rec.Body.String() != "" {
		t.Errorf("preflight should not reach downstream, got body %q", rec.Body.String())
	}
}

func TestCORS_OPTIONSWithoutPreflightHeadersPassesThrough(t *testing.T) {
	// A bare OPTIONS request (no Access-Control-Request-Method) is NOT a
	// preflight — it might be a legitimate HTTP OPTIONS for the resource.
	// Should pass through to downstream.
	//
	// 纯 OPTIONS 请求（无 Access-Control-Request-Method）不是 preflight，
	// 可能是合法的资源 OPTIONS 请求。应透传到下游。
	h := CORS(testCORSConfig())(okHandler())

	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://allowed.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200 (passed through)", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body: got %q, want ok (downstream reached)", rec.Body.String())
	}
}

func TestCORS_PreflightOriginNotAllowedStillBlocks(t *testing.T) {
	// Preflight from a disallowed origin: middleware omits CORS headers,
	// so the browser will reject. We still pass through (browser enforces).
	//
	// preflight 但 origin 不在白名单：中间件不加 CORS 头，浏览器会拒绝。
	// 我们仍透传（浏览器强制执行）。
	h := CORS(testCORSConfig())(okHandler())

	req := httptest.NewRequest("OPTIONS", "/x", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin should be absent, got %q", got)
	}
	// Passes through — downstream decides what to do.
	//
	// 透传——下游决定做什么。
	if rec.Code != http.StatusOK {
		t.Errorf("should pass through, got status %d", rec.Code)
	}
}

func TestDefaultCORSConfig_Sanity(t *testing.T) {
	c := DefaultCORSConfig()
	if len(c.AllowedOrigins) == 0 {
		t.Error("DefaultCORSConfig has no origins")
	}
	// Must not contain "*" — we banned it by design.
	//
	// 绝不允许包含 "*"——设计上禁止。
	for _, o := range c.AllowedOrigins {
		if o == "*" {
			t.Errorf("DefaultCORSConfig should not contain '*'")
		}
	}
	if c.MaxAge <= 0 {
		t.Error("DefaultCORSConfig MaxAge should be positive")
	}
}
