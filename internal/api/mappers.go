package api

import (
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
)

func mapNode(n swarm.Node) map[string]any {
	role := "worker"
	if n.Spec.Role == swarm.NodeRoleManager {
		role = "manager"
	}
	var leader any
	if n.ManagerStatus != nil && n.ManagerStatus.Leader {
		leader = true
	}
	labels := []map[string]string{}
	for k, v := range n.Spec.Labels {
		labels = append(labels, map[string]string{"name": k, "value": v})
	}
	plugins := map[string][]string{"networks": {}, "volumes": {}}
	for _, p := range n.Description.Engine.Plugins {
		if p.Type == "Network" {
			plugins["networks"] = append(plugins["networks"], p.Name)
		} else if p.Type == "Volume" {
			plugins["volumes"] = append(plugins["volumes"], p.Name)
		}
	}
	return map[string]any{
		"id": n.ID, "version": n.Version.Index, "nodeName": n.Description.Hostname,
		"role": role, "state": string(n.Status.State), "availability": string(n.Spec.Availability),
		"address": n.Status.Addr, "engine": n.Description.Engine.EngineVersion,
		"os": n.Description.Platform.OS, "arch": n.Description.Platform.Architecture,
		"leader": leader, "labels": labels, "plugins": plugins,
		"resources": map[string]any{
			"cpu":    float64(n.Description.Resources.NanoCPUs) / 1e9,
			"memory": n.Description.Resources.MemoryBytes / (1024 * 1024),
		},
		"stats": nil,
	}
}

func mapNodes(nodes []swarm.Node) []map[string]any {
	r := make([]map[string]any, len(nodes))
	for i, n := range nodes {
		r[i] = mapNode(n)
	}
	return r
}

func mapService(s swarm.Service, tasks []swarm.Task) map[string]any {
	spec := s.Spec
	cs := spec.TaskTemplate.ContainerSpec
	name, tag, digest := parseImage(cs.Image)
	mode := "replicated"
	replicas := 1
	if spec.Mode.Global != nil {
		mode = "global"
		replicas = 0
	} else if spec.Mode.Replicated != nil && spec.Mode.Replicated.Replicas != nil {
		replicas = int(*spec.Mode.Replicated.Replicas)
	}
	stack := spec.Labels["com.docker.stack.namespace"]
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
	} else if total == 0 {
		state = "not running"
	}
	ports := []map[string]any{}
	for _, p := range s.Endpoint.Ports {
		ports = append(ports, map[string]any{"containerPort": p.TargetPort, "hostPort": p.PublishedPort, "protocol": string(p.Protocol), "mode": string(p.PublishMode)})
	}
	mounts := []map[string]any{}
	for _, m := range cs.Mounts {
		mounts = append(mounts, map[string]any{"containerPath": m.Target, "host": m.Source, "type": string(m.Type), "id": nil, "volumeOptions": nil, "readOnly": m.ReadOnly, "stack": nil})
	}
	networks := []map[string]any{}
	for _, n := range spec.TaskTemplate.Networks {
		networks = append(networks, map[string]any{"networkName": n.Target, "id": n.Target})
	}
	variables := []map[string]string{}
	for _, e := range cs.Env {
		p := strings.SplitN(e, "=", 2)
		v := ""
		if len(p) > 1 {
			v = p[1]
		}
		variables = append(variables, map[string]string{"name": p[0], "value": v})
	}
	labels := []map[string]string{}
	for k, v := range spec.Labels {
		labels = append(labels, map[string]string{"name": k, "value": v})
	}
	containerLabels := []map[string]string{}
	for k, v := range cs.Labels {
		containerLabels = append(containerLabels, map[string]string{"name": k, "value": v})
	}
	secrets := []map[string]any{}
	for _, sec := range cs.Secrets {
		secrets = append(secrets, map[string]any{"secretName": sec.SecretName, "secretTarget": sec.File.Name})
	}
	configs := []map[string]any{}
	for _, cfg := range cs.Configs {
		configs = append(configs, map[string]any{"configName": cfg.ConfigName, "configTarget": cfg.File.Name})
	}
	res := map[string]map[string]any{"reservation": {"cpu": 0.0, "memory": 0}, "limit": {"cpu": 0.0, "memory": 0}}
	if spec.TaskTemplate.Resources != nil {
		if r := spec.TaskTemplate.Resources.Reservations; r != nil {
			res["reservation"] = map[string]any{"cpu": float64(r.NanoCPUs) / 1e9, "memory": r.MemoryBytes / (1024 * 1024)}
		}
		if l := spec.TaskTemplate.Resources.Limits; l != nil {
			res["limit"] = map[string]any{"cpu": float64(l.NanoCPUs) / 1e9, "memory": l.MemoryBytes / (1024 * 1024)}
		}
	}
	deployment := map[string]any{"autoredeploy": false, "placement": []any{}}
	if spec.UpdateConfig != nil {
		deployment["update"] = map[string]any{"parallelism": spec.UpdateConfig.Parallelism, "delay": int64(spec.UpdateConfig.Delay.Seconds()), "order": string(spec.UpdateConfig.Order), "failureAction": string(spec.UpdateConfig.FailureAction)}
	}
	if spec.TaskTemplate.RestartPolicy != nil {
		deployment["restartPolicy"] = map[string]any{"condition": string(spec.TaskTemplate.RestartPolicy.Condition), "delay": int64(spec.TaskTemplate.RestartPolicy.Delay.Seconds()), "attempts": spec.TaskTemplate.RestartPolicy.MaxAttempts, "window": 0}
	}
	if spec.TaskTemplate.Placement != nil {
		pl := []map[string]string{}
		for _, c := range spec.TaskTemplate.Placement.Constraints {
			pl = append(pl, map[string]string{"rule": c})
		}
		deployment["placement"] = pl
	}
	updateStatus, updateMsg := "", ""
	if s.UpdateStatus != nil {
		updateStatus = string(s.UpdateStatus.State)
		updateMsg = s.UpdateStatus.Message
	}
	agent := spec.Labels["swarmpit.agent"] == "true"
	immutable := spec.Labels["swarmpit.service.immutable"] == "true"
	var agentVal, immutableVal any
	if agent {
		agentVal = true
	}
	if immutable {
		immutableVal = true
	}
	hosts := []map[string]string{}
	for _, h := range cs.Hosts {
		p := strings.SplitN(h, " ", 2)
		if len(p) == 2 {
			hosts = append(hosts, map[string]string{"name": p[1], "value": p[0]})
		}
	}
	logdriver := map[string]any{"name": nil, "opts": []any{}}
	if spec.TaskTemplate.LogDriver != nil {
		opts := []map[string]string{}
		for k, v := range spec.TaskTemplate.LogDriver.Options {
			opts = append(opts, map[string]string{"name": k, "value": v})
		}
		logdriver = map[string]any{"name": spec.TaskTemplate.LogDriver.Name, "opts": opts}
	}
	return map[string]any{
		"id": s.ID, "version": s.Version.Index, "createdAt": s.CreatedAt, "updatedAt": s.UpdatedAt,
		"serviceName": spec.Name, "mode": mode, "stack": stack, "replicas": replicas, "state": state,
		"agent": agentVal, "immutable": immutableVal, "links": []any{},
		"repository": map[string]string{"name": name, "tag": tag, "image": name + ":" + tag, "imageDigest": digest},
		"status":          map[string]any{"tasks": map[string]int{"running": running, "total": total}, "update": updateStatus, "message": updateMsg},
		"ports": ports, "mounts": mounts, "networks": networks, "secrets": secrets, "configs": configs,
		"hosts": hosts, "variables": variables, "labels": labels, "containerLabels": containerLabels,
		"command": cs.Args, "entrypoint": cs.Command, "hostname": cs.Hostname, "isolation": nil, "sysctls": nil,
		"user": cs.User, "dir": cs.Dir, "tty": cs.TTY, "healthcheck": nil,
		"logdriver": logdriver, "resources": res, "deployment": deployment,
	}
}

func mapServices(services []swarm.Service, tasks []swarm.Task) []map[string]any {
	r := make([]map[string]any, len(services))
	for i, s := range services {
		r[i] = mapService(s, tasks)
	}
	return r
}

func mapTask(t swarm.Task, nodes []swarm.Node, services []swarm.Service) map[string]any {
	image := ""
	if t.Spec.ContainerSpec != nil {
		image = t.Spec.ContainerSpec.Image
	}
	name, tag, digest := parseImage(image)
	nodeName := t.NodeID
	for _, n := range nodes {
		if n.ID == t.NodeID {
			nodeName = n.Description.Hostname
			break
		}
	}
	svcName := t.ServiceID
	for _, s := range services {
		if s.ID == t.ServiceID {
			svcName = s.Spec.Name
			break
		}
	}
	slot := ""
	if t.Slot > 0 {
		slot = "." + strings.TrimLeft(strings.Replace(string(rune(t.Slot+'0')), "\x00", "", -1), "\x00")
	}
	res := map[string]map[string]any{"reservation": {"cpu": 0.0, "memory": 0}, "limit": {"cpu": 0.0, "memory": 0}}
	if t.Spec.Resources != nil {
		if r := t.Spec.Resources.Reservations; r != nil {
			res["reservation"] = map[string]any{"cpu": float64(r.NanoCPUs) / 1e9, "memory": r.MemoryBytes / (1024 * 1024)}
		}
		if l := t.Spec.Resources.Limits; l != nil {
			res["limit"] = map[string]any{"cpu": float64(l.NanoCPUs) / 1e9, "memory": l.MemoryBytes / (1024 * 1024)}
		}
	}
	logdriver := "json-file"
	if t.Spec.LogDriver != nil {
		logdriver = t.Spec.LogDriver.Name
	}
	return map[string]any{
		"id": t.ID, "version": t.Version.Index, "createdAt": t.CreatedAt, "updatedAt": t.UpdatedAt,
		"taskName": svcName + slot, "nodeName": nodeName, "nodeId": t.NodeID,
		"serviceName": svcName, "serviceId": t.ServiceID,
		"repository":  map[string]any{"image": name + ":" + tag, "imageDigest": digest},
		"state": string(t.Status.State), "desiredState": string(t.DesiredState),
		"status": map[string]any{"error": t.Status.Err, "message": t.Status.Message},
		"logdriver": logdriver, "resources": res, "stats": nil,
	}
}

func mapTasks(tasks []swarm.Task, nodes []swarm.Node, services []swarm.Service) []map[string]any {
	r := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		r[i] = mapTask(t, nodes, services)
	}
	return r
}

func mapNetwork(n types.NetworkResource) map[string]any {
	stack := n.Labels["com.docker.stack.namespace"]
	options := []map[string]string{}
	for k, v := range n.Options {
		options = append(options, map[string]string{"name": k, "value": v})
	}
	var ipam map[string]string
	if len(n.IPAM.Config) > 0 {
		ipam = map[string]string{"subnet": n.IPAM.Config[0].Subnet, "gateway": n.IPAM.Config[0].Gateway}
	}
	var labelsVal any
	if len(n.Labels) > 0 {
		labelsVal = n.Labels
	}
	return map[string]any{
		"id": n.ID, "networkName": n.Name, "created": n.Created, "scope": n.Scope,
		"driver": n.Driver, "internal": n.Internal, "attachable": n.Attachable,
		"ingress": n.Ingress, "enableIPv6": n.EnableIPv6,
		"options": options, "labels": labelsVal, "stack": stack, "ipam": ipam,
	}
}

func mapNetworks(nets []types.NetworkResource) []map[string]any {
	r := make([]map[string]any, len(nets))
	for i, n := range nets {
		r[i] = mapNetwork(n)
	}
	return r
}

func mapSecret(s swarm.Secret) map[string]any {
	return map[string]any{
		"id": s.ID, "version": s.Version.Index, "secretName": s.Spec.Name,
		"createdAt": s.CreatedAt, "updatedAt": s.UpdatedAt,
	}
}

func mapSecrets(secrets []swarm.Secret) []map[string]any {
	r := make([]map[string]any, len(secrets))
	for i, s := range secrets {
		r[i] = mapSecret(s)
	}
	return r
}

func mapConfig(c swarm.Config) map[string]any {
	return map[string]any{
		"id": c.ID, "version": c.Version.Index, "configName": c.Spec.Name,
		"createdAt": c.CreatedAt, "updatedAt": c.UpdatedAt,
		"data": string(c.Spec.Data),
	}
}

func mapConfigs(configs []swarm.Config) []map[string]any {
	r := make([]map[string]any, len(configs))
	for i, c := range configs {
		r[i] = mapConfig(c)
	}
	return r
}

func mapVolume(v *volume.Volume) map[string]any {
	stack := v.Labels["com.docker.stack.namespace"]
	options := []map[string]string{}
	for k, val := range v.Options {
		options = append(options, map[string]string{"name": k, "value": val})
	}
	return map[string]any{
		"id": v.Name, "volumeName": v.Name, "driver": v.Driver,
		"stack": stack, "labels": v.Labels, "options": options,
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

func mapStack(name string, services []swarm.Service, tasks []swarm.Task) map[string]any {
	var stackSvcs []map[string]any
	for _, s := range services {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			stackSvcs = append(stackSvcs, mapService(s, tasks))
		}
	}
	if stackSvcs == nil {
		stackSvcs = []map[string]any{}
	}
	return map[string]any{"stackName": name, "state": "deployed", "services": stackSvcs}
}

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
		tag = "latest"
	}
	return
}
