package docker

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Container represents a docker container
// and holds the information for communicating
// with the running docker container.
type Container struct {
	t            *testing.T
	ID           string
	ExposedPorts map[string][]Port // the map keys are the exposed container ports
	Volumes      []string
}

type Port struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// StartLocalStackContainer spins up a localstack container to
// mock aws services, provide configuration with envars and give
// a temp directory for to bind the container /tmp/localstack directory
// to.
func StartLocalStackContainer(t *testing.T, envars map[string]string, tmpDirVolume string) *Container {
	t.Helper()

	envArgs := make([]string, 0)
	if envars != nil {
		for key, value := range envars {
			envArgs = append(envArgs, "-e", key+"="+value)
		}
	}

	args := []string{"container", "run", "-P", "-d", "-v", tmpDirVolume + ":/tmp/localstack"}
	args = append(args, envArgs...)
	args = append(args, "localstack/localstack")

	cmd := exec.Command("docker", args...)
	return execStartContainerCommand(t, cmd)
}

// execStartContainerCommand takes the start container command and executes it
// to return a *Container.
func execStartContainerCommand(t *testing.T, cmd *exec.Cmd) *Container {
	t.Helper()

	idBuf, err := cmd.Output()
	if err != nil {
		t.Fatalf("could not start container: %v: %v", string(idBuf), err)
	}
	containerID := strings.TrimSpace(string(idBuf))

	// get container info.
	cmd = exec.Command("docker", "container", "inspect", containerID)
	infoBuf, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("could not inspect container with id %v: %v", containerID, err)
	}

	var containerInfo []struct {
		ID      string    `json:"Id"`
		Created time.Time `json:"Created"`
		Mounts  []struct {
			Type string `json:"Type"`
			Name string `json:"Name"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Ports map[string][]Port `json:"Ports"`
		} `json:"NetworkSettings"`
	}

	if err := json.Unmarshal(infoBuf, &containerInfo); err != nil {
		t.Fatalf("could not unmarshal container info for container %v: %v", containerID, err)
	}

	// get volume associated with the container.
	volumes := make([]string, 0)
	for _, mount := range containerInfo[0].Mounts {
		if mount.Type == "volume" {
			volumes = append(volumes, mount.Name)
		}
	}

	exposedPorts := make(map[string][]Port)
	portReg := regexp.MustCompile(`^\d+`)

	for key, value := range containerInfo[0].NetworkSettings.Ports {
		containerPort := portReg.Find([]byte(key))
		exposedPorts[string(containerPort)] = value
	}

	return &Container{
		t:            t,
		ID:           containerID,
		ExposedPorts: exposedPorts,
		Volumes:      volumes,
	}
}

// NetworkConnect connects a container to the given network.
func NetworkConnect(t *testing.T, network string, containerID string) {
	t.Helper()

	cmd := exec.Command("docker", "network", "connect", network, containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to connect containers: %v, : %v", string(out), err)
	}
}

// Logs outputs the logs produced by the container.
func (c *Container) Logs() string {
	c.t.Helper()

	cmd := exec.Command("docker", "logs", c.ID)
	logs, err := cmd.CombinedOutput()
	if err != nil {
		c.t.Fatalf("could not get logs from container %v: %v", c.ID, err)
	}
	return string(logs)
}

// Cleanup stops and removes the container, it also removes any volumes
// created by the container. This should be called after for every creation
// of a container to avoid leaking system resources.
func (c *Container) Cleanup() {
	c.t.Helper()

	cmd := exec.Command("docker", "container", "stop", c.ID)
	if output, err := cmd.CombinedOutput(); err != nil {
		c.t.Fatalf("could not stop container: %v: %v: %v", c.ID, string(output), err)
	}
	cmd = exec.Command("docker", "container", "rm", c.ID)
	if output, err := cmd.CombinedOutput(); err != nil {
		c.t.Fatalf("could not remove container: %v: %v: %v", c.ID, string(output), err)
	}

	if c.Volumes != nil && len(c.Volumes) > 0 {
		args := []string{"volume", "rm"}
		args = append(args, c.Volumes...)
		cmd = exec.Command("docker", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			c.t.Fatalf("could not remove volumes for container: %v: %v: %v", c.ID, string(output), err)
		}
	}

}
