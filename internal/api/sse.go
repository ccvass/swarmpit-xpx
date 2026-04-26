package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
)

// SSE subscription routing — each client subscribes to a specific handler

type sseSubscription struct {
	Handler string // e.g. "service-list", "service-info", "index"
	Params  map[string]string
}

type sseClient struct {
	ch   chan []byte
	done chan struct{}
	sub  sseSubscription
}

var (
	sseClients = make(map[*sseClient]bool)
	sseMu      sync.RWMutex
)

// parseEDNSubscription parses a Clojure EDN subscription string like:
// {:handler :service-list, :params {}}
// {:handler :service-info, :params {:id "image-puller"}}
func parseEDNSubscription(edn string) sseSubscription {
	sub := sseSubscription{Params: map[string]string{}}
	// Extract handler
	re := regexp.MustCompile(`:handler\s+:([a-z-]+)`)
	if m := re.FindStringSubmatch(edn); len(m) > 1 {
		sub.Handler = m[1]
	}
	// Extract params — simple key-value pairs like :id "value" or :serviceName "value"
	pre := regexp.MustCompile(`:(\w+)\s+"([^"]*)"`)
	// Find params section
	if idx := strings.Index(edn, ":params"); idx >= 0 {
		paramsStr := edn[idx:]
		for _, m := range pre.FindAllStringSubmatch(paramsStr, -1) {
			sub.Params[m[1]] = m[2]
		}
	}
	return sub
}

// SSEHandler handles Server-Sent Events connections with subscription routing
func SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Parse subscription from query params
	subB64 := r.URL.Query().Get("subscription")
	var sub sseSubscription
	if subB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(subB64)
		if err != nil {
			// Try URL-safe base64
			decoded, _ = base64.URLEncoding.DecodeString(subB64)
		}
		if len(decoded) > 0 {
			sub = parseEDNSubscription(string(decoded))
		}
	}

	slog.Debug("sse client connected", "handler", sub.Handler, "params", sub.Params)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{ch: make(chan []byte, 16), done: make(chan struct{}), sub: sub}
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

	// Send initial data immediately
	if data := fetchSubscriptionData(sub); data != nil {
		if msg, err := json.Marshal(data); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}

	// Periodic refresh ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-client.ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			if data := fetchSubscriptionData(sub); data != nil {
				if msg, err := json.Marshal(data); err == nil {
					fmt.Fprintf(w, "data: %s\n\n", msg)
					flusher.Flush()
				}
			}
		case <-r.Context().Done():
			return
		}
	}
}

// fetchSubscriptionData returns the data for a given subscription
func fetchSubscriptionData(sub sseSubscription) any {
	info, _ := docker.Info()
	switch sub.Handler {
	case "index":
		return fetchDashboardData()
	case "service-list":
		services, _ := docker.Services()
		tasks, _ := docker.Tasks()
		networks, _ := docker.Networks()
		return mapServices(services, tasks, networks, info)
	case "stack-list":
		services, _ := docker.Services()
		tasks, _ := docker.Tasks()
		networks, _ := docker.Networks()
		return extractStacks(services, tasks, networks, info)
	case "node-list":
		nodes, _ := docker.Nodes()
		return mapNodes(nodes)
	case "task-list":
		tasks, _ := docker.Tasks()
		services, _ := docker.Services()
		nodes, _ := docker.Nodes()
		return mapTasks(tasks, nodes, services, info)
	default:
		// Don't send SSE data for unknown handlers — let API calls handle it
		// This prevents the frontend :default handler from corrupting form state
		return nil
	}
}

// limitTimeseries trims each series to the last N points to reduce SSE payload
func limitTimeseries(series []map[string]any, maxPoints int) []map[string]any {
	for i, s := range series {
		if times, ok := s["time"].([]string); ok && len(times) > maxPoints {
			offset := len(times) - maxPoints
			series[i]["time"] = times[offset:]
			if cpus, ok := s["cpu"].([]float64); ok { series[i]["cpu"] = cpus[offset:] }
			if mems, ok := s["memory"].([]float64); ok { series[i]["memory"] = mems[offset:] }
		}
	}
	return series
}

// clusterStats returns aggregated cluster stats (same as dashboard)
func clusterStats() map[string]any {
	nodes, _ := docker.Nodes()
	cache := getNodeStatsCache()
	totalCPU := 0.0
	totalMem := int64(0)
	resources := map[string]map[string]any{}
	cpuSum, memSum, diskSum := 0.0, 0.0, 0.0
	memUsed, diskUsed, diskTotal := int64(0), int64(0), int64(0)
	nc := 0
	for _, nd := range nodes {
		if nd.Status.State != "ready" { continue }
		cpu := float64(nd.Description.Resources.NanoCPUs) / 1e9
		mem := nd.Description.Resources.MemoryBytes
		totalCPU += cpu
		totalMem += mem
		resources[nd.ID] = map[string]any{"cores": cpu, "memory": mem}
		if s, ok := cache[nd.ID]; ok {
			nc++
			if c, ok := s["cpu"].(map[string]any); ok {
				if v, ok := c["usedPercentage"].(float64); ok { cpuSum += v }
			}
			if m, ok := s["memory"].(map[string]any); ok {
				if v, ok := m["usedPercentage"].(float64); ok { memSum += v }
				if v, ok := m["used"].(float64); ok { memUsed += int64(v) }
			}
			if d, ok := s["disk"].(map[string]any); ok {
				if v, ok := d["usedPercentage"].(float64); ok { diskSum += v }
				if v, ok := d["used"].(float64); ok { diskUsed += int64(v) }
				if v, ok := d["total"].(float64); ok { diskTotal += int64(v) }
			}
		}
	}
	cpuAvg, memAvg, diskAvg := 0.0, 0.0, 0.0
	if nc > 0 { cpuAvg = cpuSum / float64(nc); memAvg = memSum / float64(nc); diskAvg = diskSum / float64(nc) }
	return map[string]any{
		"resources": resources,
		"cpu":    map[string]any{"usage": cpuAvg, "cores": totalCPU},
		"memory": map[string]any{"usage": memAvg, "used": memUsed, "total": totalMem},
		"disk":   map[string]any{"usage": diskAvg, "used": diskUsed, "total": diskTotal},
	}
}

// extractStacks builds stack list from services (same logic as StackList handler)
func extractStacks(svcs []swarm.Service, tasks []swarm.Task, nets []types.NetworkResource, info system.Info) []map[string]any {
	stacks := map[string]bool{}
	for _, s := range svcs {
		ns := s.Spec.Labels["com.docker.stack.namespace"]
		if ns != "" { stacks[ns] = true }
	}
	result := []map[string]any{}
	for name := range stacks {
		result = append(result, mapStack(name, svcs, tasks, nets, info))
	}
	return result
}

// fetchDashboardData returns full dashboard data
func fetchDashboardData() map[string]any {
	services, _ := docker.Services()
	tasks, _ := docker.Tasks()
	networks, _ := docker.Networks()
	info, _ := docker.Info()
	nodes, _ := docker.Nodes()
	return map[string]any{
		"stats":              clusterStats(),
		"services":           mapServices(services, tasks, networks, info),
		"nodes":              mapNodes(nodes),
		"nodes-ts":           limitTimeseries(getHostTimeseries(), 100),
		"services-ts-cpu":    limitTimeseries(getServiceTimeseries("cpu"), 100),
		"services-ts-memory": limitTimeseries(getServiceTimeseries("memory"), 100),
	}
}

// fetchServiceInfoData returns service detail + tasks + networks + stats
func fetchServiceInfoData(idOrName string, info system.Info) map[string]any {
	svcID := resolveServiceID(idOrName)
	if svcID == "" { svcID = idOrName }
	svc, err := docker.Service(svcID)
	if err != nil { return nil }
	tasks, _ := docker.Tasks()
	nodes, _ := docker.Nodes()
	networks, _ := docker.Networks()
	var svcTasks []map[string]any
	for _, t := range tasks {
		if t.ServiceID == svc.ID {
			svcTasks = append(svcTasks, mapTask(t, nodes, []swarm.Service{svc}, info))
		}
	}
	if svcTasks == nil { svcTasks = []map[string]any{} }
	var svcNets []map[string]any
	for _, n := range networks {
		for _, vip := range svc.Endpoint.VirtualIPs {
			if vip.NetworkID == n.ID {
				svcNets = append(svcNets, mapNetwork(n))
			}
		}
	}
	if svcNets == nil { svcNets = []map[string]any{} }
	return map[string]any{
		"service":  mapService(svc, tasks, networks, info),
		"tasks":    svcTasks,
		"networks": svcNets,
		"stats":    clusterStats(),
	}
}

// fetchNodeInfoData returns node detail + tasks
func fetchNodeInfoData(idOrName string, info system.Info) map[string]any {
	nd, err := docker.Node(idOrName)
	if err != nil { return nil }
	cache := getNodeStatsCache()
	tasks, _ := docker.Tasks()
	services, _ := docker.Services()
	nodes, _ := docker.Nodes()
	var nodeTasks []map[string]any
	for _, t := range tasks {
		if t.NodeID == nd.ID {
			nodeTasks = append(nodeTasks, mapTask(t, nodes, services, info))
		}
	}
	if nodeTasks == nil { nodeTasks = []map[string]any{} }
	return map[string]any{
		"node":  mapNodeWithStats(nd, cache[nd.ID]),
		"tasks": nodeTasks,
	}
}

func fetchTaskInfoData(svcName string, info system.Info) map[string]any {
	svcID := resolveServiceID(svcName)
	if svcID == "" { svcID = svcName }
	tasks, _ := docker.Tasks()
	services, _ := docker.Services()
	nodes, _ := docker.Nodes()
	var svcTasks []map[string]any
	for _, t := range tasks {
		if t.ServiceID == svcID {
			svcTasks = append(svcTasks, mapTask(t, nodes, services, info))
		}
	}
	if svcTasks == nil { svcTasks = []map[string]any{} }
	return map[string]any{"tasks": svcTasks}
}

func fetchNetworkInfoData(idOrName string) map[string]any {
	networks, _ := docker.Networks()
	var net *types.NetworkResource
	for i, n := range networks {
		if n.ID == idOrName || n.Name == idOrName {
			net = &networks[i]
			break
		}
	}
	if net == nil { return nil }
	services, _ := docker.Services()
	tasks, _ := docker.Tasks()
	info, _ := docker.Info()
	netsAll, _ := docker.Networks()
	var linked []map[string]any
	for _, svc := range services {
		for _, vip := range svc.Endpoint.VirtualIPs {
			if vip.NetworkID == net.ID {
				linked = append(linked, mapService(svc, tasks, netsAll, info))
			}
		}
	}
	if linked == nil { linked = []map[string]any{} }
	return map[string]any{
		"network":  mapNetwork(*net),
		"services": linked,
	}
}

func fetchVolumeInfoData(name string) map[string]any {
	vols, _ := docker.Volumes()
	for _, v := range vols.Volumes {
		if v.Name == name {
			return mapVolume(v)
		}
	}
	return nil
}

func fetchSecretInfoData(idOrName string) map[string]any {
	sec, err := docker.SecretInspect(idOrName)
	if err != nil { return nil }
	return mapSecret(sec)
}

func fetchConfigInfoData(idOrName string) map[string]any {
	cfg, err := docker.ConfigInspect(idOrName)
	if err != nil { return nil }
	return mapConfig(cfg)
}

// Broadcast sends an event to all connected SSE clients (used by agent stats push)
func Broadcast(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Debug("sse broadcast marshal failed", "err", err)
		return
	}
	sseMu.RLock()
	defer sseMu.RUnlock()
	for client := range sseClients {
		if client.sub.Handler == "index" {
			select {
			case client.ch <- data:
			default:
			}
		}
	}
}

// EventPush handles agent stats push (POST /events)
// #101: validates SWARMPIT_AGENT_SECRET when configured
func EventPush(w http.ResponseWriter, r *http.Request) {
	if secret := os.Getenv("SWARMPIT_AGENT_SECRET"); secret != "" {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != secret {
			jsonErr(w, 401, "invalid agent secret")
			return
		}
	}
	var event map[string]any
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		jsonErr(w, 400, "invalid event data")
		return
	}
	storeAgentStats(event)
	w.WriteHeader(http.StatusAccepted)
}

// Agent stats cache
var agentStatsCache = struct {
	sync.RWMutex
	nodes map[string]map[string]any
	tasks map[string]map[string]any
}{nodes: make(map[string]map[string]any), tasks: make(map[string]map[string]any)}

func storeAgentStats(event map[string]any) {
	msg := event
	if m, ok := event["message"].(map[string]any); ok {
		msg = m
	}
	id, ok := msg["id"].(string)
	if !ok || id == "" {
		return
	}

	agentStatsCache.Lock()
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
	recordTimeseries(id, msg)
}

func getNodeStatsCache() map[string]map[string]any {
	agentStatsCache.RLock()
	defer agentStatsCache.RUnlock()
	r := make(map[string]map[string]any, len(agentStatsCache.nodes))
	for k, v := range agentStatsCache.nodes {
		r[k] = v
	}
	return r
}

func getTaskStats(taskName, taskID string) map[string]any {
	agentStatsCache.RLock()
	defer agentStatsCache.RUnlock()
	// Agent stores tasks by container ID with name like "/serviceName.slot.taskID"
	needle := "." + taskID
	for _, tm := range agentStatsCache.tasks {
		if name, ok := tm["name"].(string); ok && strings.HasSuffix(name, needle) {
			cpu := 0.0
			mem := 0.0
			if v, ok := tm["cpuPercentage"].(float64); ok { cpu = v / 100.0 }
			if v, ok := tm["memory"].(float64); ok { mem = v }
			return map[string]any{"cpu": cpu, "memory": mem}
		}
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
