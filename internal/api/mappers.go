package api

import (
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
)

// ── Node mapper ──

func mapNode(n swarm.Node) map[string]any {
	res := n.Description.Resources
	cpuCores := float64(0)
	memBytes := int64(0)
	if res.NanoCPUs > 0 {
		cpuCores = float64(res.NanoCPUs) / 1e9
	}
	if res.MemoryBytes > 0 {
		memBytes = res.MemoryBytes
	}
	role := "worker"
	if n.Spec.Role == swarm.NodeRoleManager {
		role = "manager"
	}
	var leader interface{}
	if n.ManagerStatus != nil && n.ManagerStatus.Leader {
		leader = true
	}
	labels := []map[string]string{}
	for k, v := range n.Spec.Labels {
		labels = append(labels, map[string]string{"name": k, "value": v})
	}
	return map[string]any{
		"id":           n.ID,
		"version":      n.Version.Index,
		"nodeName":     n.Description.Hostname,
		"role":         role,
		"state":        string(n.Status.State),
		"availability": string(n.Spec.Availability),
		"address":      n.Status.Addr,
		"engine":       n.Description.Engine.EngineVersion,
		"os":           n.Description.Platform.OS,
		"arch":         n.Description.Platform.Architecture,
		"leader":       leader,
		"labels":       labels,
		"resources": map[string]any{
			"cpu":    cpuCores,
			"memory": memBytes,
		},
	}
}

func mapNodes(nodes []swarm.Node) []map[string]any {
	result := make([]map[string]any, len(nodes))
	for i, n := range nodes {
		result[i] = mapNode(n)
	}
	return result
}

// ── Service mapper ──

func mapService(s swarm.Service, tasks []swarm.Task) map[string]any {
	spec := s.Spec
	cs := spec.TaskTemplate.ContainerSpec

	// Image parsing
	image := cs.Image
	name, tag, digest := parseImage(image)

	// Mode
	mode := "replicated"
	replicas := 1
	if spec.Mode.Global != nil {
		mode = "global"
		replicas = 0
	} else if spec.Mode.Replicated != nil && spec.Mode.Replicated.Replicas != nil {
		replicas = int(*spec.Mode.Replicated.Replicas)
	}

	// Stack
	stack := spec.Labels["com.docker.stack.namespace"]

	// Status from tasks
	running, total := 0, 0
	for _, t := range tasks {
		if t.ServiceID == s.ID {
			total++
			if t.Status.State == swarm.TaskStateRunning {
				running++
			}
		}
	}
	state := "running"
	if running == 0 && total > 0 {
		state = "not running"
	}

	// Ports
	ports := []map[string]any{}
	if s.Endpoint.Ports != nil {
		for _, p := range s.Endpoint.Ports {
			ports = append(ports, map[string]any{
				"containerPort": p.TargetPort,
				"hostPort":      p.PublishedPort,
				"protocol":      string(p.Protocol),
				"mode":          string(p.PublishMode),
			})
		}
	}

	// Mounts
	mounts := []map[string]any{}
	for _, m := range cs.Mounts {
		mounts = append(mounts, map[string]any{
			"containerPath": m.Target,
			"host":          m.Source,
			"type":          string(m.Type),
			"readOnly":      m.ReadOnly,
		})
	}

	// Networks
	networks := []map[string]any{}
	for _, n := range spec.TaskTemplate.Networks {
		networks = append(networks, map[string]any{"networkName": n.Target})
	}

	// Variables
	variables := []map[string]string{}
	for _, e := range cs.Env {
		parts := strings.SplitN(e, "=", 2)
		v := ""
		if len(parts) > 1 {
			v = parts[1]
		}
		variables = append(variables, map[string]string{"name": parts[0], "value": v})
	}

	// Labels
	labels := []map[string]string{}
	for k, v := range spec.Labels {
		labels = append(labels, map[string]string{"name": k, "value": v})
	}
	containerLabels := []map[string]string{}
	for k, v := range cs.Labels {
		containerLabels = append(containerLabels, map[string]string{"name": k, "value": v})
	}

	// Secrets
	secrets := []map[string]any{}
	for _, sec := range cs.Secrets {
		secrets = append(secrets, map[string]any{"secretName": sec.SecretName, "secretTarget": sec.File.Name})
	}

	// Configs
	configs := []map[string]any{}
	for _, cfg := range cs.Configs {
		configs = append(configs, map[string]any{"configName": cfg.ConfigName, "configTarget": cfg.File.Name})
	}

	// Resources
	resources := map[string]map[string]any{"reservation": {"cpu": 0, "memory": 0}, "limit": {"cpu": 0, "memory": 0}}
	if spec.TaskTemplate.Resources != nil {
		if r := spec.TaskTemplate.Resources.Reservations; r != nil {
			resources["reservation"] = map[string]any{"cpu": float64(r.NanoCPUs) / 1e9, "memory": r.MemoryBytes / 1e6}
		}
		if l := spec.TaskTemplate.Resources.Limits; l != nil {
			resources["limit"] = map[string]any{"cpu": float64(l.NanoCPUs) / 1e9, "memory": l.MemoryBytes / 1e6}
		}
	}

	// Deployment
	deployment := map[string]any{}
	if spec.UpdateConfig != nil {
		deployment["update"] = map[string]any{
			"parallelism":   spec.UpdateConfig.Parallelism,
			"delay":         int64(spec.UpdateConfig.Delay.Seconds()),
			"order":         string(spec.UpdateConfig.Order),
			"failureAction": string(spec.UpdateConfig.FailureAction),
		}
	}
	if spec.TaskTemplate.RestartPolicy != nil {
		deployment["restartPolicy"] = map[string]any{
			"condition": string(spec.TaskTemplate.RestartPolicy.Condition),
			"delay":     int64(spec.TaskTemplate.RestartPolicy.Delay.Seconds()),
		}
	}

	updateStatus := ""
	updateMessage := ""
	if s.UpdateStatus != nil {
		updateStatus = string(s.UpdateStatus.State)
		updateMessage = s.UpdateStatus.Message
	}

	return map[string]any{
		"id":              s.ID,
		"version":         s.Version.Index,
		"createdAt":       s.CreatedAt,
		"updatedAt":       s.UpdatedAt,
		"serviceName":     spec.Name,
		"mode":            mode,
		"stack":           stack,
		"replicas":        replicas,
		"state":           state,
		"repository":      map[string]string{"name": name, "tag": tag, "image": name + ":" + tag, "imageDigest": digest},
		"status":          map[string]any{"tasks": map[string]int{"running": running, "total": total}, "update": updateStatus, "message": updateMessage},
		"ports":           ports,
		"mounts":          mounts,
		"networks":        networks,
		"variables":       variables,
		"labels":          labels,
		"containerLabels": containerLabels,
		"secrets":         secrets,
		"configs":         configs,
		"resources":       resources,
		"deployment":      deployment,
		"command":         cs.Args,
		"entrypoint":      cs.Command,
		"hostname":        cs.Hostname,
		"user":            cs.User,
		"dir":             cs.Dir,
		"tty":             cs.TTY,
	}
}

func mapServices(services []swarm.Service, tasks []swarm.Task) []map[string]any {
	result := make([]map[string]any, len(services))
	for i, s := range services {
		result[i] = mapService(s, tasks)
	}
	return result
}

// ── Network mapper ──

func mapNetwork(n types.NetworkResource) map[string]any {
	return map[string]any{
		"id":          n.ID,
		"networkName": n.Name,
		"driver":      n.Driver,
		"scope":       n.Scope,
		"internal":    n.Internal,
		"created":     n.Created,
	}
}

func mapNetworks(nets []types.NetworkResource) []map[string]any {
	result := make([]map[string]any, len(nets))
	for i, n := range nets {
		result[i] = mapNetwork(n)
	}
	return result
}

// ── Task mapper ──

func mapTask(t swarm.Task) map[string]any {
	image := ""
	if t.Spec.ContainerSpec != nil {
		image = t.Spec.ContainerSpec.Image
	}
	name, tag, _ := parseImage(image)
	return map[string]any{
		"id":          t.ID,
		"taskName":    t.ServiceID,
		"nodeId":      t.NodeID,
		"serviceId":   t.ServiceID,
		"version":     t.Version.Index,
		"createdAt":   t.CreatedAt,
		"updatedAt":   t.UpdatedAt,
		"repository":  map[string]string{"name": name, "tag": tag, "image": name + ":" + tag},
		"state":       string(t.Status.State),
		"status":      map[string]string{"state": string(t.Status.State), "message": t.Status.Message, "error": t.Status.Err},
		"desiredState": string(t.DesiredState),
	}
}

func mapTasks(tasks []swarm.Task) []map[string]any {
	result := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		result[i] = mapTask(t)
	}
	return result
}

// ── Helpers ──

func parseImage(image string) (name, tag, digest string) {
	// Split digest
	if idx := strings.Index(image, "@"); idx >= 0 {
		digest = image[idx+1:]
		image = image[:idx]
	}
	// Split tag
	if idx := strings.LastIndex(image, ":"); idx >= 0 && !strings.Contains(image[idx:], "/") {
		tag = image[idx+1:]
		name = image[:idx]
	} else {
		name = image
		tag = "latest"
	}
	return
}
