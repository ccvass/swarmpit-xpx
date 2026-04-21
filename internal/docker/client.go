package docker

import (
	"context"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
)

var cli *client.Client

func Init() error {
	var err error
	cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	return err
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

func Ping() (types.Ping, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.Ping(c)
}

func Info() (system.Info, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.Info(c)
}

func Nodes() ([]swarm.Node, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.NodeList(c, types.NodeListOptions{})
}

func Node(id string) (swarm.Node, error) {
	c, cancel := ctx()
	defer cancel()
	node, _, err := cli.NodeInspectWithRaw(c, id)
	return node, err
}

func Services() ([]swarm.Service, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.ServiceList(c, types.ServiceListOptions{})
}

func Service(id string) (swarm.Service, error) {
	c, cancel := ctx()
	defer cancel()
	svc, _, err := cli.ServiceInspectWithRaw(c, id, types.ServiceInspectOptions{})
	return svc, err
}

func Tasks() ([]swarm.Task, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.TaskList(c, types.TaskListOptions{})
}

func ServiceTasks(serviceID string, running bool) ([]swarm.Task, error) {
	c, cancel := ctx()
	defer cancel()
	f := filters.NewArgs(filters.Arg("service", serviceID))
	if running {
		f.Add("desired-state", "running")
	}
	return cli.TaskList(c, types.TaskListOptions{Filters: f})
}

func Networks() ([]types.NetworkResource, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.NetworkList(c, types.NetworkListOptions{})
}

func Secrets() ([]swarm.Secret, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.SecretList(c, types.SecretListOptions{})
}

func Configs() ([]swarm.Config, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.ConfigList(c, types.ConfigListOptions{})
}

func Version() (types.Version, error) {
	c, cancel := ctx()
	defer cancel()
	return cli.ServerVersion(c)
}
