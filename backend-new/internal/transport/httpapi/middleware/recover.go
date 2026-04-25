// Package middleware provides HTTP middlewares wired around per-handler
// business logic — recover, request logging, CORS, locale/user injection,
// and 404 fallback.
//
// Package middleware 提供 HTTP 中间件——panic 恢复、请求日志、CORS、
// locale / userID 注入、404 兜底。
package middleware

import (
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// Recover catches handler panics and returns a 500 INTERNAL_ERROR
// envelope. Must be the OUTERMOST middleware so it also catches panics
// from inner middlewares. The raw panic value is logged with stack
// trace, never sent to the client.
//
// Recover 捕获 handler 的 panic 并返回 500 INTERNAL_ERROR envelope。
// 必须放在中间件链**最外层**以覆盖内层 panic。panic 原值仅记入日志
// （含堆栈），绝不返回给客户端。
func Recover(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				log.Error("panic recovered",
					zap.Any("panic", rec),
					zap.String("stack", string(debug.Stack())),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
				// Best-effort 500. If headers already flushed, write fails silently.
				// 尽力写 500。若 header 已刷出，此写入静默失败。
				response.Error(w, http.StatusInternalServerError,
					"INTERNAL_ERROR", "internal server error", nil)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
