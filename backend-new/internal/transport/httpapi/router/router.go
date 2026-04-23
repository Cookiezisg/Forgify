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
//	Recover → RequestLogger → CORS → InjectUserID → mux
//
// Why this order:
//   - Recover is outermost so it catches panics from ANY inner layer,
//     including the logger itself if it ever misbehaves.
//   - RequestLogger is just inside recover so the access log can still
//     record the 500 status that recover writes.
//   - CORS sits inside the logger so cross-origin preflight responses are
//     also logged (useful for debugging browser integration).
//   - InjectUserID is innermost (right above the mux) so preflight OPTIONS
//     requests — which terminate inside CORS — don't need a userID. Every
//     request that reaches a business handler has one in its context.
//
// New 构造后端完整的 HTTP handler：路由 + 中间件链 + 404 兜底。main.go 只调
// 一次，结果交给 http.Server。
//
// 中间件顺序（线上从外到内，代码里最后包的是最外层）：
//
//	Recover → RequestLogger → CORS → InjectUserID → mux
//
// 为什么这个顺序：
//   - Recover 在最外层，能捕获**任何**内层的 panic，包括 logger 自己出错。
//   - RequestLogger 紧邻 Recover 内层，使 Recover 写出的 500 状态也能
//     进入访问日志。
//   - CORS 在 logger 内层，跨域 preflight 响应也被记录（利于调试浏览器接入）。
//   - InjectUserID 在最内层（紧贴 mux），这样 preflight OPTIONS 请求——在
//     CORS 那层就被响应结束——不需要 userID。所有到达业务 handler 的请求
//     ctx 中都已带 userID。
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

	// Apply the middleware chain. Assembly is extracted into applyChain
	// so tests can exercise the exact same chain against custom handlers.
	//
	// 应用中间件链。装配逻辑抽到 applyChain，测试可以用**同一条链**
	// 包裹测试用 handler 来验证 ctx 注入等行为。
	return applyChain(mux, deps)
}

// applyChain wraps h with the full middleware chain in the correct order.
// Inside-out assembly: the outermost middleware (Recover) is applied LAST
// so it runs FIRST on every request.
//
// applyChain 按正确顺序把 h 用完整中间件链包裹。从内向外装配：最外层的
// 中间件（Recover）**最后**被应用，因而**最先**在每次请求中执行。
func applyChain(h http.Handler, deps Deps) http.Handler {
	h = middleware.InjectUserID(h)                          // innermost / 最内层
	h = middleware.CORS(middleware.DefaultCORSConfig())(h)
	h = middleware.RequestLogger(deps.Log)(h)
	h = middleware.Recover(deps.Log)(h)                     // outermost / 最外层
	return h
}
