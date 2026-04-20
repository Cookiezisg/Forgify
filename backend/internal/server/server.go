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
	convSvc *service.ConversationService
	toolSvc *service.ToolService
}

func New() *Server {
	broker := newSSEBroker()
	bridge := events.NewBridge(broker.publish)

	keyProvider := func(provider string) (key, baseURL string, err error) {
		return service.GetRawKeyForProvider(provider)
	}
	gateway := model.New(keyProvider, bridge)
	convSvc := service.NewConversationService(gateway, bridge)
	toolSvc := service.NewToolService()

	s := &Server{
		mux:     http.NewServeMux(),
		broker:  broker,
		Events:  bridge,
		chatSvc: service.NewChatService(gateway, bridge, convSvc, toolSvc),
		convSvc: convSvc,
		toolSvc: toolSvc,
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
	s.mux.HandleFunc("GET /api/conversations/archived", s.listArchivedConversations)
	s.mux.HandleFunc("GET /api/conversations/search", s.searchConversations)
	s.mux.HandleFunc("GET /api/asset-conversations/{assetId}", s.listConversationsByAsset)
	s.mux.HandleFunc("POST /api/conversations", s.createConversation)
	s.mux.HandleFunc("PATCH /api/conversations/{id}/rename", s.renameConversation)
	s.mux.HandleFunc("PATCH /api/conversations/{id}/archive", s.archiveConversation)
	s.mux.HandleFunc("PATCH /api/conversations/{id}/restore", s.restoreConversation)
	s.mux.HandleFunc("PATCH /api/conversations/{id}/bind", s.bindConversation)
	s.mux.HandleFunc("PATCH /api/conversations/{id}/unbind", s.unbindConversation)
	s.mux.HandleFunc("DELETE /api/conversations/{id}", s.deleteConversation)
	s.mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)

	// Chat
	s.mux.HandleFunc("POST /api/chat/send", s.sendMessage)
	s.mux.HandleFunc("POST /api/chat/stop", s.stopGeneration)
	s.mux.HandleFunc("POST /api/conversations/{id}/compact", s.fullCompact)

	// Attachments
	s.mux.HandleFunc("POST /api/attachments/upload", s.handleUploadAttachment)

	// Tools
	s.mux.HandleFunc("GET /api/tools", s.listTools)
	s.mux.HandleFunc("POST /api/tools", s.createTool)
	s.mux.HandleFunc("GET /api/tools/{id}", s.getTool)
	s.mux.HandleFunc("PUT /api/tools/{id}", s.updateTool)
	s.mux.HandleFunc("DELETE /api/tools/{id}", s.deleteTool)
	s.mux.HandleFunc("POST /api/tools/{id}/run", s.runTool)
	s.mux.HandleFunc("GET /api/tools/{id}/test-history", s.listToolTestHistory)
	s.mux.HandleFunc("GET /api/tools/{id}/pending", s.getToolPendingChange)
	s.mux.HandleFunc("POST /api/tools/{id}/accept", s.acceptPendingChange)
	s.mux.HandleFunc("POST /api/tools/{id}/reject", s.rejectPendingChange)
	s.mux.HandleFunc("PATCH /api/tools/{id}/meta", s.updateToolMeta)
	s.mux.HandleFunc("GET /api/tools/{id}/tags", s.listToolTags)
	s.mux.HandleFunc("POST /api/tools/{id}/tags", s.addToolTag)
	s.mux.HandleFunc("DELETE /api/tools/{id}/tags/{tag}", s.removeToolTag)
	s.mux.HandleFunc("GET /api/tools/{id}/versions", s.listToolVersions)
	s.mux.HandleFunc("POST /api/tools/{id}/versions/{v}/restore", s.restoreToolVersion)
	s.mux.HandleFunc("GET /api/tools/{id}/test-cases", s.listToolTestCases)
	s.mux.HandleFunc("POST /api/tools/{id}/test-cases", s.saveToolTestCase)
	s.mux.HandleFunc("DELETE /api/test-cases/{id}", s.deleteToolTestCase)
	s.mux.HandleFunc("GET /api/tools/{id}/export", s.exportTool)
	s.mux.HandleFunc("POST /api/tools/import/parse", s.importToolParse)
	s.mux.HandleFunc("POST /api/tools/import/confirm", s.importToolConfirm)

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
