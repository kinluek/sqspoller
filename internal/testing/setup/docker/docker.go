package docker

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"io/ioutil"
	"regexp"
	"testing"
	"time"
)

const localstackImage = "localstack/localstack:0.10.7"

// Container represents a docker container and holds the information required
// for communicating with the it.
type Container struct {
	ID           string
	ExposedPorts map[string][]nat.PortBinding
	running      bool  // flag to indicate whether the container has already been stopped.
}

// newClient creates a new docker client.
func newClient(t *testing.T) *client.Client {
	t.Helper()
	cli, err := client.NewEnvClient()
	if err != nil {
		t.Fatalf("could not create docker client: %v", err)
	}
	return cli
}

// StartLocalStackContainer runs a localstack container to mock AWS services.
// Provide configuration using envars.
func StartLocalStackContainer(t *testing.T, envars map[string]string) *Container {
	t.Helper()
	cli := newClient(t)
	defer cli.Close()

	ctx := context.Background()

	// Make sure we have the image to start the container from.
	imageCheckAndPull(t, ctx, cli, localstackImage)

	// Create container
	containerConfig := container.Config{
		Env:   listify(envars),
		Image: localstackImage,
	}
	hostConfig := container.HostConfig{
		AutoRemove:      true,
		PublishAllPorts: true,
	}
	container, err := cli.ContainerCreate(ctx, &containerConfig, &hostConfig, nil, "")
	if err != nil {
		t.Fatalf("could not create container: %v", err)
	}

	// Start container
	err = cli.ContainerStart(ctx, container.ID, types.ContainerStartOptions{})
	if err != nil {
		t.Fatalf("could not start container %s: %v", container.ID[:12], err)
	}

	// Inspect container to find host configuration
	info, err := cli.ContainerInspect(ctx, container.ID)
	if err != nil {
		t.Fatalf("could not inspect container %s: %v", container.ID[:12], err)
	}
	exposedPorts := mapPorts(info.NetworkSettings.Ports)

	return &Container{
		ID:           container.ID,
		ExposedPorts: exposedPorts,
		running:      true,
	}
}

// StopContainer stops and removes a running container.
func StopContainer(t *testing.T, container *Container, timeout time.Duration) {
	t.Helper()
	if !container.running {
		return
	}
	cli := newClient(t)
	defer cli.Close()

	ctx := context.Background()

	// container alias for logging
	alias := container.ID[:12]

	rmfConfig := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   true,
		Force:         true,
	}

	// ContainerStop call should stop and remove the container, as containers can
	// only be created with the StartContainer function which sets the AutoRemove
	// config to true.
	if err := cli.ContainerStop(ctx, container.ID, &timeout); err != nil {
		t.Logf("could not stop container: %v: %v", alias, err)
		t.Logf("attempting to force remove container..")
		if err := cli.ContainerRemove(ctx, container.ID, rmfConfig); err != nil {
			t.Fatalf("could not forcefully remove container %v", alias)
		}
		t.Logf("container %v was forced removed", alias)
		return
	}
}

// NetworkConnect connects a container to the given network.
func NetworkConnect(t *testing.T, network string, containerID string) {
	t.Helper()
	cli := newClient(t)
	defer cli.Close()

	ctx := context.Background()

	if err := cli.NetworkConnect(ctx, network, containerID, nil); err != nil {
		t.Fatalf("could not connect container %v, to netowork %v", containerID[:12], network)
	}
}

// imageCheckAndPull checks to see if the image exists on the machine, if it doesn't
// the image is pulled from docker.io.
func imageCheckAndPull(t *testing.T, ctx context.Context, cli *client.Client, image string) {
	t.Helper()

	filters := filters.NewArgs()
	filters.Add("reference", image)
	images, err := cli.ImageList(ctx, types.ImageListOptions{
		All:     false,
		Filters: filters,
	})
	if err != nil {
		t.Fatalf("could not list images %v", err)
	}
	if len(images) == 0 {
		t.Logf("could not find %v locally", image)
		t.Logf("pulling image %v from docker.io...", image)
		r, err := cli.ImagePull(ctx, "docker.io/" + image, types.ImagePullOptions{

		})
		if err != nil {
			t.Fatalf("could not pull %v image %v", image, err)
		}
		_, err = ioutil.ReadAll(r)
		if err != nil {
			t.Fatalf("error downloading %v image %v", image, err)
		}
		t.Logf("finished downloading image %v", image)
	}
}


func mapPorts(m nat.PortMap) map[string][]nat.PortBinding {
	exposedPorts := make(map[string][]nat.PortBinding)
	portReg := regexp.MustCompile(`^\d+`)
	for key, value := range m {
		containerPort := portReg.Find([]byte(key))
		exposedPorts[string(containerPort)] = value
	}
	return exposedPorts
}

func listify(m map[string]string) []string {
	if m == nil {
		return nil
	}
	list := make([]string, 0)
	for key, value := range m {
		list = append(list, key+"="+value)
	}
	return list
}
