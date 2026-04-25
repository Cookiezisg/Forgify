// auth_test.go — tests for InjectUserID middleware.
//
// The round-trip and isolation behavior of SetUserID/GetUserID is tested
// in reqctx/userid_test.go. Here we focus on the middleware's wiring:
// that it actually stamps DefaultLocalUserID into ctx before forwarding.
//
// auth_test.go — InjectUserID 中间件的测试。
//
// SetUserID/GetUserID 的往返和隔离行为在 reqctx/userid_test.go 测过。
// 这里只验证中间件本身的接线：它确实在转发前把 DefaultLocalUserID
// 塞进了 ctx。
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func TestInjectUserID_StampsDefaultIntoContext(t *testing.T) {
	var gotID string
	var gotOK bool
	handler := InjectUserID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, gotOK = reqctx.GetUserID(r.Context())
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))

	if !gotOK {
		t.Fatalf("userID not set by middleware")
	}
	if gotID != reqctx.DefaultLocalUserID {
		t.Errorf("userID: got %q, want %q", gotID, reqctx.DefaultLocalUserID)
	}
}

func TestInjectUserID_DoesNotAffectResponse(t *testing.T) {
	// Middleware must be transparent — whatever the handler writes
	// passes through unchanged.
	//
	// 中间件对响应必须透明——handler 写什么就原样透传。
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
