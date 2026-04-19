package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type SSEBroker struct {
	clients map[chan string]struct{}
	mu      sync.RWMutex
}

func newSSEBroker() *SSEBroker {
	return &SSEBroker{clients: make(map[chan string]struct{})}
}

func (b *SSEBroker) publish(event string, payload any) {
	data, _ := json.Marshal(payload)
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *SSEBroker) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
	}()

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
