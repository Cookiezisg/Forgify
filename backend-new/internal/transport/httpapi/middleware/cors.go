package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig configures the CORS middleware.
//
// AllowedOrigins is a whitelist — we deliberately do NOT support "*" to
// keep the middleware compatible with future credentialed requests (browsers
// reject "*" + credentials by spec).
//
// CORSConfig 配置 CORS 中间件。
//
// AllowedOrigins 是白名单——我们**不**支持 "*"，以便未来接入带凭证的请求
// （浏览器规范拒绝 "*" + credentials 组合）。
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         time.Duration
}

// DefaultCORSConfig returns sensible defaults for a Forgify-style backend:
// localhost dev origins, all standard REST verbs, common headers.
//
// DefaultCORSConfig 返回 Forgify 这类后端的合理默认：localhost 开发 origin、
// 所有标准 REST 方法、常用请求头。
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

// CORS returns a middleware that handles browser cross-origin requests.
//
// Behavior:
//   - Preflight (OPTIONS + Access-Control-Request-Method header) → 204 with
//     the full set of CORS response headers, request is NOT passed to next.
//   - Non-preflight request with allowed Origin → Access-Control-Allow-Origin
//     is set, request passes through.
//   - Non-preflight request with disallowed Origin → no CORS headers added,
//     request still passes through. The browser (not the server) will block
//     the response from reaching the JS caller.
//   - Request without Origin header (same-origin, curl, Electron loopback)
//     → passes through untouched.
//
// CORS 返回处理浏览器跨域请求的中间件。
//
// 行为：
//   - Preflight（OPTIONS 且含 Access-Control-Request-Method 头）→ 返回 204
//     并附上完整 CORS 响应头，不调用下游 handler。
//   - 带合法 Origin 的普通请求 → 加上 Access-Control-Allow-Origin，透传到下游。
//   - 带非法 Origin 的请求 → 不加任何 CORS 头，仍透传；浏览器（非服务器）
//     会阻止 JS 读取响应。
//   - 无 Origin 头的请求（同源、curl、Electron loopback）→ 原样透传。
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

			// No Origin header → not a browser cross-origin request, skip.
			//
			// 无 Origin 头 → 不是浏览器跨域请求，跳过。
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Origin not in whitelist → omit CORS headers; browser enforces.
			//
			// Origin 不在白名单 → 不加 CORS 头；浏览器端会强制执行。
			if _, ok := allowed[origin]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			// Allowed origin: set the core response header.
			// Vary informs caches the response varies by Origin.
			//
			// 合法 origin：设置核心响应头。Vary 告诉缓存响应因 Origin 而异。
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")

			// Preflight detection: OPTIONS + Access-Control-Request-Method.
			//
			// preflight 检测：OPTIONS 且带 Access-Control-Request-Method 头。
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Non-preflight: pass through with the Origin header attached.
			//
			// 非 preflight：带上 Origin 头透传到下游。
			next.ServeHTTP(w, r)
		})
	}
}
