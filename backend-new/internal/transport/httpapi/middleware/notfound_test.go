// notfound_test.go — unit tests for the NotFound fallback handler.
//
// notfound_test.go — NotFound fallback handler 的单元测试。
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// envelopeShape is the JSON we expect NotFound to produce.
// envelopeShape 是我们预期 NotFound 产生的 JSON 形状。
type envelopeShape struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestNotFound_ReturnsEnvelope404(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/api/v1/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}

	var env envelopeShape
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code: got %q, want NOT_FOUND", env.Error.Code)
	}
}

func TestNotFound_IncludesPathInMessage(t *testing.T) {
	// The message should tell the client which path failed to match —
	// useful for debugging frontend typos in API paths.
	//
	// 消息应告诉客户端哪条路径没匹配——便于排查前端拼错 API 路径的 bug。
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/api/v1/totally-made-up", nil))

	var env envelopeShape
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if !strings.Contains(env.Error.Message, "/api/v1/totally-made-up") {
		t.Errorf("message should contain path, got: %q", env.Error.Message)
	}
}

func TestNotFound_WorksForAnyMethod(t *testing.T) {
	// 404 is path-based, not method-based; all methods on an unknown path
	// should uniformly produce 404 NOT_FOUND.
	//
	// 404 基于路径而非方法；任何方法访问未知路径都应返回 404 NOT_FOUND。
	methods := []string{"GET", "POST", "PATCH", "PUT", "DELETE"}
	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NotFound(rec, httptest.NewRequest(m, "/missing", nil))

			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: status got %d, want 404", m, rec.Code)
			}
			var env envelopeShape
			_ = json.Unmarshal(rec.Body.Bytes(), &env)
			if env.Error.Code != "NOT_FOUND" {
				t.Errorf("%s: code got %q, want NOT_FOUND", m, env.Error.Code)
			}
		})
	}
}

func TestNotFound_DoesNotLeakGoDefault(t *testing.T) {
	// Regression guard: Go's default 404 body is literally "404 page not found\n".
	// We must not accidentally fall back to that text.
	//
	// 回归守卫：Go 默认 404 响应体是纯文本 "404 page not found\n"。
	// 我们的响应不应该包含这段。
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/x", nil))

	body := rec.Body.String()
	if strings.Contains(body, "404 page not found") {
		t.Errorf("response contains Go's default 404 text, envelope not applied: %q", body)
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "{") {
		t.Errorf("response is not JSON: %q", body)
	}
}
