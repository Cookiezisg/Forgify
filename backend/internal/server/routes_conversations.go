package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/storage"
)

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	rows, err := storage.DB().Query(`
		SELECT id, title, status, created_at, updated_at
		FROM conversations WHERE status='active'
		ORDER BY updated_at DESC LIMIT 100`)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &c.Status, &created, &updated); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.CreatedAt, _ = time.Parse(time.DateTime, created)
		c.UpdatedAt, _ = time.Parse(time.DateTime, updated)
		convs = append(convs, c)
	}
	if convs == nil {
		convs = []Conversation{}
	}
	jsonOK(w, convs)
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Title == "" {
		req.Title = "新对话"
	}
	id := uuid.NewString()
	if _, err := storage.DB().Exec(
		`INSERT INTO conversations (id, title) VALUES (?, ?)`, id, req.Title,
	); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, Conversation{ID: id, Title: req.Title, Status: "active"})
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	storage.DB().Exec(
		`UPDATE conversations SET status='archived', updated_at=datetime('now') WHERE id=?`, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := storage.DB().Query(`
		SELECT id, conversation_id, role, content, created_at
		FROM messages WHERE conversation_id=? ORDER BY created_at ASC`, id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &created); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m.CreatedAt, _ = time.Parse(time.DateTime, created)
		msgs = append(msgs, m)
	}
	if msgs == nil {
		msgs = []Message{}
	}
	jsonOK(w, msgs)
}
