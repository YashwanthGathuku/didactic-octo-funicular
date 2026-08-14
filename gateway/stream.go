package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type StreamEvent struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"` // "FILE_INGESTED", "ANOMALY_DETECTED", "LEDGER_COMMITTED", "SWARM_MESSAGE"
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

type EventBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan StreamEvent]bool
}

var GlobalBroadcaster = &EventBroadcaster{
	clients: make(map[chan StreamEvent]bool),
}

func (b *EventBroadcaster) Subscribe() chan StreamEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan StreamEvent, 10)
	b.clients[ch] = true
	return ch
}

func (b *EventBroadcaster) Unsubscribe(ch chan StreamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

func (b *EventBroadcaster) Broadcast(eventType string, data interface{}) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	event := StreamEvent{
		ID:        fmt.Sprintf("EVT-%d", time.Now().UnixNano()),
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

// RegisterStreamRoutes wires Server-Sent Events into Chi router
func RegisterStreamRoutes(r chi.Router) {
	r.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		ch := GlobalBroadcaster.Subscribe()
		defer GlobalBroadcaster.Unsubscribe(ch)

		// Send initial heartbeat
		initBytes, _ := json.Marshal(map[string]string{"status": "CONNECTED", "stream": "SENTINEL_REALTIME_BUS"})
		fmt.Fprintf(w, "event: connected\ndata: %s\n\n", string(initBytes))
		flusher.Flush()

		notify := r.Context().Done()
		for {
			select {
			case <-notify:
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				dataBytes, _ := json.Marshal(event)
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(dataBytes))
				flusher.Flush()
			}
		}
	})
}
