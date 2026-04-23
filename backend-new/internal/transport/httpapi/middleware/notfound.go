package middleware

import (
	"net/http"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// NotFound is the router's fallback handler for unmatched URLs.
// Produces an N1-compliant envelope instead of Go's default plain
// "404 page not found" text.
//
// NotFound 是路由的 fallback handler，处理未匹配 URL。返回符合 N1 的
// envelope，取代 Go 默认的纯文本 "404 page not found"。
func NotFound(w http.ResponseWriter, r *http.Request) {
	response.Error(w, http.StatusNotFound,
		"NOT_FOUND",
		"route not found: "+r.URL.Path,
		nil)
}
