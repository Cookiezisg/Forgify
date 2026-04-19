package server

import (
	"encoding/json"
	"net/http"

	"github.com/sunweilin/forgify/internal/service"
)

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := service.ListAPIKeys()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if keys == nil {
		keys = []service.APIKey{}
	}
	jsonOK(w, keys)
}

func (s *Server) saveAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string `json:"id"`
		Provider    string `json:"provider"`
		DisplayName string `json:"displayName"`
		Key         string `json:"key"`
		BaseURL     string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Provider == "" || req.Key == "" {
		jsonError(w, "provider and key are required", http.StatusBadRequest)
		return
	}
	k, err := service.SaveAPIKey(req.ID, req.Provider, req.DisplayName, req.Key, req.BaseURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, k)
}

func (s *Server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := service.DeleteAPIKey(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
		BaseURL  string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	ok, msg, err := service.TestAPIKeyConnection(r.Context(), req.Provider, req.Key, req.BaseURL)
	if err != nil {
		jsonOK(w, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	jsonOK(w, map[string]any{"ok": ok, "message": msg})
}
