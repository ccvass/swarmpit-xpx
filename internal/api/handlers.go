package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ccvass/swarmpit-xpx/internal/auth"
	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
	"github.com/docker/docker/api/types/swarm"
	"github.com/go-chi/chi/v5"
)

func json200(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func Version(w http.ResponseWriter, r *http.Request) {
	ver, _ := docker.Version()
	ping, _ := docker.Ping()
	apiVer := 0.0
	if v, err := strconv.ParseFloat(ping.APIVersion, 64); err == nil {
		apiVer = v
	}
	instanceName := os.Getenv("SWARMPIT_INSTANCE_NAME")
	var instVal any = nil
	if instanceName != "" {
		instVal = instanceName
	}
	json200(w, map[string]any{
		"name": "swarmpit-xpx", "version": "2.2.0", "revision": nil,
		"initialized": store.AdminExists(), "statistics": true, "instanceName": instVal,
		"docker": map[string]any{"api": apiVer, "engine": ver.Version},
	})
}

func HealthLive(w http.ResponseWriter, r *http.Request)  { json200(w, map[string]string{"status": "UP"}) }
func HealthReady(w http.ResponseWriter, r *http.Request) {
	_, err := docker.Ping()
	s := "UP"
	c := 200
	if err != nil {
		s = "DOWN"
		c = 503
	}
	w.WriteHeader(c)
	json200(w, map[string]any{"status": s, "components": map[string]string{"docker": s, "sqlite": "UP", "stats": "in-memory"}})
}

func Login(w http.ResponseWriter, r *http.Request) {
	u, p, ok := auth.DecodeBasic(r.Header.Get("Authorization"))
	if !ok {
		jsonErr(w, 400, "Missing credentials")
		return
	}
	user := store.AuthenticateUser(u, p)
	if user == nil {
		jsonErr(w, 401, "The username or password you entered is incorrect.")
		return
	}
	token, _ := auth.GenerateJWT(user.Username, user.Role)
	json200(w, map[string]string{"token": "Bearer " + token})
}

func Initialize(w http.ResponseWriter, r *http.Request) {
	if store.AdminExists() {
		jsonErr(w, 400, "Admin already exists")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if _, err := store.CreateUser(body.Username, body.Password, "admin", body.Email); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	json200(w, map[string]string{"status": "ok"})
}

// ── Docker resources with mappers ──

func NodeList(w http.ResponseWriter, r *http.Request) {
	nodes, err := docker.Nodes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapNodes(nodes))
}

func NodeDetail(w http.ResponseWriter, r *http.Request) {
	node, err := docker.Node(resolveNodeID(chi.URLParam(r, "id")))
	if err != nil { jsonErr(w, 404, err.Error()); return }
	cache := getNodeStatsCache()
	json200(w, mapNodeWithStats(node, cache[node.ID]))
}

func ServiceList(w http.ResponseWriter, r *http.Request) {
	svcs, err := docker.Services()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	json200(w, mapServices(svcs, tasks, nets, info))
}

func ServiceInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	svc, err := docker.Service(id)
	if err != nil {
		// Docker SDK accepts name too, but try listing if it fails
		svcs, _ := docker.Services()
		found := false
		for _, s := range svcs {
			if strings.HasPrefix(s.ID, id) || s.Spec.Name == id {
				svc = s
				found = true
				break
			}
		}
		if !found {
			jsonErr(w, 404, "Service not found")
			return
		}
	}
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	json200(w, mapService(svc, tasks, nets, info))
}

func TaskList(w http.ResponseWriter, r *http.Request) {
	tasks, err := docker.Tasks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	nodes, _ := docker.Nodes()
	svcs, _ := docker.Services()
	info, _ := docker.Info()
	json200(w, mapTasks(tasks, nodes, svcs, info))
}

func NetworkList(w http.ResponseWriter, r *http.Request) {
	nets, err := docker.Networks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapNetworks(nets))
}

func SecretList(w http.ResponseWriter, r *http.Request) {
	secrets, err := docker.Secrets()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapSecrets(secrets))
}

func ConfigList(w http.ResponseWriter, r *http.Request) {
	configs, err := docker.Configs()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapConfigs(configs))
}

func VolumeList(w http.ResponseWriter, r *http.Request) {
	vols, err := docker.Volumes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapVolumes(vols.Volumes))
}

func StackList(w http.ResponseWriter, r *http.Request) {
	svcs, err := docker.Services()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	stacks := map[string]bool{}
	for _, s := range svcs {
		ns := s.Spec.Labels["com.docker.stack.namespace"]
		if ns != "" {
			stacks[ns] = true
		}
	}
	result := []map[string]any{}
	for name := range stacks {
		result = append(result, mapStack(name, svcs, tasks, nets, info))
	}
	json200(w, result)
}

func StackInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	json200(w, mapStack(name, svcs, tasks, nets, info))
}

func StackServices(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	var result []map[string]any
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			result = append(result, mapService(s, tasks, nets, info))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackTasks(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tasks, _ := docker.Tasks()
	svcs, _ := docker.Services()
	nodes, _ := docker.Nodes()
	info, _ := docker.Info()
	svcIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			svcIDs[s.ID] = true
		}
	}
	var result []map[string]any
	for _, t := range tasks {
		if svcIDs[t.ServiceID] {
			result = append(result, mapTask(t, nodes, svcs, info))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackNetworks(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	nets, _ := docker.Networks()
	// Collect network IDs used by services in this stack
	netIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, n := range s.Spec.TaskTemplate.Networks {
				netIDs[n.Target] = true
			}
		}
	}
	var result []map[string]any
	for _, n := range nets {
		if netIDs[n.ID] {
			result = append(result, mapNetwork(n))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackVolumes(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	// Volumes from stack label + volumes used in mounts by stack services
	svcs, _ := docker.Services()
	vols, _ := docker.Volumes()
	volNames := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, m := range s.Spec.TaskTemplate.ContainerSpec.Mounts {
				if string(m.Type) == "volume" {
					volNames[m.Source] = true
				}
			}
		}
	}
	// Also include volumes with the stack label
	var result []map[string]any
	for _, v := range vols.Volumes {
		if v.Labels["com.docker.stack.namespace"] == name || volNames[v.Name] {
			result = append(result, mapVolume(v))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackConfigs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	configs, _ := docker.Configs()
	// Collect config IDs used by stack services
	cfgIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, c := range s.Spec.TaskTemplate.ContainerSpec.Configs {
				cfgIDs[c.ConfigID] = true
			}
		}
	}
	var result []map[string]any
	for _, c := range configs {
		if c.Spec.Labels["com.docker.stack.namespace"] == name || cfgIDs[c.ID] {
			result = append(result, mapConfig(c))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackSecrets(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	secrets, _ := docker.Secrets()
	// Collect secret IDs used by stack services
	secIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, sec := range s.Spec.TaskTemplate.ContainerSpec.Secrets {
				secIDs[sec.SecretID] = true
			}
		}
	}
	var result []map[string]any
	for _, s := range secrets {
		if s.Spec.Labels["com.docker.stack.namespace"] == name || secIDs[s.ID] {
			result = append(result, mapSecret(s))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackFile(w http.ResponseWriter, r *http.Request) {
	json200(w, map[string]any{"name": chi.URLParam(r, "name"), "spec": ""})
}

func StackCompose(w http.ResponseWriter, r *http.Request) {
	json200(w, map[string]string{"compose": ""})
}


// Stats returns cluster resource stats (CPU, memory, disk from node resources)
func Stats(w http.ResponseWriter, r *http.Request) {
	nodes, err := docker.Nodes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	cache := getNodeStatsCache()
	totalCPU := 0.0
	totalMem := int64(0)
	resources := map[string]map[string]any{}
	cpuSum, memSum, diskSum := 0.0, 0.0, 0.0
	memUsed, diskUsed, diskTotal := int64(0), int64(0), int64(0)
	n := 0
	for _, nd := range nodes {
		if nd.Status.State != "ready" { continue }
		cpu := float64(nd.Description.Resources.NanoCPUs) / 1e9
		mem := nd.Description.Resources.MemoryBytes
		totalCPU += cpu
		totalMem += mem
		resources[nd.ID] = map[string]any{"cores": cpu, "memory": mem}
		if s, ok := cache[nd.ID]; ok {
			n++
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
	if n > 0 { cpuAvg = cpuSum / float64(n); memAvg = memSum / float64(n); diskAvg = diskSum / float64(n) }
	json200(w, map[string]any{
		"resources": resources,
		"cpu":    map[string]any{"usage": cpuAvg, "cores": totalCPU},
		"memory": map[string]any{"usage": memAvg, "used": memUsed, "total": totalMem},
		"disk":   map[string]any{"usage": diskAvg, "used": diskUsed, "total": diskTotal},
	})
}


// NodeTimeseries returns empty timeseries (stats not implemented in Go backend yet)
func NodeTimeseries(w http.ResponseWriter, r *http.Request) {
	json200(w, getHostTimeseries())
}

// ServiceTaskList returns tasks for a specific service
func ServiceTaskList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Resolve name to ID if needed
	svcs, _ := docker.Services()
	resolvedID := id
	for _, s := range svcs {
		if s.Spec.Name == id || strings.HasPrefix(s.ID, id) {
			resolvedID = s.ID
			break
		}
	}
	tasks, err := docker.Tasks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	nodes, _ := docker.Nodes()
	info, _ := docker.Info()
	var result []map[string]any
	for _, t := range tasks {
		if t.ServiceID == resolvedID {
			result = append(result, mapTask(t, nodes, svcs, info))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func TaskInfo(w http.ResponseWriter, r *http.Request) {
	tasks, err := docker.Tasks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	nodes, _ := docker.Nodes()
	svcs, _ := docker.Services()
	info, _ := docker.Info()
	id := chi.URLParam(r, "id")
	for _, t := range tasks {
		if t.ID == id {
			json200(w, mapTask(t, nodes, svcs, info))
			return
		}
	}
	jsonErr(w, 404, "Task not found")
}

func NetworkInfo(w http.ResponseWriter, r *http.Request) {
	nets, err := docker.Networks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, n := range nets {
		if n.ID == id || n.Name == id {
			json200(w, mapNetwork(n))
			return
		}
	}
	jsonErr(w, 404, "Network not found")
}

func VolumeInfo(w http.ResponseWriter, r *http.Request) {
	vols, err := docker.Volumes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, v := range vols.Volumes {
		if v.Name == id {
			json200(w, mapVolume(v))
			return
		}
	}
	jsonErr(w, 404, "Volume not found")
}

func SecretInfo(w http.ResponseWriter, r *http.Request) {
	secrets, err := docker.Secrets()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, s := range secrets {
		if s.ID == id || s.Spec.Name == id {
			json200(w, mapSecret(s))
			return
		}
	}
	jsonErr(w, 404, "Secret not found")
}

func ConfigInfo(w http.ResponseWriter, r *http.Request) {
	configs, err := docker.Configs()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, c := range configs {
		if c.ID == id || c.Spec.Name == id {
			json200(w, mapConfig(c))
			return
		}
	}
	jsonErr(w, 404, "Config not found")
}

func resolveServiceID(id string) string {
	svcs, _ := docker.Services()
	for _, s := range svcs {
		if s.Spec.Name == id || strings.HasPrefix(s.ID, id) { return s.ID }
	}
	return id
}


func resolveNodeID(id string) string {
	nodes, _ := docker.Nodes()
	for _, n := range nodes {
		if n.ID == id || n.Description.Hostname == id {
			return n.ID
		}
	}
	return id
}

func ServiceLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := docker.ServiceLogs(resolveServiceID(chi.URLParam(r, "id")), r.URL.Query().Get("tail"))
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, map[string]string{"logs": logs})
}

func ServiceDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteService(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func SecretDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteSecret(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func ConfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteConfig(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func NetworkDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteNetwork(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func WebhookTrigger(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	svcID, ok := store.FindWebhook(token)
	if !ok { jsonErr(w, 404, "Webhook not found"); return }
	svc, err := docker.Service(svcID)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	svc.Spec.TaskTemplate.ForceUpdate++
	if err := docker.UpdateService(svcID, svc.Version, svc.Spec); err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, map[string]string{"status": "triggered"})
}

func AuditList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit == 0 { limit = 50 }
	json200(w, store.AuditEntries(limit, offset))
}

func GitDeploy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RepoURL     string `json:"repo-url"`
		Branch      string `json:"branch"`
		ComposePath string `json:"compose-path"`
		StackName   string `json:"stack-name"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.RepoURL == "" || body.StackName == "" {
		jsonErr(w, 400, "repo-url and stack-name required")
		return
	}
	// TODO: implement git clone + stack deploy
	json200(w, map[string]string{"status": "not implemented yet"})
}

// Node tasks
func NodeTasks(w http.ResponseWriter, r *http.Request) {
	id := resolveNodeID(chi.URLParam(r, "id"))
	tasks, _ := docker.Tasks()
	nodes, _ := docker.Nodes()
	svcs, _ := docker.Services()
	info, _ := docker.Info()
	var result []map[string]any
	for _, t := range tasks {
		if t.NodeID == id { result = append(result, mapTask(t, nodes, svcs, info)) }
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

// Resource-linked services
func NetworkServices(w http.ResponseWriter, r *http.Request) { linkedServices(w, r, "network") }
func VolumeServices(w http.ResponseWriter, r *http.Request)  { linkedServices(w, r, "volume") }
func SecretServices(w http.ResponseWriter, r *http.Request)  { linkedServices(w, r, "secret") }
func ConfigServices(w http.ResponseWriter, r *http.Request)  { linkedServices(w, r, "config") }

func linkedServices(w http.ResponseWriter, r *http.Request, resType string) {
	id := chi.URLParam(r, "id")
	svcs, _ := docker.Services()
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	var result []map[string]any
	for _, s := range svcs {
		spec := s.Spec
		match := false
		switch resType {
		case "network":
			for _, n := range spec.TaskTemplate.Networks {
				if n.Target == id { match = true; break }
			}
			// Also check by name
			if !match {
				for _, n := range nets {
					if n.Name == id {
						for _, sn := range spec.TaskTemplate.Networks {
							if sn.Target == n.ID { match = true; break }
						}
					}
				}
			}
		case "volume":
			for _, m := range spec.TaskTemplate.ContainerSpec.Mounts {
				if string(m.Type) == "volume" && m.Source == id { match = true; break }
			}
		case "secret":
			for _, sec := range spec.TaskTemplate.ContainerSpec.Secrets {
				if sec.SecretID == id || sec.SecretName == id { match = true; break }
			}
		case "config":
			for _, cfg := range spec.TaskTemplate.ContainerSpec.Configs {
				if cfg.ConfigID == id || cfg.ConfigName == id { match = true; break }
			}
		}
		if match { result = append(result, mapService(s, tasks, nets, info)) }
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

// Service sub-resources
func ServiceNetworks(w http.ResponseWriter, r *http.Request) {
	svc, err := docker.Service(chi.URLParam(r, "id"))
	if err != nil { json200(w, []any{}); return }
	nets, _ := docker.Networks()
	netMap := map[string]any{}
	for _, n := range nets { netMap[n.ID] = mapNetwork(n) }
	var result []any
	for _, n := range svc.Spec.TaskTemplate.Networks {
		if v, ok := netMap[n.Target]; ok { result = append(result, v) }
	}
	if result == nil { result = []any{} }
	json200(w, result)
}

func ServiceCompose(w http.ResponseWriter, r *http.Request) {
	json200(w, map[string]string{"compose": ""})
}

// Timeseries (empty — no historical data in Go backend yet)
func ServicesTsCPU(w http.ResponseWriter, r *http.Request) { json200(w, getServiceTimeseries("cpu")) }
func ServicesTsMemory(w http.ResponseWriter, r *http.Request) { json200(w, getServiceTimeseries("memory")) }
func TaskTimeseries(w http.ResponseWriter, r *http.Request) { json200(w, getTaskTimeseries(chi.URLParam(r, "id"))) }

// Plugins and placement
func LabelsService(w http.ResponseWriter, r *http.Request) {
	svcs, _ := docker.Services()
	labels := map[string]bool{}
	for _, s := range svcs {
		for k := range s.Spec.Labels { labels[k] = true }
	}
	result := []string{}
	for k := range labels { result = append(result, k) }
	json200(w, result)
}

func PluginNetwork(w http.ResponseWriter, r *http.Request) {
	json200(w, []string{"bridge", "host", "overlay", "macvlan"})
}

func PluginVolume(w http.ResponseWriter, r *http.Request) {
	json200(w, []string{"local"})
}

func PluginLog(w http.ResponseWriter, r *http.Request) {
	json200(w, []string{"json-file", "syslog", "journald", "gelf", "fluentd", "awslogs", "splunk", "none"})
}

func Placement(w http.ResponseWriter, r *http.Request) {
	nodes, _ := docker.Nodes()
	var result []string
	for _, n := range nodes {
		result = append(result, "node.hostname == "+n.Description.Hostname)
		for k, v := range n.Spec.Labels {
			result = append(result, "node.labels."+k+" == "+v)
		}
	}
	json200(w, result)
}

// ── CRUD handlers (#36) ──

func ServiceCreate(w http.ResponseWriter, r *http.Request) {
	var spec swarm.ServiceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		jsonErr(w, 400, "invalid service spec: "+err.Error())
		return
	}
	resp, err := docker.CreateService(spec)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func ServiceUpdate(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil {
		jsonErr(w, 404, "service not found")
		return
	}
	var spec swarm.ServiceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		jsonErr(w, 400, "invalid service spec: "+err.Error())
		return
	}
	if err := docker.UpdateService(id, svc.Version, spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "updated"})
}

func StackCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Spec string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Spec == "" {
		jsonErr(w, 400, "name and spec (YAML) required")
		return
	}
	out, err := deployStack(body.Name, body.Spec)
	if err != nil {
		jsonErr(w, 500, out)
		return
	}
	json200(w, map[string]string{"status": "deployed", "output": out})
}

func StackUpdate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		Spec string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Spec == "" {
		jsonErr(w, 400, "spec (YAML) required")
		return
	}
	out, err := deployStack(name, body.Spec)
	if err != nil {
		jsonErr(w, 500, out)
		return
	}
	json200(w, map[string]string{"status": "updated", "output": out})
}

func deployStack(name, spec string) (string, error) {
	cmd := exec.Command("docker", "stack", "deploy", "-c", "-", name)
	cmd.Stdin = strings.NewReader(spec)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func StackDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	out, err := exec.Command("docker", "stack", "rm", name).CombinedOutput()
	if err != nil {
		jsonErr(w, 500, string(out))
		return
	}
	json200(w, map[string]string{"status": "removed", "output": string(out)})
}

func NetworkCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NetworkName string `json:"networkName"`
		Driver      string `json:"driver"`
		Internal    bool   `json:"internal"`
		Attachable  bool   `json:"attachable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.NetworkName == "" {
		jsonErr(w, 400, "networkName required")
		return
	}
	if body.Driver == "" {
		body.Driver = "overlay"
	}
	resp, err := docker.CreateNetwork(body.NetworkName, body.Driver, body.Internal, body.Attachable)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func VolumeCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VolumeName string `json:"volumeName"`
		Driver     string `json:"driver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VolumeName == "" {
		jsonErr(w, 400, "volumeName required")
		return
	}
	if body.Driver == "" {
		body.Driver = "local"
	}
	v, err := docker.CreateVolume(body.VolumeName, body.Driver)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"name": v.Name})
}

func SecretCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SecretName string `json:"secretName"`
		Data       string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SecretName == "" {
		jsonErr(w, 400, "secretName required")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		jsonErr(w, 400, "data must be base64 encoded")
		return
	}
	resp, err := docker.CreateSecret(swarm.SecretSpec{
		Annotations: swarm.Annotations{Name: body.SecretName},
		Data:        data,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func ConfigCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConfigName string `json:"configName"`
		Data       string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ConfigName == "" {
		jsonErr(w, 400, "configName required")
		return
	}
	resp, err := docker.CreateConfig(swarm.ConfigSpec{
		Annotations: swarm.Annotations{Name: body.ConfigName},
		Data:        []byte(body.Data),
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func SecretUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sec, err := docker.SecretInspect(id)
	if err != nil {
		jsonErr(w, 404, "secret not found")
		return
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		jsonErr(w, 400, "data must be base64 encoded")
		return
	}
	sec.Spec.Data = data
	if err := docker.UpdateSecret(id, sec.Version, sec.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "updated"})
}

func ConfigUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, err := docker.ConfigInspect(id)
	if err != nil {
		jsonErr(w, 404, "config not found")
		return
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	cfg.Spec.Data = []byte(body.Data)
	if err := docker.UpdateConfig(id, cfg.Version, cfg.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "updated"})
}

// ── Redeploy & Rollback (#37) ──

func ServiceRedeploy(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil {
		jsonErr(w, 404, "service not found")
		return
	}
	svc.Spec.TaskTemplate.ForceUpdate++
	if err := docker.UpdateService(id, svc.Version, svc.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "redeployed"})
}

func ServiceRollback(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil {
		jsonErr(w, 404, "service not found")
		return
	}
	if svc.PreviousSpec == nil {
		jsonErr(w, 400, "no previous spec available for rollback")
		return
	}
	if err := docker.UpdateService(id, svc.Version, *svc.PreviousSpec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "rolled back"})
}
