package middleware

import (
	"net/http"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// NotFound is the handler registered as the router's fallback for URLs
// that don't match any registered route. It produces an N1-compliant
// envelope instead of Go's default plain-text "404 page not found".
//
// It is technically a terminal handler rather than a wrapping middleware,
// but lives in this package because it completes the set of HTTP-level
// cross-cutting concerns (recover, logger, CORS, notfound).
//
// NotFound 作为路由的 fallback handler，处理所有未匹配任何注册路由的 URL。
// 它返回符合 N1 标准的 envelope，而不是 Go 默认的纯文本 "404 page not found"。
//
// 严格说它是终点 handler 而非包裹式中间件，但放在本包里是因为它和
// recover / logger / CORS 同属"HTTP 层横切关注点"。
func NotFound(w http.ResponseWriter, r *http.Request) {
	response.Error(w, http.StatusNotFound,
		"NOT_FOUND",
		"route not found: "+r.URL.Path,
		nil)
}
