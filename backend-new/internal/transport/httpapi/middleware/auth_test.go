// auth_test.go — unit tests for InjectUserID middleware and
// UserIDFromContext helper.
//
// auth_test.go — InjectUserID 中间件 和 UserIDFromContext 辅助函数的单元测试。
package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInjectUserID_AddsDefaultIDToContext(t *testing.T) {
	// After InjectUserID runs, the downstream handler must be able to read
	// DefaultLocalUserID from ctx via UserIDFromContext.
	//
	// InjectUserID 运行后，下游 handler 必须能通过 UserIDFromContext 从 ctx
	// 读出 DefaultLocalUserID。
	var gotID string
	var gotOK bool
	handler := InjectUserID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, gotOK = UserIDFromContext(r.Context())
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))

	if !gotOK {
		t.Fatalf("UserIDFromContext ok flag: got false, want true")
	}
	if gotID != DefaultLocalUserID {
		t.Errorf("userID: got %q, want %q", gotID, DefaultLocalUserID)
	}
}

func TestUserIDFromContext_MissingMiddlewareReturnsFalse(t *testing.T) {
	// Without InjectUserID in the chain, UserIDFromContext must return
	// ("", false) so handlers can treat it as a wiring bug (500).
	//
	// 没有 InjectUserID 的情况下，UserIDFromContext 必须返回 ("", false)
	// 让 handler 能把它当作接线 bug 处理（返回 500）。
	ctx := context.Background()
	id, ok := UserIDFromContext(ctx)
	if ok {
		t.Errorf("ok: got true, want false for empty context")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestUserIDFromContext_EmptyStringValueReturnsFalse(t *testing.T) {
	// If some misconfigured middleware injected an empty string, we should
	// still report false — handlers must not treat empty userID as valid.
	//
	// 如果有错误配置的中间件注入了空字符串，我们应仍返回 false——handler
	// 不应把空 userID 当成有效值。
	ctx := context.WithValue(context.Background(), userIDKey, "")
	id, ok := UserIDFromContext(ctx)
	if ok {
		t.Errorf("ok: got true for empty string, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestInjectUserID_DoesNotAffectResponse(t *testing.T) {
	// The middleware must be transparent to the HTTP response: whatever the
	// downstream handler writes passes through unchanged.
	//
	// 中间件对 HTTP 响应必须是透明的：下游 handler 写的响应原样透传。
	handler := InjectUserID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func TestUserIDFromContext_PrivateKeyIsolation(t *testing.T) {
	// The ctx key is unexported. Callers who naively use string key
	// "userID" or similar must NOT accidentally collide with our value.
	// This test guards that isolation.
	//
	// ctx key 是未导出的私有类型。外部代码用 string key "userID" 等朴素键时
	// **不得**与我们的值意外命中。本测试守护这一隔离。
	ctx := context.WithValue(context.Background(), "userID", "attacker-injected") //nolint:staticcheck // intentional bad key type
	id, ok := UserIDFromContext(ctx)
	if ok {
		t.Errorf("string-keyed value leaked into private context key: got id=%q", id)
	}
}
