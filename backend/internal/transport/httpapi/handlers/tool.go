// tool.go — HTTP handler for /api/v1/tools/*. Thin: decode → service → envelope.
//
// tool.go — /api/v1/tools/* 的 HTTP handler。薄层：解码 → service → envelope。
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	tooldomain "github.com/sunweilin/forgify/backend/internal/domain/tool"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/pagination"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ToolHandler serves the 22 /api/v1/tools/* endpoints.
//
// ToolHandler 提供 /api/v1/tools/* 的 22 个端点。
type ToolHandler struct {
	svc *toolapp.Service
	log *zap.Logger
}

// NewToolHandler wires handler dependencies.
//
// NewToolHandler 装配 handler 依赖。
func NewToolHandler(svc *toolapp.Service, log *zap.Logger) *ToolHandler {
	return &ToolHandler{svc: svc, log: log}
}

// Register attaches all tool routes to mux.
//
// Register 把所有工具路由挂载到 mux。
func (h *ToolHandler) Register(mux *http.ServeMux) {
	// Collection
	mux.HandleFunc("POST /api/v1/tools", h.Create)
	mux.HandleFunc("GET /api/v1/tools", h.List)
	mux.HandleFunc("POST /api/v1/tools:import", h.Import)

	// Resource
	mux.HandleFunc("GET /api/v1/tools/{id}", h.Get)
	mux.HandleFunc("PATCH /api/v1/tools/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/tools/{id}", h.Delete)

	// Resource actions (:run, :export, :revert, :test, :generate-test-cases)
	mux.HandleFunc("POST /api/v1/tools/{idAction}", h.postOnTool)

	// Versions
	mux.HandleFunc("GET /api/v1/tools/{id}/versions", h.ListVersions)
	mux.HandleFunc("GET /api/v1/tools/{id}/versions/{version}", h.GetVersion)

	// Pending
	mux.HandleFunc("GET /api/v1/tools/{id}/pending", h.GetPending)
	mux.HandleFunc("POST /api/v1/tools/{id}/pending:accept", h.AcceptPending)
	mux.HandleFunc("POST /api/v1/tools/{id}/pending:reject", h.RejectPending)

	// Test cases
	mux.HandleFunc("GET /api/v1/tools/{id}/test-cases", h.ListTestCases)
	mux.HandleFunc("POST /api/v1/tools/{id}/test-cases", h.CreateTestCase)
	mux.HandleFunc("DELETE /api/v1/tools/{id}/test-cases/{tcId}", h.DeleteTestCase)
	mux.HandleFunc("POST /api/v1/tools/{id}/test-cases/{tcIdAction}", h.postOnTestCase)

	// History
	mux.HandleFunc("GET /api/v1/tools/{id}/run-history", h.ListRunHistory)
	mux.HandleFunc("GET /api/v1/tools/{id}/test-history", h.ListTestHistory)
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (h *ToolHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Code        string   `json:"code"`
		Tags        []string `json:"tags"`
	}
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Create(r.Context(), toolapp.CreateInput{
		Name: req.Name, Description: req.Description,
		Code: req.Code, Tags: req.Tags,
	})
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Created(w, t)
}

func (h *ToolHandler) List(w http.ResponseWriter, r *http.Request) {
	p, err := pagination.Parse(r)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), tooldomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit})
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Paged(w, items, next, next != "")
}

func (h *ToolHandler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, t)
}

func (h *ToolHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Tags        *[]string `json:"tags"`
		Code        *string   `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Update(r.Context(), r.PathValue("id"), toolapp.UpdateInput{
		Name: req.Name, Description: req.Description, Tags: req.Tags, Code: req.Code,
	})
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, t)
}

func (h *ToolHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.NoContent(w)
}

// ── Import / Export ───────────────────────────────────────────────────────────

func (h *ToolHandler) Import(w http.ResponseWriter, r *http.Request) {
	var data json.RawMessage
	if err := decodeJSON(r, &data); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.Import(r.Context(), []byte(data))
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Created(w, t)
}

// ── Resource action dispatcher ────────────────────────────────────────────────

// postOnTool dispatches POST /api/v1/tools/{idAction} based on the action suffix.
//
// postOnTool 按 action 后缀分派 POST /api/v1/tools/{idAction}。
func (h *ToolHandler) postOnTool(w http.ResponseWriter, r *http.Request) {
	idAction := r.PathValue("idAction")
	id, action, ok := strings.Cut(idAction, ":")
	if !ok {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action", nil)
		return
	}
	switch action {
	case "run":
		h.Run(w, r, id)
	case "export":
		h.Export(w, r, id)
	case "revert":
		h.Revert(w, r, id)
	case "test":
		h.RunAllTests(w, r, id)
	case "generate-test-cases":
		h.GenerateTestCases(w, r, id)
	default:
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action: "+action, nil)
	}
}

func (h *ToolHandler) Run(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Input map[string]any `json:"input"`
	}
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	result, err := h.svc.RunTool(r.Context(), id, req.Input)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, result)
}

func (h *ToolHandler) Export(w http.ResponseWriter, r *http.Request, id string) {
	data, err := h.svc.Export(r.Context(), id)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *ToolHandler) Revert(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Version int `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	t, err := h.svc.RevertToVersion(r.Context(), id, req.Version)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, t)
}

func (h *ToolHandler) RunAllTests(w http.ResponseWriter, r *http.Request, id string) {
	results, err := h.svc.RunAllTests(r.Context(), id)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	total, passed := len(results), 0
	for _, r := range results {
		if r.Pass != nil && *r.Pass {
			passed++
		}
	}
	response.Success(w, http.StatusOK, map[string]any{
		"total": total, "passed": passed, "failed": total - passed, "results": results,
	})
}

// GenerateTestCases streams AI-generated test cases via SSE.
//
// GenerateTestCases 通过 SSE 流式推送 AI 生成的测试用例。
func (h *ToolHandler) GenerateTestCases(w http.ResponseWriter, r *http.Request, id string) {
	count := 5
	if s := r.URL.Query().Get("count"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 20 {
			count = n
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flush, canFlush := w.(http.Flusher)

	emit := func(e toolapp.GenerateEvent) {
		b, _ := json.Marshal(e)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if canFlush {
			flush.Flush()
		}
	}
	if err := h.svc.GenerateTestCases(r.Context(), id, count, emit); err != nil {
		h.log.Error("generate test cases", zap.Error(err))
	}
}

// ── Versions ──────────────────────────────────────────────────────────────────

func (h *ToolHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.svc.ListVersions(r.Context(), r.PathValue("id"))
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, versions)
}

func (h *ToolHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "version must be an integer", nil)
		return
	}
	version, err := h.svc.GetVersion(r.Context(), r.PathValue("id"), v)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, version)
}

// ── Pending ───────────────────────────────────────────────────────────────────

func (h *ToolHandler) GetPending(w http.ResponseWriter, r *http.Request) {
	pending, err := h.svc.GetActivePending(r.Context(), r.PathValue("id"))
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, pending)
}

func (h *ToolHandler) AcceptPending(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.AcceptPending(r.Context(), r.PathValue("id"))
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, t)
}

func (h *ToolHandler) RejectPending(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RejectPending(r.Context(), r.PathValue("id")); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.NoContent(w)
}

// ── Test cases ────────────────────────────────────────────────────────────────

func (h *ToolHandler) ListTestCases(w http.ResponseWriter, r *http.Request) {
	cases, err := h.svc.ListTestCases(r.Context(), r.PathValue("id"))
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, cases)
}

func (h *ToolHandler) CreateTestCase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		InputData      string `json:"inputData"`
		ExpectedOutput string `json:"expectedOutput"`
	}
	if err := decodeJSON(r, &req); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	tc, err := h.svc.CreateTestCase(r.Context(), r.PathValue("id"), toolapp.TestCaseInput{
		Name: req.Name, InputData: req.InputData, ExpectedOutput: req.ExpectedOutput,
	})
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Created(w, tc)
}

func (h *ToolHandler) DeleteTestCase(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteTestCase(r.Context(), r.PathValue("tcId")); err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.NoContent(w)
}

// postOnTestCase dispatches POST /api/v1/tools/{id}/test-cases/{tcIdAction}.
//
// postOnTestCase 分派 POST /api/v1/tools/{id}/test-cases/{tcIdAction}。
func (h *ToolHandler) postOnTestCase(w http.ResponseWriter, r *http.Request) {
	tcIdAction := r.PathValue("tcIdAction")
	tcID, action, ok := strings.Cut(tcIdAction, ":")
	if !ok || action != "run" {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "unknown action", nil)
		return
	}
	result, err := h.svc.RunTestCase(r.Context(), tcID, "")
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, result)
}

// ── History ───────────────────────────────────────────────────────────────────

func (h *ToolHandler) ListRunHistory(w http.ResponseWriter, r *http.Request) {
	p, err := pagination.Parse(r)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	items, err := h.svc.ListRunHistoryForTool(r.Context(), r.PathValue("id"), p.Limit)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, items)
}

func (h *ToolHandler) ListTestHistory(w http.ResponseWriter, r *http.Request) {
	p, err := pagination.Parse(r)
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	batchID := r.URL.Query().Get("batchId")
	var items any
	if batchID != "" {
		items, err = h.svc.ListTestHistoryByBatch(r.Context(), batchID)
	} else {
		items, err = h.svc.ListTestHistoryForTool(r.Context(), r.PathValue("id"), p.Limit)
	}
	if err != nil {
		response.FromDomainError(w, h.log, err)
		return
	}
	response.Success(w, http.StatusOK, items)
}
