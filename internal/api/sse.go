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
	// Store stats from agent
	storeAgentStats(event)
	Broadcast(event)
	w.WriteHeader(http.StatusAccepted)
}

// Agent stats cache
var agentStatsCache = struct {
	sync.RWMutex
	nodes map[string]map[string]any
}{nodes: make(map[string]map[string]any)}

func storeAgentStats(event map[string]any) {
	// Agent sends: {"id": "nodeId", "cpu": {...}, "memory": {...}, "disk": {...}, ...}
	id, ok := event["id"].(string)
	if !ok || id == "" {
		return
	}
	agentStatsCache.Lock()
	agentStatsCache.nodes[id] = event
	agentStatsCache.Unlock()
}

func getAgentStats() (cpuUsage, memUsage float64, memUsed int64, diskUsage float64, diskUsed, diskTotal int64) {
	agentStatsCache.RLock()
	defer agentStatsCache.RUnlock()
	n := len(agentStatsCache.nodes)
	if n == 0 {
		return
	}
	for _, stats := range agentStatsCache.nodes {
		if cpu, ok := stats["cpu"].(map[string]any); ok {
			if v, ok := cpu["usedPercentage"].(float64); ok {
				cpuUsage += v
			}
		}
		if mem, ok := stats["memory"].(map[string]any); ok {
			if v, ok := mem["usedPercentage"].(float64); ok {
				memUsage += v
			}
			if v, ok := mem["used"].(float64); ok {
				memUsed += int64(v)
			}
		}
		if disk, ok := stats["disk"].(map[string]any); ok {
			if v, ok := disk["usedPercentage"].(float64); ok {
				diskUsage += v
			}
			if v, ok := disk["used"].(float64); ok {
				diskUsed += int64(v)
			}
			if v, ok := disk["total"].(float64); ok {
				diskTotal += int64(v)
			}
		}
	}
	cpuUsage /= float64(n)
	memUsage /= float64(n)
	diskUsage /= float64(n)
	return
}
