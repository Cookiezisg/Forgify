package server

import (
	"encoding/json"
	"net/http"

	"github.com/sunweilin/forgify/internal/sandbox"
	"github.com/sunweilin/forgify/internal/service"
)

func (s *Server) listTools(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	query := r.URL.Query().Get("q")
	tools, err := s.toolSvc.List(category, query)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, tools)
}

func (s *Server) getTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tool, err := s.toolSvc.Get(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tool == nil {
		jsonError(w, "tool not found", http.StatusNotFound)
		return
	}
	jsonOK(w, tool)
}

func (s *Server) createTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"displayName"`
		Description string   `json:"description"`
		Code        string   `json:"code"`
		Category    string   `json:"category"`
		Requirements []string `json:"requirements"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	tool := &service.Tool{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		Code:         req.Code,
		Category:     req.Category,
		Requirements: req.Requirements,
		Status:       "draft",
	}
	if err := s.toolSvc.Save(tool); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, tool)
}

func (s *Server) updateTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.toolSvc.Get(id)
	if err != nil || existing == nil {
		jsonError(w, "tool not found", http.StatusNotFound)
		return
	}
	if existing.Builtin {
		jsonError(w, "cannot modify built-in tool", http.StatusForbidden)
		return
	}

	var req struct {
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
		Code        string `json:"code"`
		Category    string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	existing.DisplayName = req.DisplayName
	existing.Description = req.Description
	existing.Code = req.Code
	existing.Category = req.Category
	// Re-parse will be done by forge/parser when integrated
	if err := s.toolSvc.Save(existing); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, existing)
}

func (s *Server) deleteTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.toolSvc.Delete(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tool, err := s.toolSvc.Get(id)
	if err != nil || tool == nil {
		jsonError(w, "tool not found", http.StatusNotFound)
		return
	}

	var req struct {
		Params map[string]any `json:"params"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Params == nil {
		req.Params = map[string]any{}
	}

	result, err := sandbox.Run(r.Context(), tool.Code, tool.Name, tool.Requirements, req.Params, 0)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save test record
	passed := result.Error == ""
	inputJSON, _ := json.Marshal(req.Params)
	outputJSON, _ := json.Marshal(result.Output)
	s.toolSvc.SaveTestRecord(&service.ToolTestRecord{
		ToolID:     id,
		Passed:     passed,
		DurationMs: result.DurationMs,
		InputJSON:  string(inputJSON),
		OutputJSON: string(outputJSON),
		ErrorMsg:   result.Error,
	})
	s.toolSvc.UpdateTestResult(id, passed)

	jsonOK(w, result)
}

func (s *Server) listToolTestHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	history, err := s.toolSvc.ListTestHistory(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, history)
}

func (s *Server) exportTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	data, err := s.toolSvc.Export(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=tool.forgify-tool")
	w.Write(data)
}

func (s *Server) importToolParse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	result, err := s.toolSvc.ParseImport(req.Data)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, result)
}

func (s *Server) importToolConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data   json.RawMessage `json:"data"`
		Action string          `json:"action"` // "new", "rename", "replace"
		ReplaceID string       `json:"replaceId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	tool, err := s.toolSvc.ConfirmImport(req.Data, req.Action, req.ReplaceID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, tool)
}
