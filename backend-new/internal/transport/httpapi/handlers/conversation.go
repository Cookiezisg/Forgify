// conversation.go — HTTP handler for /api/v1/conversations/*.
//
// conversation.go — /api/v1/conversations/* 的 HTTP handler。
package handlers

import (
	"net/http"

	"go.uber.org/zap"

	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/pagination"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ConversationHandler serves the 4 /api/v1/conversations/* endpoints.
//
// ConversationHandler 提供 /api/v1/conversations/* 的 4 个端点。
type ConversationHandler struct {
	svc *convapp.Service
	log *zap.Logger
}

// NewConversationHandler wires the handler dependencies.
//
// NewConversationHandler 装配 handler 依赖。
func NewConversationHandler(svc *convapp.Service, log *zap.Logger) *ConversationHandler {
	return &ConversationHandler{svc: svc, log: log}
}

// Register attaches conversation routes.
//
// Register 挂载对话路由。
func (h *ConversationHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/conversations", h.Create)
	mux.HandleFunc("GET /api/v1/conversations", h.List)
	mux.HandleFunc("PATCH /api/v1/conversations/{id}", h.Rename)
	mux.HandleFunc("DELETE /api/v1/conversations/{id}", h.Delete)
}

type createConvRequest struct {
	Title string `json:"title"`
}

type renameConvRequest struct {
	Title string `json:"title"`
}

// Create: POST /api/v1/conversations → 201.
//
// Create：POST /api/v1/conversations → 201。
func (h *ConversationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createConvRequest
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	c, err := h.svc.Create(r.Context(), req.Title)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Created(w, c)
}

// List: GET /api/v1/conversations?cursor=&limit= → 200 paged.
//
// List：GET /api/v1/conversations?cursor=&limit= → 200 分页。
func (h *ConversationHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := pagination.Parse(r)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), convdomain.ListFilter{
		Cursor: p.Cursor,
		Limit:  p.Limit,
	})
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Paged(w, items, next, next != "")
}

// Rename: PATCH /api/v1/conversations/{id} → 200.
//
// Rename：PATCH /api/v1/conversations/{id} → 200。
func (h *ConversationHandler) Rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req renameConvRequest
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	c, err := h.svc.Rename(r.Context(), id, req.Title)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, c)
}

// Delete: DELETE /api/v1/conversations/{id} → 204.
//
// Delete：DELETE /api/v1/conversations/{id} → 204。
func (h *ConversationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.NoContent(w)
}
