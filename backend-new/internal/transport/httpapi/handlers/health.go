// Package handlers contains per-resource HTTP handlers. Each resource
// is a struct carrying its service dependencies, with a Register(mux)
// method that attaches its routes. router/ calls Register on each to
// assemble the full HTTP API.
//
// Package handlers 按资源组织 HTTP handler。每个资源是一个持有 service
// 依赖的 struct，通过 Register(mux) 把路由挂到 mux 上。router/ 调用每个
// handler 的 Register 组装完整 HTTP API。
package handlers

import (
	"net/http"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// HealthHandler serves /api/v1/health. Used by Electron after spawning
// the backend subprocess to detect readiness.
//
// HealthHandler 提供 /api/v1/health。Electron 启动后端子进程后
// 用它检测就绪。
type HealthHandler struct{}

// NewHealthHandler constructs a HealthHandler.
//
// NewHealthHandler 构造一个 HealthHandler。
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Register attaches health routes to mux.
//
// Register 把 health 路由挂到 mux 上。
func (h *HealthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", h.Get)
}

// Get returns {"data": {"status": "ok"}}.
//
// Get 返回 {"data": {"status": "ok"}}。
func (h *HealthHandler) Get(w http.ResponseWriter, _ *http.Request) {
	response.Success(w, http.StatusOK, map[string]string{"status": "ok"})
}
