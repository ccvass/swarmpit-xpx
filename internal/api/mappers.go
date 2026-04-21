package api

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
)

// ── helpers ──

func parseImage(image string) (name, tag, digest string) {
	if idx := strings.Index(image, "@"); idx >= 0 {
		digest = image[idx+1:]
		image = image[:idx]
	}
	if idx := strings.LastIndex(image, ":"); idx >= 0 && !strings.Contains(image[idx:], "/") {
		tag = image[idx+1:]
		name = image[:idx]
	} else {
		name = image
		tag = ""
	}
	return
}

func mapToNameValue(m map[string]string) []map[string]string {
	r := []map[string]string{}
	for k, v := range m {
		r = append(r, map[string]string{"name": k, "value": v})
	}
	return r
}

func nanoToSec(n int64) int64 { return n / 1_000_000_000 }

func nilStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilLabels(m map[string]string) any {
	if len(m) == 0 {
		return nil
	}
	return m
}

func resources(r *swarm.Resources) map[string]any {
	if r == nil {
		return map[string]any{"cpu": 0.0, "memory": 0}
	}
	return map[string]any{
		"cpu":    float64(r.NanoCPUs) / 1e9,
		"memory": int64(r.MemoryBytes) / (1024 * 1024),
	}
}

func limitResources(l *swarm.Limit) map[string]any {
	if l == nil {
		return map[string]any{"cpu": 0.0, "memory": 0}
	}
	return map[string]any{
		"cpu":    float64(l.NanoCPUs) / 1e9,
		"memory": int64(l.MemoryBytes) / (1024 * 1024),
	}
}

func serviceResources(tt *swarm.TaskSpec) map[string]any {
	res := map[string]any{
		"reservation": map[string]any{"cpu": 0.0, "memory": 0},
		"limit":       map[string]any{"cpu": 0.0, "memory": 0},
	}
	if tt.Resources != nil {
		if tt.Resources.Reservations != nil {
			res["reservation"] = resources(tt.Resources.Reservations)
		}
		if tt.Resources.Limits != nil {
			res["limit"] = limitResources(tt.Resources.Limits)
		}
	}
	return res
}

// ── Node ──

func mapNode(n swarm.Node) map[string]any {
	role := string(n.Spec.Role)
	var addr string
	if role == "manager" && n.ManagerStatus != nil {
		parts := strings.SplitN(n.ManagerStatus.Addr, ":", 2)
		addr = parts[0]
	} else {
		addr = n.Status.Addr
	}
	var leader any
	if n.ManagerStatus != nil && n.ManagerStatus.Leader {
		leader = true
	}
	labels := []map[string]string{}
	for k, v := range n.Spec.Labels {
		labels = append(labels, map[string]string{"name": k, "value": v})
	}
	nets := []string{}
	vols := []string{}
	for _, p := range n.Description.Engine.Plugins {
		switch p.Type {
		case "Network":
			nets = append(nets, p.Name)
		case "Volume":
			vols = append(vols, p.Name)
		}
	}
	return map[string]any{
		"id": n.ID, "version": n.Version.Index,
		"nodeName": n.Description.Hostname, "role": role,
		"availability": string(n.Spec.Availability),
		"labels": labels, "state": string(n.Status.State),
		"address": addr,
		"engine":  n.Description.Engine.EngineVersion,
		"arch":    n.Description.Platform.Architecture,
		"os":      n.Description.Platform.OS,
		"resources": resources(&n.Description.Resources),
		"plugins": map[string]any{"networks": nets, "volumes": vols},
		"leader":  leader,
	}
}

func mapNodes(nodes []swarm.Node) []map[string]any {
	r := make([]map[string]any, len(nodes))
	for i, n := range nodes {
		r[i] = mapNode(n)
	}
	return r
}

// ── Network ──

func mapNetwork(n types.NetworkResource) map[string]any {
	var ipam map[string]any
	if len(n.IPAM.Config) > 0 {
		c := n.IPAM.Config[0]
		ipam = map[string]any{"subnet": c.Subnet, "gateway": c.Gateway}
	}
	var stackVal any
	if s := n.Labels["com.docker.stack.namespace"]; s != "" {
		stackVal = s
	}
	return map[string]any{
		"id": n.ID, "networkName": n.Name, "created": n.Created,
		"scope": n.Scope, "driver": n.Driver, "internal": n.Internal,
		"options": mapToNameValue(n.Options), "attachable": n.Attachable,
		"ingress": n.Ingress, "enableIPv6": n.EnableIPv6,
		"labels": nilLabels(n.Labels), "stack": stackVal, "ipam": ipam,
	}
}

func mapNetworks(nets []types.NetworkResource) []map[string]any {
	r := []map[string]any{}
	for _, n := range nets {
		if n.Driver != "null" {
			r = append(r, mapNetwork(n))
		}
	}
	return r
}

// ── Secret ──

func mapSecret(s swarm.Secret) map[string]any {
	return map[string]any{
		"id": s.ID, "version": s.Version.Index,
		"secretName": s.Spec.Name,
		"createdAt":  s.CreatedAt, "updatedAt": s.UpdatedAt,
	}
}

func mapSecrets(secrets []swarm.Secret) []map[string]any {
	r := make([]map[string]any, len(secrets))
	for i, s := range secrets {
		r[i] = mapSecret(s)
	}
	return r
}

// ── Config ──

func mapConfig(c swarm.Config) map[string]any {
	return map[string]any{
		"id": c.ID, "version": c.Version.Index,
		"configName": c.Spec.Name,
		"createdAt":  c.CreatedAt, "updatedAt": c.UpdatedAt,
		"data":       base64.StdEncoding.EncodeToString(c.Spec.Data),
	}
}

func mapConfigs(configs []swarm.Config) []map[string]any {
	r := make([]map[string]any, len(configs))
	for i, c := range configs {
		r[i] = mapConfig(c)
	}
	return r
}

// ── Volume ──

func mapVolume(v *volume.Volume) map[string]any {
	var stackVal any
	if s := v.Labels["com.docker.stack.namespace"]; s != "" {
		stackVal = s
	}
	return map[string]any{
		"id": v.Name, "volumeName": v.Name, "driver": v.Driver,
		"stack": stackVal, "labels": nilLabels(v.Labels),
		"options": mapToNameValue(v.Options),
		"mountpoint": v.Mountpoint, "scope": v.Scope,
	}
}

func mapVolumes(vols []*volume.Volume) []map[string]any {
	r := make([]map[string]any, len(vols))
	for i, v := range vols {
		r[i] = mapVolume(v)
	}
	return r
}

// ── Task ──

func taskName(taskID, nodeID string, slot int, svcName, mode string) string {
	switch mode {
	case "replicated":
		return fmt.Sprintf("%s.%d", svcName, slot)
	case "global":
		return fmt.Sprintf("%s.%s", svcName, nodeID)
	default:
		return taskID
	}
}

func serviceMode(spec *swarm.ServiceSpec) string {
	if spec == nil {
		return ""
	}
	m := spec.Mode
	switch {
	case m.Replicated != nil:
		return "replicated"
	case m.Global != nil:
		return "global"
	case m.ReplicatedJob != nil:
		return "replicatedjob"
	case m.GlobalJob != nil:
		return "globaljob"
	default:
		return ""
	}
}

func mapTask(t swarm.Task, nodes []swarm.Node, services []swarm.Service, info system.Info) map[string]any {
	image := ""
	if t.Spec.ContainerSpec != nil {
		image = t.Spec.ContainerSpec.Image
	}
	parts := strings.SplitN(image, "@", 2)
	imageName := parts[0]
	var imageDigest any
	if len(parts) > 1 {
		imageDigest = parts[1]
	}

	nodeName := t.NodeID
	for _, n := range nodes {
		if n.ID == t.NodeID {
			nodeName = n.Description.Hostname
			break
		}
	}

	svcName := t.ServiceID
	var svc *swarm.Service
	for i := range services {
		if services[i].ID == t.ServiceID {
			svc = &services[i]
			svcName = svc.Spec.Name
			break
		}
	}

	mode := ""
	if svc != nil {
		mode = serviceMode(&svc.Spec)
	}
	tName := taskName(t.ID, t.NodeID, t.Slot, svcName, mode)

	// logdriver: task's own or fallback to system default
	logdriver := info.LoggingDriver
	if t.Spec.LogDriver != nil && t.Spec.LogDriver.Name != "" {
		logdriver = t.Spec.LogDriver.Name
	}

	// resources come from the SERVICE's TaskTemplate, not the task
	var res map[string]any
	if svc != nil {
		res = serviceResources(&svc.Spec.TaskTemplate)
	} else {
		res = serviceResources(&swarm.TaskSpec{})
	}

	var errVal any
	if t.Status.Err != "" {
		errVal = t.Status.Err
	}

	return map[string]any{
		"id": t.ID, "taskName": tName,
		"version":   t.Version.Index,
		"createdAt": t.CreatedAt, "updatedAt": t.UpdatedAt,
		"repository": map[string]any{"image": imageName, "imageDigest": imageDigest},
		"state":        string(t.Status.State),
		"status":       map[string]any{"error": errVal},
		"desiredState": string(t.DesiredState),
		"logdriver":    logdriver,
		"serviceName":  svcName,
		"resources":    res,
		"nodeId":       t.NodeID,
		"nodeName":     nodeName,
	}
}

func mapTasks(tasks []swarm.Task, nodes []swarm.Node, services []swarm.Service, info system.Info) []map[string]any {
	r := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		r[i] = mapTask(t, nodes, services, info)
	}
	return r
}

// ── Service ──

func mapService(s swarm.Service, tasks []swarm.Task, networks []types.NetworkResource, info system.Info) map[string]any {
	spec := s.Spec
	tt := spec.TaskTemplate
	cs := tt.ContainerSpec
	svcLabels := spec.Labels
	if svcLabels == nil {
		svcLabels = map[string]string{}
	}

	image := cs.Image
	parts := strings.SplitN(image, "@", 2)
	imageName := parts[0]
	var imageDigest any
	if len(parts) > 1 {
		imageDigest = parts[1]
	}
	imgDetails := map[string]any{"name": imageName, "tag": "", "image": imageName, "imageDigest": imageDigest}
	if sep := strings.LastIndex(imageName, ":"); sep >= 0 && !strings.Contains(imageName[sep:], "/") {
		imgDetails["name"] = imageName[:sep]
		imgDetails["tag"] = imageName[sep+1:]
	}

	mode := serviceMode(&spec)
	var stackVal any
	if st := svcLabels["com.docker.stack.namespace"]; st != "" {
		stackVal = st
	}

	// agent / immutable: Clojure checks key existence, not value
	var agentVal, immutableVal any
	if _, ok := svcLabels["swarmpit.agent"]; ok {
		agentVal = true
	}
	if _, ok := svcLabels["swarmpit.service.immutable"]; ok {
		immutableVal = true
	}

	// links
	linkPrefix := "swarmpit.service.link."
	links := []map[string]string{}
	for k, v := range svcLabels {
		if strings.HasPrefix(k, linkPrefix) {
			links = append(links, map[string]string{"name": k[len(linkPrefix):], "value": v})
		}
	}

	// replicas
	var replicas any
	if mode == "replicated" && spec.Mode.Replicated != nil && spec.Mode.Replicated.Replicas != nil {
		replicas = int(*spec.Mode.Replicated.Replicas)
	}

	// count running and total from tasks
	running := 0
	noShutdown := 0
	for _, t := range tasks {
		if t.ServiceID != s.ID {
			continue
		}
		if t.DesiredState != swarm.TaskStateShutdown {
			noShutdown++
		}
		if t.Status.State == swarm.TaskStateRunning && t.DesiredState == swarm.TaskStateRunning {
			running++
		}
	}

	var total any
	if mode == "replicated" {
		total = replicas
	} else {
		total = noShutdown
	}

	// state
	var cmpTotal int
	if mode == "replicated" && replicas != nil {
		cmpTotal = replicas.(int)
	} else {
		cmpTotal = noShutdown
	}
	state := "not running"
	if running > 0 {
		if running < cmpTotal {
			state = "partly running"
		} else {
			state = "running"
		}
	}

	// ports
	ports := []map[string]any{}
	for _, p := range s.Endpoint.Ports {
		ports = append(ports, map[string]any{
			"containerPort": p.TargetPort, "protocol": string(p.Protocol),
			"mode": string(p.PublishMode), "hostPort": p.PublishedPort,
		})
	}

	// mounts
	mounts := []map[string]any{}
	for _, m := range cs.Mounts {
		mt := map[string]any{
			"containerPath": m.Target, "host": m.Source,
			"type": string(m.Type), "id": nil,
			"volumeOptions": nil, "readOnly": m.ReadOnly, "stack": nil,
		}
		if string(m.Type) == "volume" {
			mt["id"] = m.Source
		}
		if m.VolumeOptions != nil {
			vo := map[string]any{
				"labels": nilLabels(m.VolumeOptions.Labels),
				"driver": nil,
			}
			if m.VolumeOptions.DriverConfig != nil {
				vo["driver"] = map[string]any{
					"name":    m.VolumeOptions.DriverConfig.Name,
					"options": m.VolumeOptions.DriverConfig.Options,
				}
			}
			mt["volumeOptions"] = vo
			if m.VolumeOptions.Labels != nil {
				mt["stack"] = nilStr(m.VolumeOptions.Labels["com.docker.stack.namespace"])
			}
		}
		mounts = append(mounts, mt)
	}

	// networks: resolve targets to full network objects
	svcNets := []map[string]any{}
	netMap := map[string]types.NetworkResource{}
	for _, net := range networks {
		netMap[net.ID] = net
	}
	// also check host network from task attachments
	for _, t := range tasks {
		if t.ServiceID != s.ID {
			continue
		}
		for _, na := range t.NetworksAttachments {
			if na.Network.Spec.Name == "host" && na.Network.ID != "" {
				for _, hn := range networks {
					if hn.Driver == "host" {
						netMap[na.Network.ID] = hn
						break
					}
				}
			}
		}
	}
	for _, n := range tt.Networks {
		if nr, ok := netMap[n.Target]; ok {
			mapped := mapNetwork(nr)
			mapped["serviceAliases"] = n.Aliases
			svcNets = append(svcNets, mapped)
		}
	}

	// secrets
	secrets := []map[string]any{}
	for _, sec := range cs.Secrets {
		sm := map[string]any{
			"id": sec.SecretID, "secretName": sec.SecretName,
			"secretTarget": nil, "uid": nil, "gid": nil, "mode": nil,
		}
		if sec.File != nil {
			sm["secretTarget"] = sec.File.Name
			sm["uid"] = sec.File.UID
			sm["gid"] = sec.File.GID
			sm["mode"] = sec.File.Mode
		}
		secrets = append(secrets, sm)
	}

	// configs
	configs := []map[string]any{}
	for _, cfg := range cs.Configs {
		cm := map[string]any{
			"id": cfg.ConfigID, "configName": cfg.ConfigName,
			"configTarget": nil, "uid": nil, "gid": nil, "mode": nil,
		}
		if cfg.File != nil {
			cm["configTarget"] = cfg.File.Name
			cm["uid"] = cfg.File.UID
			cm["gid"] = cfg.File.GID
			cm["mode"] = cfg.File.Mode
		}
		configs = append(configs, cm)
	}

	// hosts
	hosts := []map[string]string{}
	for _, h := range cs.Hosts {
		p := strings.SplitN(h, " ", 2)
		if len(p) == 2 {
			hosts = append(hosts, map[string]string{"name": p[1], "value": p[0]})
		}
	}

	// variables
	variables := []map[string]any{}
	for _, e := range cs.Env {
		p := strings.SplitN(e, "=", 2)
		var v any
		if len(p) > 1 {
			v = p[1]
		}
		variables = append(variables, map[string]any{"name": p[0], "value": v})
	}

	// labels: filter out swarmpit.* and com.docker.*
	labels := []map[string]string{}
	for k, v := range svcLabels {
		if !strings.HasPrefix(k, "swarmpit") && !strings.HasPrefix(k, "com.docker") {
			labels = append(labels, map[string]string{"name": k, "value": v})
		}
	}

	// containerLabels: filter out com.docker.*
	containerLabels := []map[string]string{}
	for k, v := range cs.Labels {
		if !strings.HasPrefix(k, "com.docker") {
			containerLabels = append(containerLabels, map[string]string{"name": k, "value": v})
		}
	}

	// sysctls
	sysctls := mapToNameValue(cs.Sysctls)

	// isolation
	var isolation any
	if cs.Isolation != "" {
		isolation = string(cs.Isolation)
	}

	// command / entrypoint / hostname / user / dir / tty
	var command, entrypoint any
	if len(cs.Args) > 0 {
		command = cs.Args
	}
	if len(cs.Command) > 0 {
		entrypoint = cs.Command
	}

	var ttyVal any
	if cs.TTY {
		ttyVal = true
	}

	// healthcheck
	var healthcheck any
	if cs.Healthcheck != nil {
		healthcheck = map[string]any{
			"test":     cs.Healthcheck.Test,
			"interval": nanoToSec(int64(cs.Healthcheck.Interval)),
			"timeout":  nanoToSec(int64(cs.Healthcheck.Timeout)),
			"retries":  cs.Healthcheck.Retries,
		}
	}

	// logdriver: fallback to system default
	logName := info.LoggingDriver
	if tt.LogDriver != nil && tt.LogDriver.Name != "" {
		logName = tt.LogDriver.Name
	}
	logOpts := []map[string]string{}
	if tt.LogDriver != nil {
		logOpts = mapToNameValue(tt.LogDriver.Options)
	}

	// update status
	var updateState, updateMsg any
	if s.UpdateStatus != nil {
		if s.UpdateStatus.State != "" {
			updateState = string(s.UpdateStatus.State)
		}
		if s.UpdateStatus.Message != "" {
			updateMsg = s.UpdateStatus.Message
		}
	}

	// deployment.update
	upd := map[string]any{"parallelism": uint64(1), "delay": int64(0), "order": "stop-first", "failureAction": "pause"}
	if spec.UpdateConfig != nil {
		upd["parallelism"] = spec.UpdateConfig.Parallelism
		upd["delay"] = nanoToSec(int64(spec.UpdateConfig.Delay))
		if spec.UpdateConfig.Order != "" {
			upd["order"] = spec.UpdateConfig.Order
		}
		if spec.UpdateConfig.FailureAction != "" {
			upd["failureAction"] = spec.UpdateConfig.FailureAction
		}
	}

	// deployment.rollback
	rb := map[string]any{"parallelism": uint64(1), "delay": int64(0), "order": "stop-first", "failureAction": "pause"}
	if spec.RollbackConfig != nil {
		rb["parallelism"] = spec.RollbackConfig.Parallelism
		rb["delay"] = nanoToSec(int64(spec.RollbackConfig.Delay))
		if spec.RollbackConfig.Order != "" {
			rb["order"] = spec.RollbackConfig.Order
		}
		if spec.RollbackConfig.FailureAction != "" {
			rb["failureAction"] = spec.RollbackConfig.FailureAction
		}
	}

	// deployment.restartPolicy
	rp := map[string]any{"condition": "any", "delay": int64(5), "window": int64(0), "attempts": uint64(0)}
	if tt.RestartPolicy != nil {
		if tt.RestartPolicy.Condition != "" {
			rp["condition"] = string(tt.RestartPolicy.Condition)
		}
		if tt.RestartPolicy.Delay != nil {
			rp["delay"] = nanoToSec(int64(*tt.RestartPolicy.Delay))
		}
		if tt.RestartPolicy.MaxAttempts != nil {
			rp["attempts"] = *tt.RestartPolicy.MaxAttempts
		}
		if tt.RestartPolicy.Window != nil {
			rp["window"] = nanoToSec(int64(*tt.RestartPolicy.Window))
		}
	}

	// placement
	placement := []map[string]string{}
	if tt.Placement != nil {
		for _, c := range tt.Placement.Constraints {
			placement = append(placement, map[string]string{"rule": c})
		}
	}

	// maxReplicas
	var maxReplicas any
	if tt.Placement != nil && tt.Placement.MaxReplicas > 0 {
		maxReplicas = tt.Placement.MaxReplicas
	}

	// autoredeploy
	autoredeploy := svcLabels["swarmpit.service.deployment.autoredeploy"] == "true"

	return map[string]any{
		"id": s.ID, "version": s.Version.Index,
		"createdAt": s.CreatedAt, "updatedAt": s.UpdatedAt,
		"repository":  imgDetails,
		"serviceName": spec.Name, "mode": mode,
		"stack": stackVal, "agent": agentVal, "immutable": immutableVal,
		"links": links, "replicas": replicas,
		"state": state,
		"status": map[string]any{
			"tasks":   map[string]any{"running": running, "total": total},
			"update":  updateState,
			"message": updateMsg,
		},
		"ports": ports, "mounts": mounts, "networks": svcNets,
		"secrets": secrets, "configs": configs,
		"hosts": hosts, "variables": variables,
		"labels": labels, "containerLabels": containerLabels,
		"command": command, "entrypoint": entrypoint,
		"hostname": nilStr(cs.Hostname), "isolation": isolation,
		"sysctls": sysctls,
		"user": nilStr(cs.User), "dir": nilStr(cs.Dir), "tty": ttyVal,
		"healthcheck": healthcheck,
		"logdriver":   map[string]any{"name": logName, "opts": logOpts},
		"resources":   serviceResources(&tt),
		"deployment": map[string]any{
			"update": upd, "forceUpdate": tt.ForceUpdate,
			"restartPolicy": rp, "rollback": rb,
			"rollbackAllowed": s.PreviousSpec != nil,
			"autoredeploy": autoredeploy,
			"placement": placement, "maxReplicas": maxReplicas,
		},
	}
}

func mapServices(services []swarm.Service, tasks []swarm.Task, networks []types.NetworkResource, info system.Info) []map[string]any {
	r := make([]map[string]any, len(services))
	for i, s := range services {
		r[i] = mapService(s, tasks, networks, info)
	}
	return r
}

func mapStack(name string, services []swarm.Service, tasks []swarm.Task, networks []types.NetworkResource, info system.Info) map[string]any {
	var stackSvcs []map[string]any
	for _, s := range services {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			stackSvcs = append(stackSvcs, mapService(s, tasks, networks, info))
		}
	}
	if stackSvcs == nil {
		stackSvcs = []map[string]any{}
	}
	state := "deployed"
	if len(stackSvcs) == 0 {
		state = "inactive"
	}
	return map[string]any{
		"stackName": name, "state": state, "services": stackSvcs,
		"networks": []any{}, "volumes": []any{}, "configs": []any{}, "secrets": []any{},
	}
}
