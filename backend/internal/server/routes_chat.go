package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConversationID string `json:"conversationId"`
		Message        string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.ConversationID == "" || req.Message == "" {
		jsonError(w, "conversationId and message are required", http.StatusBadRequest)
		return
	}
	if err := s.chatSvc.SendMessage(r.Context(), req.ConversationID, req.Message); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stopGeneration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConversationID string `json:"conversationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	s.chatSvc.StopGeneration(req.ConversationID)
	w.WriteHeader(http.StatusNoContent)
}
