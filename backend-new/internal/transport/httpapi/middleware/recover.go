// Package middleware provides HTTP middlewares wired outside of per-handler
// business logic — recover, request logging, CORS, and 404 fallback.
//
// Package middleware 提供 HTTP 中间件，封装在业务 handler 外层——panic 恢复、
// 请求日志、CORS、404 兜底。
package middleware

import (
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// Recover returns a middleware that converts any handler panic into a 500
// INTERNAL_ERROR envelope response and logs the panic with stack trace.
//
// The returned wrapper must be the OUTERMOST layer in the middleware chain
// so it also catches panics from inner middlewares (e.g. the logger).
//
// The raw panic value is never sent to the client — only "internal server
// error" goes on the wire. The full stack trace is logged at ERROR level
// for observability.
//
// Recover 返回一个中间件：把任何 handler 的 panic 转成 500 INTERNAL_ERROR
// envelope 响应，并通过 zap 记录 panic 值和完整堆栈。
//
// 返回的 wrapper 必须放在中间件链的**最外层**，这样它也能捕获内层中间件
// （如 logger）的 panic。
//
// 原始 panic 值永远不会返回给客户端——对外只返回 "internal server error"，
// 完整堆栈只写入日志供观测。
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
				// Best-effort 500. If the handler already flushed headers
				// before panicking, the write silently fails — we still
				// have the log line.
				//
				// 尽力写 500。如果 handler 在 panic 前已刷出 headers，
				// 这里写不进去会静默失败——但至少日志已经记录了。
				response.Error(w, http.StatusInternalServerError,
					"INTERNAL_ERROR", "internal server error", nil)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
