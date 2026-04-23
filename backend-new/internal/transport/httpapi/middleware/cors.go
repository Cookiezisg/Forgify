package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig configures the CORS middleware. AllowedOrigins is a strict
// whitelist — "*" is deliberately unsupported so the middleware stays
// compatible with credentialed requests (browsers reject "*" + credentials).
//
// CORSConfig 配置 CORS 中间件。AllowedOrigins 是严格白名单——**不支持** "*"，
// 保持与带凭证请求兼容（浏览器规范拒绝 "*" + credentials）。
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         time.Duration
}

// DefaultCORSConfig returns Forgify's standard dev/prod origins and REST
// methods.
//
// DefaultCORSConfig 返回 Forgify 标准的 dev/prod origin 和 REST 方法。
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{
			"http://localhost:5173", // Vite dev server
			"http://localhost:3000",
			"http://127.0.0.1:5173",
			"http://127.0.0.1:3000",
		},
		AllowedMethods: []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Requested-With"},
		MaxAge:         24 * time.Hour,
	}
}

// CORS returns a middleware handling browser cross-origin requests:
//   - Preflight (OPTIONS + Access-Control-Request-Method) → 204 with
//     full CORS headers; does NOT call next.
//   - Allowed Origin on normal request → adds Allow-Origin, passes through.
//   - Disallowed Origin → no CORS headers; passes through (browser blocks).
//   - No Origin header → pass through untouched.
//
// CORS 返回处理跨域请求的中间件：
//   - Preflight（OPTIONS + Access-Control-Request-Method）→ 204 + 全套
//     CORS 头，**不**调用 next。
//   - 合法 Origin 普通请求 → 加 Allow-Origin 透传。
//   - 非法 Origin → 不加 CORS 头透传（由浏览器拦截）。
//   - 无 Origin 头 → 原样透传。
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowed[o] = struct{}{}
	}
	allowMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	maxAge := strconv.Itoa(int(cfg.MaxAge.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
