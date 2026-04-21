package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

type sseClient struct {
	ch   chan []byte
	done chan struct{}
}

var (
	sseClients = make(map[*sseClient]bool)
	sseMu      sync.RWMutex
)

// SSEHandler handles Server-Sent Events connections — sends keepalive only
func SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, ":ok\n\n")
	flusher.Flush()
	// Keep connection open with periodic keepalive
	<-r.Context().Done()
}

// Broadcast sends an event to all connected SSE clients
func Broadcast(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Debug("sse broadcast marshal failed", "err", err)
		return
	}
	sseMu.RLock()
	defer sseMu.RUnlock()
	for client := range sseClients {
		select {
		case client.ch <- data:
		default:
			// Client too slow, skip
		}
	}
}

// EventPush handles agent stats push (POST /events)
func EventPush(w http.ResponseWriter, r *http.Request) {
	var event map[string]any
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		jsonErr(w, 400, "invalid event data")
		return
	}
	Broadcast(event)
	w.WriteHeader(http.StatusAccepted)
}
