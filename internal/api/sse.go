package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/ccvass/swarmpit-xpx/internal/docker"
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
	}()

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
	storeAgentStats(event)
	// Build dashboard data and broadcast to SSE clients
	go broadcastDashboard()
	w.WriteHeader(http.StatusAccepted)
}

func broadcastDashboard() {
	cpuU, memU, memUsed, diskU, diskUsed, diskTotal := getAgentStats()
	nodes, _ := docker.Nodes()
	totalCPU := 0.0
	totalMem := int64(0)
	resources := map[string]map[string]any{}
	for _, n := range nodes {
		if n.Status.State != "ready" { continue }
		cpu := float64(n.Description.Resources.NanoCPUs) / 1e9
		mem := n.Description.Resources.MemoryBytes
		totalCPU += cpu
		totalMem += mem
		resources[n.ID] = map[string]any{"cores": cpu, "memory": mem}
	}
	stats := map[string]any{
		"resources": resources,
		"cpu":    map[string]any{"usage": cpuU, "cores": totalCPU},
		"memory": map[string]any{"usage": memU, "used": memUsed, "total": totalMem},
		"disk":   map[string]any{"usage": diskU, "used": diskUsed, "total": diskTotal},
	}
	Broadcast(map[string]any{"stats": stats})
}

// Agent stats cache
var agentStatsCache = struct {
	sync.RWMutex
	nodes map[string]map[string]any
	tasks map[string]map[string]any
}{nodes: make(map[string]map[string]any), tasks: make(map[string]map[string]any)}

func storeAgentStats(event map[string]any) {
	// Agent sends: {"type": "stats", "message": {"id": "nodeId", "cpu": {...}, "memory": {...}, "disk": {...}}}
	// Or directly: {"id": "nodeId", "cpu": {...}, ...}
	msg := event
	if m, ok := event["message"].(map[string]any); ok {
		msg = m
	}
	id, ok := msg["id"].(string)
	if !ok || id == "" {
		return
	}

	agentStatsCache.Lock()
	// Store node-level stats
	nodeStats := map[string]any{}
	if cpu, ok := msg["cpu"].(map[string]any); ok {
		nodeStats["cpu"] = cpu
	}
	if mem, ok := msg["memory"].(map[string]any); ok {
		nodeStats["memory"] = mem
	}
	if disk, ok := msg["disk"].(map[string]any); ok {
		nodeStats["disk"] = disk
	}
	agentStatsCache.nodes[id] = nodeStats

	// Store per-task stats from agent
	if tasks, ok := msg["tasks"].([]any); ok {
		for _, t := range tasks {
			if tm, ok := t.(map[string]any); ok {
				if tid, ok := tm["id"].(string); ok && tid != "" {
					agentStatsCache.tasks[tid] = tm
				}
			}
		}
	}
	agentStatsCache.Unlock()
}

// getNodeStatsCache returns a copy of node stats keyed by node ID
func getNodeStatsCache() map[string]map[string]any {
	agentStatsCache.RLock()
	defer agentStatsCache.RUnlock()
	r := make(map[string]map[string]any, len(agentStatsCache.nodes))
	for k, v := range agentStatsCache.nodes {
		r[k] = v
	}
	return r
}

// getTaskStats returns stats for a specific task, or nil
func getTaskStats(taskID string) any {
	agentStatsCache.RLock()
	defer agentStatsCache.RUnlock()
	if s, ok := agentStatsCache.tasks[taskID]; ok {
		return s
	}
	return nil
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
