package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sunweilin/forgify/internal/storage"
)

// ---------- Conversations ----------

func (s *Server) listConversations(w http.ResponseWriter, _ *http.Request) {
	convs, err := s.convSvc.List()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, convs)
}

func (s *Server) listArchivedConversations(w http.ResponseWriter, _ *http.Request) {
	convs, err := s.convSvc.ListArchived()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, convs)
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	conv, err := s.convSvc.Create(req.Title)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, conv)
}

func (s *Server) renameConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.convSvc.Rename(id, req.Title); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) archiveConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.convSvc.Archive(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) restoreConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.convSvc.Restore(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.convSvc.Delete(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) searchConversations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonOK(w, []*struct{}{})
		return
	}
	convs, err := s.convSvc.Search(q)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, convs)
}

func (s *Server) bindConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		AssetID   string `json:"assetId"`
		AssetType string `json:"assetType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.convSvc.Bind(id, req.AssetID, req.AssetType); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unbindConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.convSvc.Unbind(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Messages ----------

type messageResponse struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	ContentType    string    `json:"contentType"`
	Metadata       *string   `json:"metadata,omitempty"`
	ModelID        *string   `json:"modelId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := storage.DB().Query(`
		SELECT id, conversation_id, role, content, content_type, metadata, model_id, created_at
		FROM messages WHERE conversation_id=? ORDER BY created_at ASC`, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var msgs []messageResponse
	for rows.Next() {
		var m messageResponse
		var created string
		var contentType, metadata, modelID *string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&contentType, &metadata, &modelID, &created); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if contentType != nil {
			m.ContentType = *contentType
		} else {
			m.ContentType = "text"
		}
		m.Metadata = metadata
		m.ModelID = modelID
		m.CreatedAt, _ = time.Parse(time.DateTime, created)
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []messageResponse{}
	}
	jsonOK(w, msgs)
}
