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

// SSEHandler handles Server-Sent Events connections
func SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{ch: make(chan []byte, 64), done: make(chan struct{})}
	sseMu.Lock()
	sseClients[client] = true
	sseMu.Unlock()

	defer func() {
		sseMu.Lock()
		delete(sseClients, client)
		sseMu.Unlock()
		close(client.done)
	}()

	// Initial OK
	fmt.Fprintf(w, ":ok\n\n")
	flusher.Flush()

	for {
		select {
		case msg := <-client.ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
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
