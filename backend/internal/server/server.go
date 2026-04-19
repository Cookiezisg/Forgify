package server

import (
	"encoding/json"
	"net/http"

	"github.com/sunweilin/forgify/internal/events"
	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/service"
)

type Server struct {
	mux     *http.ServeMux
	broker  *SSEBroker
	Events  *events.Bridge
	chatSvc *service.ChatService
}

func New() *Server {
	broker := newSSEBroker()
	bridge := events.NewBridge(broker.publish)

	keyProvider := func(provider string) (key, baseURL string, err error) {
		return service.GetRawKeyForProvider(provider)
	}
	gateway := model.New(keyProvider, bridge)

	s := &Server{
		mux:     http.NewServeMux(),
		broker:  broker,
		Events:  bridge,
		chatSvc: service.NewChatService(gateway, bridge),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /events", s.broker.handleSSE)

	// API Keys
	s.mux.HandleFunc("GET /api/api-keys", s.listAPIKeys)
	s.mux.HandleFunc("POST /api/api-keys", s.saveAPIKey)
	s.mux.HandleFunc("DELETE /api/api-keys/{id}", s.deleteAPIKey)
	s.mux.HandleFunc("POST /api/api-keys/test", s.testAPIKey)

	// Conversations
	s.mux.HandleFunc("GET /api/conversations", s.listConversations)
	s.mux.HandleFunc("POST /api/conversations", s.createConversation)
	s.mux.HandleFunc("DELETE /api/conversations/{id}", s.deleteConversation)
	s.mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)

	// Chat
	s.mux.HandleFunc("POST /api/chat/send", s.sendMessage)
	s.mux.HandleFunc("POST /api/chat/stop", s.stopGeneration)

	// Models
	s.mux.HandleFunc("GET /api/models", s.listModels)
	s.mux.HandleFunc("GET /api/model-config", s.getModelConfig)
	s.mux.HandleFunc("POST /api/model-config", s.saveModelConfig)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
