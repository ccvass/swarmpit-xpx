package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

var cli *client.Client

func Init() error {
	var err error
	cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("docker client init: %w", err)
	}
	return nil
}

// InitWithHost reinitializes the Docker client to point at a remote host (#104).
func InitWithHost(host string) error {
	if host == "" {
		return Init()
	}
	var err error
	cli, err = client.NewClientWithOpts(
		client.WithHost(host),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("docker client init with host %s: %w", host, err)
	}
	// Verify connectivity
	if _, err := Ping(); err != nil {
		// Rollback to local on failure
		_ = Init()
		return fmt.Errorf("cluster unreachable at %s: %w", host, err)
	}
	return nil
}

func withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

func Ping() (types.Ping, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	p, err := cli.Ping(ctx)
	if err != nil {
		return p, fmt.Errorf("docker ping: %w", err)
	}
	return p, nil
}

func Info() (system.Info, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	info, err := cli.Info(ctx)
	if err != nil {
		return info, fmt.Errorf("docker info: %w", err)
	}
	return info, nil
}

func Version() (types.Version, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	v, err := cli.ServerVersion(ctx)
	if err != nil {
		return v, fmt.Errorf("docker version: %w", err)
	}
	return v, nil
}

func Nodes() ([]swarm.Node, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	n, err := cli.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	return n, nil
}

func Node(id string) (swarm.Node, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	n, _, err := cli.NodeInspectWithRaw(ctx, id)
	if err != nil {
		return n, fmt.Errorf("inspect node %s: %w", id, err)
	}
	return n, nil
}

func Services() ([]swarm.Service, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	s, err := cli.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return s, nil
}

func Service(id string) (swarm.Service, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	s, _, err := cli.ServiceInspectWithRaw(ctx, id, types.ServiceInspectOptions{})
	if err != nil {
		return s, fmt.Errorf("inspect service %s: %w", id, err)
	}
	return s, nil
}

func Tasks() ([]swarm.Task, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	t, err := cli.TaskList(ctx, types.TaskListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return t, nil
}

func ServiceTasks(serviceID string, running bool) ([]swarm.Task, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	f := filters.NewArgs(filters.Arg("service", serviceID))
	if running {
		f.Add("desired-state", "running")
	}
	t, err := cli.TaskList(ctx, types.TaskListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("list tasks for service %s: %w", serviceID, err)
	}
	return t, nil
}

func Networks() ([]types.NetworkResource, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	n, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return n, nil
}

func Secrets() ([]swarm.Secret, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	s, err := cli.SecretList(ctx, types.SecretListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return s, nil
}

func Configs() ([]swarm.Config, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	c, err := cli.ConfigList(ctx, types.ConfigListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}
	return c, nil
}

func UpdateService(id string, version swarm.Version, spec swarm.ServiceSpec) error {
	ctx, cancel := withTimeout()
	defer cancel()
	_, err := cli.ServiceUpdate(ctx, id, version, spec, types.ServiceUpdateOptions{})
	if err != nil {
		return fmt.Errorf("update service %s: %w", id, err)
	}
	return nil
}

func Volumes() (volume.ListResponse, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	v, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return v, fmt.Errorf("list volumes: %w", err)
	}
	return v, nil
}

func ServiceLogs(id string, tail string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if tail == "" {
		tail = "100"
	}
	reader, err := cli.ServiceLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: tail, Timestamps: true})
	if err != nil {
		return "", fmt.Errorf("service logs %s: %w", id, err)
	}
	defer reader.Close()
	var buf bytes.Buffer
	// Demultiplex Docker log stream (8-byte header per frame)
	if _, err := stdcopy.StdCopy(&buf, &buf, reader); err != nil {
		// Fallback: raw TTY stream (no multiplexing)
		buf.Reset()
		if _, err := io.Copy(&buf, reader); err != nil {
			return "", fmt.Errorf("read logs %s: %w", id, err)
		}
	}
	return buf.String(), nil
}

func DeleteService(id string) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.ServiceRemove(ctx, id)
}

func DeleteSecret(id string) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.SecretRemove(ctx, id)
}

func DeleteConfig(id string) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.ConfigRemove(ctx, id)
}

func DeleteNetwork(id string) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.NetworkRemove(ctx, id)
}

func CreateService(spec swarm.ServiceSpec) (swarm.ServiceCreateResponse, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.ServiceCreate(ctx, spec, types.ServiceCreateOptions{})
}

func CreateNetwork(name, driver string, internal, attachable bool) (network.CreateResponse, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:     driver,
		Attachable: attachable,
		Internal:   internal,
	})
}

func CreateVolume(name, driver string) (*volume.Volume, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	v, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: name, Driver: driver})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func CreateSecret(spec swarm.SecretSpec) (types.SecretCreateResponse, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.SecretCreate(ctx, spec)
}

func CreateConfig(spec swarm.ConfigSpec) (types.ConfigCreateResponse, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.ConfigCreate(ctx, spec)
}

func SecretInspect(id string) (swarm.Secret, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	s, _, err := cli.SecretInspectWithRaw(ctx, id)
	return s, err
}

func ConfigInspect(id string) (swarm.Config, error) {
	ctx, cancel := withTimeout()
	defer cancel()
	c, _, err := cli.ConfigInspectWithRaw(ctx, id)
	return c, err
}

func UpdateSecret(id string, version swarm.Version, spec swarm.SecretSpec) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.SecretUpdate(ctx, id, version, spec)
}

func UpdateConfig(id string, version swarm.Version, spec swarm.ConfigSpec) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.ConfigUpdate(ctx, id, version, spec)
}

func NodeUpdate(id string, version swarm.Version, spec swarm.NodeSpec) error {
	ctx, cancel := withTimeout()
	defer cancel()
	return cli.NodeUpdate(ctx, id, version, spec)
}

func Client() *client.Client { return cli }

func ImageListOpts(dangling bool) image.ListOptions {
	f := filters.NewArgs()
	if dangling {
		f.Add("dangling", "true")
	}
	return image.ListOptions{Filters: f}
}
