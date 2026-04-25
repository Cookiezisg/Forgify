package middleware

import (
	"net/http"

	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// InjectUserID is the Phase 2 simplified auth middleware: stamps
// DefaultLocalUserID into ctx. Will be rewritten to parse real auth
// credentials (JWT / session) later.
//
// InjectUserID 是 Phase 2 的简化 auth 中间件：把 DefaultLocalUserID
// 塞入 ctx。未来重写为解析真实凭证（JWT / session）。
func InjectUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := reqctx.SetUserID(r.Context(), reqctx.DefaultLocalUserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
