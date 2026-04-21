package api

import (
	"sync"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/docker"
)

// Ring buffer for timeseries data — keeps 24h at ~5s intervals per node/task
const maxPoints = 17280 // 24h * 60min * 12 (every 5s)

type tsPoint struct {
	Ts     int64   `json:"time"`
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

var tsStore = struct {
	sync.RWMutex
	hosts    map[string][]tsPoint // nodeID -> points
	tasks    map[string][]tsPoint // taskName -> points
	services map[string][]tsPoint // serviceName -> points (aggregated)
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

// Called from storeAgentStats when agent sends node+task stats
func recordTimeseries(nodeID string, event map[string]any) {
	now := time.Now().Unix()
	tsStore.Lock()
	defer tsStore.Unlock()

	// Host stats
	cpuPct := 0.0
	memPct := 0.0
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
	tsStore.hosts[nodeID] = appendPoint(tsStore.hosts[nodeID], tsPoint{Ts: now, CPU: cpuPct, Memory: memPct})

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

			// Aggregate by service (extract service name from task name like "/serviceName.slot.id")
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
	// Task name format: "/serviceName.slot.taskID" or "/serviceName.nodeID.taskID"
	if len(taskName) > 0 && taskName[0] == '/' {
		taskName = taskName[1:]
	}
	// Find the last two dots and take everything before the second-to-last
	dots := 0
	for i := len(taskName) - 1; i >= 0; i-- {
		if taskName[i] == '.' {
			dots++
			if dots == 2 {
				return taskName[:i]
			}
		}
	}
	// If only one dot, take before it
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
	// Resolve node IDs to hostnames
	nodes, _ := docker.Nodes()
	hostnames := map[string]string{}
	for _, n := range nodes {
		hostnames[n.ID] = n.Description.Hostname
	}
	var result []map[string]any
	for host, points := range tsStore.hosts {
		if len(points) == 0 { continue }
		name := hostnames[host]
		if name == "" { name = host }
		times := make([]int64, len(points))
		cpus := make([]float64, len(points))
		mems := make([]float64, len(points))
		for i, p := range points {
			times[i] = p.Ts * 1000
			cpus[i] = p.CPU
			mems[i] = p.Memory
		}
		result = append(result, map[string]any{
			"name": name, "time": times, "cpu": cpus, "memory": mems,
		})
	}
	if result == nil { result = []map[string]any{} }
	return result
}

func getServiceTimeseries(sortBy string) []map[string]any {
	tsStore.RLock()
	defer tsStore.RUnlock()
	var result []map[string]any
	for svc, points := range tsStore.services {
		if len(points) == 0 { continue }
		times := make([]int64, len(points))
		cpus := make([]float64, len(points))
		mems := make([]float64, len(points))
		for i, p := range points {
			times[i] = p.Ts * 1000
			cpus[i] = p.CPU
			mems[i] = p.Memory
		}
		result = append(result, map[string]any{
			"name": svc, "time": times, "cpu": cpus, "memory": mems,
		})
	}
	if result == nil { result = []map[string]any{} }
	return result
}

func getTaskTimeseries(taskName string) []map[string]any {
	tsStore.RLock()
	defer tsStore.RUnlock()
	points, ok := tsStore.tasks[taskName]
	if !ok || len(points) == 0 { return []map[string]any{} }
	times := make([]int64, len(points))
	cpus := make([]float64, len(points))
	mems := make([]float64, len(points))
	for i, p := range points {
		times[i] = p.Ts * 1000
		cpus[i] = p.CPU
		mems[i] = p.Memory
	}
	return []map[string]any{{
		"name": taskName, "time": times, "cpu": cpus, "memory": mems,
	}}
}
