package api

import (
	"log/slog"
	"sync"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
)

// Ring buffer for timeseries data — keeps 24h at ~5s intervals per node/task
const maxPoints = 17280 // 24h * 60min * 12 (every 5s)

type tsPoint struct {
	Ts     int64   `json:"time"`
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
	Disk   float64 `json:"-"` // only persisted to SQLite
}

var tsStore = struct {
	sync.RWMutex
	hosts    map[string][]tsPoint // nodeID -> points
	tasks    map[string][]tsPoint // taskName -> points
	services map[string][]tsPoint // serviceName -> points (aggregated)
	// track last flush timestamp per node to avoid duplicates
	lastFlush int64
}{
	hosts:    make(map[string][]tsPoint),
	tasks:    make(map[string][]tsPoint),
	services: make(map[string][]tsPoint),
}

func appendPoint(buf []tsPoint, p tsPoint) []tsPoint {
	buf = append(buf, p)
	if len(buf) > maxPoints {
		buf = buf[len(buf)-maxPoints:]
	}
	return buf
}

// InitTimeseries loads persisted data from SQLite and starts the flush goroutine
func InitTimeseries() {
	rows, err := store.LoadTimeseries()
	if err != nil {
		slog.Warn("timeseries: failed to load from sqlite", "err", err)
	} else if len(rows) > 0 {
		tsStore.Lock()
		for _, r := range rows {
			tsStore.hosts[r.NodeID] = appendPoint(tsStore.hosts[r.NodeID], tsPoint{
				Ts: r.Ts, CPU: r.CPU, Memory: r.Memory, Disk: r.Disk,
			})
		}
		tsStore.lastFlush = rows[len(rows)-1].Ts
		tsStore.Unlock()
		slog.Info("timeseries: loaded from sqlite", "points", len(rows))
	}

	go flushLoop()
}

func flushLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		flushTimeseries()
	}
}

func flushTimeseries() {
	tsStore.RLock()
	cutoff := tsStore.lastFlush
	var rows []store.TsRow
	for nodeID, points := range tsStore.hosts {
		for _, p := range points {
			if p.Ts > cutoff {
				rows = append(rows, store.TsRow{NodeID: nodeID, Ts: p.Ts, CPU: p.CPU, Memory: p.Memory, Disk: p.Disk})
			}
		}
	}
	tsStore.RUnlock()

	if len(rows) == 0 {
		return
	}

	if err := store.SaveTimeseries(rows); err != nil {
		slog.Warn("timeseries: flush failed", "err", err)
		return
	}

	// Prune data older than 24h
	store.PruneTimeseries(time.Now().Add(-24 * time.Hour).Unix())

	tsStore.Lock()
	tsStore.lastFlush = time.Now().Unix()
	tsStore.Unlock()
	slog.Debug("timeseries: flushed to sqlite", "rows", len(rows))
}

// Called from storeAgentStats when agent sends node+task stats
func recordTimeseries(agentID string, event map[string]any) {
	now := time.Now().Unix()

	// Resolve agent container ID to node ID by checking running agent tasks
	nodeID := agentID
	tasks, _ := docker.Tasks()
	for _, t := range tasks {
		if t.Status.ContainerStatus != nil && t.Status.ContainerStatus.ContainerID == agentID {
			nodeID = t.NodeID
			break
		}
	}

	tsStore.Lock()
	defer tsStore.Unlock()

	// Host stats
	cpuPct, memPct, diskPct := 0.0, 0.0, 0.0
	if cpu, ok := event["cpu"].(map[string]any); ok {
		if v, ok := cpu["usedPercentage"].(float64); ok {
			cpuPct = v
		}
	}
	if mem, ok := event["memory"].(map[string]any); ok {
		if v, ok := mem["usedPercentage"].(float64); ok {
			memPct = v
		}
	}
	if disk, ok := event["disk"].(map[string]any); ok {
		if v, ok := disk["usedPercentage"].(float64); ok {
			diskPct = v
		}
	}
	tsStore.hosts[nodeID] = appendPoint(tsStore.hosts[nodeID], tsPoint{Ts: now, CPU: cpuPct, Memory: memPct, Disk: diskPct})

	// Task stats — aggregate per service
	svcAgg := map[string]tsPoint{} // serviceName -> aggregated
	if tasks, ok := event["tasks"].([]any); ok {
		for _, t := range tasks {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			name, _ := tm["name"].(string)
			if name == "" {
				continue
			}
			taskCPU := 0.0
			taskMem := 0.0
			if v, ok := tm["cpuPercentage"].(float64); ok {
				taskCPU = v / 100.0
			}
			if v, ok := tm["memory"].(float64); ok {
				taskMem = v
			}
			tsStore.tasks[name] = appendPoint(tsStore.tasks[name], tsPoint{Ts: now, CPU: taskCPU, Memory: taskMem})

			svcName := extractServiceName(name)
			if svcName != "" {
				agg := svcAgg[svcName]
				agg.Ts = now
				agg.CPU += taskCPU
				agg.Memory += taskMem
				svcAgg[svcName] = agg
			}
		}
	}
	for svc, pt := range svcAgg {
		tsStore.services[svc] = appendPoint(tsStore.services[svc], pt)
	}
}

func extractServiceName(taskName string) string {
	if len(taskName) > 0 && taskName[0] == '/' {
		taskName = taskName[1:]
	}
	dots := 0
	for i := len(taskName) - 1; i >= 0; i-- {
		if taskName[i] == '.' {
			dots++
			if dots == 2 {
				return taskName[:i]
			}
		}
	}
	for i := len(taskName) - 1; i >= 0; i-- {
		if taskName[i] == '.' {
			return taskName[:i]
		}
	}
	return taskName
}

// Timeseries readers

func getHostTimeseries() []map[string]any {
	tsStore.RLock()
	defer tsStore.RUnlock()
	nodes, _ := docker.Nodes()
	hostnames := map[string]string{}
	for _, n := range nodes {
		hostnames[n.ID] = n.Description.Hostname
	}
	var result []map[string]any
	for host, points := range tsStore.hosts {
		if len(points) == 0 {
			continue
		}
		name := hostnames[host]
		if name == "" {
			continue // skip unresolved container IDs
		}
		times := make([]string, len(points))
		cpus := make([]float64, len(points))
		mems := make([]float64, len(points))
		for i, p := range points {
			times[i] = time.Unix(p.Ts, 0).Format(time.RFC3339)
			cpus[i] = p.CPU
			mems[i] = p.Memory
		}
		result = append(result, map[string]any{
			"name": name, "time": times, "cpu": cpus, "memory": mems,
		})
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func getServiceTimeseries(sortBy string) []map[string]any {
	tsStore.RLock()
	defer tsStore.RUnlock()
	var result []map[string]any
	for svc, points := range tsStore.services {
		if len(points) == 0 {
			continue
		}
		times := make([]string, len(points))
		cpus := make([]float64, len(points))
		mems := make([]float64, len(points))
		for i, p := range points {
			times[i] = time.Unix(p.Ts, 0).Format(time.RFC3339)
			cpus[i] = p.CPU
			mems[i] = p.Memory / (1024 * 1024)
		}
		result = append(result, map[string]any{
			"service": svc, "task": nil, "time": times, "cpu": cpus, "memory": mems,
		})
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func getServiceTimeseriesByName(serviceName string) map[string]any {
	tsStore.RLock()
	defer tsStore.RUnlock()
	points, ok := tsStore.services[serviceName]
	if !ok || len(points) == 0 {
		return map[string]any{"service": serviceName, "task": nil, "time": []string{}, "cpu": []float64{}, "memory": []float64{}}
	}
	times := make([]string, len(points))
	cpus := make([]float64, len(points))
	mems := make([]float64, len(points))
	for i, p := range points {
		times[i] = time.Unix(p.Ts, 0).Format(time.RFC3339)
		cpus[i] = p.CPU
		mems[i] = p.Memory / (1024 * 1024)
	}
	return map[string]any{"service": serviceName, "task": nil, "time": times, "cpu": cpus, "memory": mems}
}

func getTaskTimeseries(taskName string) []map[string]any {
	tsStore.RLock()
	defer tsStore.RUnlock()
	points, ok := tsStore.tasks[taskName]
	if !ok || len(points) == 0 {
		return []map[string]any{}
	}
	times := make([]string, len(points))
	cpus := make([]float64, len(points))
	mems := make([]float64, len(points))
	for i, p := range points {
		times[i] = time.Unix(p.Ts, 0).Format(time.RFC3339)
		cpus[i] = p.CPU
		mems[i] = p.Memory / (1024 * 1024)
	}
	return []map[string]any{{
		"task": taskName, "service": extractServiceName(taskName),
		"time": times, "cpu": cpus, "memory": mems,
	}}
}
