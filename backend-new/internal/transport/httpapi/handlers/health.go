// Package handlers contains per-resource HTTP handlers. Each resource
// (tool, chat, conversation, ...) lives in its own file and is expressed
// as a struct that carries its service dependencies and exposes a
// Register(mux) method to attach its routes.
//
// This pattern keeps routes and their implementations in the same file
// (spatial locality) and makes router.go a thin assembly layer.
//
// Package handlers 持有按资源组织的 HTTP handler。每个资源（tool、chat、
// conversation 等）在独立文件中以 struct 形式存在，struct 持有 service 依赖
// 并暴露 Register(mux) 方法把自己的路由挂到 mux 上。
//
// 这种模式让"路由定义"和"实现"在同一文件里（空间局部性），router.go 变成
// 薄薄的总装层。
package handlers

import (
	"net/http"

	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// HealthHandler serves /api/v1/health. It has no dependencies, but still
// follows the struct + Register pattern for consistency with other handlers.
//
// HealthHandler 提供 /api/v1/health。没有依赖，但仍遵循 struct + Register
// 模式，与其他 handler 保持一致。
type HealthHandler struct{}

// NewHealthHandler constructs a HealthHandler.
// NewHealthHandler 构造一个 HealthHandler。
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Register attaches health routes to the given mux.
// Register 把 health 的路由挂到给定 mux 上。
func (h *HealthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", h.Get)
}

// Get returns {"data": {"status": "ok"}}. Used by Electron after spawning
// the backend subprocess to detect readiness.
//
// Get 返回 {"data": {"status": "ok"}}。Electron 启动后端子进程后
// 调用这个端点检测就绪。
func (h *HealthHandler) Get(w http.ResponseWriter, _ *http.Request) {
	response.Success(w, http.StatusOK, map[string]string{"status": "ok"})
}
