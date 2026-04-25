// model.go — HTTP handler for /api/v1/model-configs/*.
// Thin: decode JSON → call modelapp.Service → envelope via response package.
//
// model.go — /api/v1/model-configs/* 的 HTTP handler。
// 薄层：解 JSON → 调 modelapp.Service → 通过 response 包输出 envelope。
package handlers

import (
	"net/http"

	"go.uber.org/zap"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ModelConfigHandler serves the 2 /api/v1/model-configs/* endpoints.
//
// ModelConfigHandler 提供 /api/v1/model-configs/* 的 2 个端点。
type ModelConfigHandler struct {
	svc *modelapp.Service
	log *zap.Logger
}

// NewModelConfigHandler wires the handler dependencies.
//
// NewModelConfigHandler 装配 handler 依赖。
func NewModelConfigHandler(svc *modelapp.Service, log *zap.Logger) *ModelConfigHandler {
	return &ModelConfigHandler{svc: svc, log: log}
}

// Register attaches model-config routes.
//
// Register 挂载 model-config 路由。
func (h *ModelConfigHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/model-configs", h.List)
	mux.HandleFunc("PUT /api/v1/model-configs/{scenario}", h.Upsert)
}

type upsertModelRequest struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// List: GET /api/v1/model-configs → 200 with all active configs (no pagination).
//
// List：GET /api/v1/model-configs → 200 返回所有活跃配置（不分页）。
func (h *ModelConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, items)
}

// Upsert: PUT /api/v1/model-configs/{scenario} → 200 with the created/updated config.
//
// Upsert：PUT /api/v1/model-configs/{scenario} → 200 返回新建或更新后的配置。
func (h *ModelConfigHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	scenario := r.PathValue("scenario")
	var req upsertModelRequest
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	m, err := h.svc.Upsert(r.Context(), scenario, modelapp.UpsertInput{
		Provider: req.Provider,
		ModelID:  req.ModelID,
	})
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, m)
}
