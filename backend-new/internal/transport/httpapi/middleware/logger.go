package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// RequestLogger emits one structured log line per request with method,
// path, status, response bytes, and elapsed time. Must be placed INSIDE
// Recover so access logs reflect 500 responses written by Recover.
//
// RequestLogger 每次请求打一行结构化日志：方法、路径、状态、响应字节数、
// 耗时。必须放在 Recover **内层**，访问日志才能反映 Recover 写的 500。
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

// statusRecorder wraps ResponseWriter to remember the final status code
// and written byte count for logging.
//
// statusRecorder 包装 ResponseWriter，记录最终状态码和写入字节数供日志读取。
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	// Default 200: a handler that only Writes (no WriteHeader) implies 200 OK.
	// 默认 200：只 Write 不 WriteHeader 的 handler 相当于隐式 200。
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader records the first call's status; subsequent calls are ignored
// (double WriteHeader is a handler bug).
//
// WriteHeader 只记首次调用的状态码；重复调用被忽略（重复 WriteHeader 是 handler bug）。
func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}
