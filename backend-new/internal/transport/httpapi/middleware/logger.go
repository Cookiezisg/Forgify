package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// RequestLogger returns a middleware that emits one structured log line per
// HTTP request with method, path, status, response bytes, and elapsed time.
//
// Wiring order: must be placed INSIDE Recover so that panics turned into 500
// responses by Recover are still reflected in the access log.
//
// RequestLogger 返回一个中间件：每次 HTTP 请求结束后打一行结构化日志，
// 包含方法、路径、状态码、响应字节数、耗时。
//
// 装配顺序：必须放在 Recover 内层，这样 Recover 把 panic 转成 500 后，
// 访问日志里能正确反映这个 500 状态。
func RequestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newStatusRecorder(w)

			next.ServeHTTP(rec, r)

			log.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rec.status),
				zap.Int("bytes", rec.bytes),
				zap.Int64("elapsed_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

// statusRecorder wraps an http.ResponseWriter so the middleware can observe
// the final status code and number of body bytes written by the handler.
//
// Go's stock ResponseWriter doesn't expose these after the fact, so we hook
// WriteHeader and Write to remember them.
//
// statusRecorder 包装 http.ResponseWriter，让中间件能在 handler 结束后
// 观测到最终状态码和响应体字节数。
//
// Go 原生 ResponseWriter 不保留这些信息，所以我们拦截 WriteHeader 和 Write
// 来记住它们。
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

// newStatusRecorder defaults status to 200 because a handler that writes a
// body without calling WriteHeader implicitly sets 200 OK (Go behavior).
//
// newStatusRecorder 默认 status=200，因为直接 Write 而不调 WriteHeader 的
// handler 相当于隐式 200 OK（Go 的默认行为）。
func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader records the status on the FIRST call only. Subsequent calls are
// an application bug (Go logs a warning at runtime) but we don't crash.
//
// WriteHeader 只在**首次**调用时记录状态。重复调用是应用层 bug（Go 运行时
// 会打 warning），但我们不让它 crash。
func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Write forwards to the underlying writer and accumulates the byte count.
// A Write before WriteHeader promotes status to 200 (Go's behavior).
//
// Write 转发到底层 writer，并累加字节数。在没调 WriteHeader 就 Write
// 的情况下，状态自动提升为 200（Go 行为）。
func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}
