package server

import (
	"net/http"

	"github.com/sunweilin/forgify/internal/events"
)

type Server struct {
	mux    *http.ServeMux
	broker *SSEBroker
	Events *events.Bridge
}

func New() *Server {
	broker := newSSEBroker()
	s := &Server{
		mux:    http.NewServeMux(),
		broker: broker,
		Events: events.NewBridge(broker.publish),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
