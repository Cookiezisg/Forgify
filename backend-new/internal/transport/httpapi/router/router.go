package router

import (
	"net/http"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/handlers"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// New builds the complete HTTP handler for the backend: routes + middleware
// chain + 404 fallback. main.go calls this exactly once and hands the result
// to http.Server.
//
// Middleware order (outermost first on the wire, last to wrap in code):
//
//	Recover → RequestLogger → CORS → mux
//
// Why this order:
//   - Recover is outermost so it catches panics from ANY inner layer,
//     including the logger itself if it ever misbehaves.
//   - RequestLogger is just inside recover so the access log can still
//     record the 500 status that recover writes.
//   - CORS sits inside the logger so cross-origin preflight responses are
//     also logged (useful for debugging browser integration).
//
// New 构造后端完整的 HTTP handler：路由 + 中间件链 + 404 兜底。main.go 只调
// 一次，结果交给 http.Server。
//
// 中间件顺序（线上从外到内，代码里最后包的是最外层）：
//
//	Recover → RequestLogger → CORS → mux
//
// 为什么这个顺序：
//   - Recover 在最外层，能捕获**任何**内层的 panic，包括 logger 自己出错。
//   - RequestLogger 紧邻 Recover 内层，使 Recover 写出的 500 状态也能
//     进入访问日志。
//   - CORS 在 logger 内层，跨域 preflight 响应也被记录（利于调试浏览器接入）。
func New(deps Deps) http.Handler {
	mux := http.NewServeMux()

	// Each handler registers its own routes.
	// 每个 handler 注册自己的路由。
	handlers.NewHealthHandler().Register(mux)

	// 404 fallback for any URL not matched above. Must be registered LAST
	// so specific routes take precedence.
	//
	// 未匹配的 URL 走 404 fallback。必须**最后**注册，让具体路由优先。
	mux.HandleFunc("/", middleware.NotFound)

	// Wrap mux with middleware chain. Assemble inside-out so the outermost
	// middleware ends up running first on each request.
	//
	// 用中间件链包裹 mux。从里往外装，这样最外层的中间件在每次请求中最先执行。
	var h http.Handler = mux
	h = middleware.CORS(middleware.DefaultCORSConfig())(h)
	h = middleware.RequestLogger(deps.Log)(h)
	h = middleware.Recover(deps.Log)(h) // outermost / 最外层
	return h
}
