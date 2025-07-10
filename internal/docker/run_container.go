package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/sirupsen/logrus"
)

// RunContainer creates and starts a Docker container for a given application.
func (d *Client) RunContainer(app *database.App, imageName string) (string, error) {
	logrus.Infof("Running container for app %s from image %s on port %d", app.Name, imageName, app.Port)
	d.sendDockerEvent("container_start", "Starting container", map[string]interface{}{
		"image_name": imageName,
		"port":       app.Port,
	})

	hostConfig := d.buildHostConfig(app)
	containerConfig := d.buildContainerConfig(app, imageName)

	d.sendDockerEvent("container_step", "Creating container", map[string]interface{}{"step": "create_container"})
	resp, err := d.cli.ContainerCreate(context.Background(), containerConfig, hostConfig, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		d.sendDockerEvent("container_error", "Error creating container", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("error creating container: %w", err)
	}

	d.sendDockerEvent("container_step", "Starting container", map[string]interface{}{"step": "start_container", "container_id": resp.ID})
	if err := d.cli.ContainerStart(context.Background(), resp.ID, types.ContainerStartOptions{}); err != nil {
		d.sendDockerEvent("container_error", "Error starting container", map[string]interface{}{"error": err.Error(), "container_id": resp.ID})
		return "", fmt.Errorf("error starting container: %w", err)
	}

	d.sendDockerEvent("container_success", "Container running successfully", map[string]interface{}{
		"container_id": resp.ID,
		"port":         app.Port,
		"url":          fmt.Sprintf("http://localhost:%d", app.Port),
	})
	logrus.Infof("Container running successfully: %s (ID: %s, Port: %d)", imageName, resp.ID, app.Port)
	return resp.ID, nil
}

// buildHostConfig creates the host configuration for a container.
func (d *Client) buildHostConfig(app *database.App) *container.HostConfig {
	portBinding := nat.PortBinding{
		HostIP:   defaultHostIP,
		HostPort: fmt.Sprintf("%d", app.Port),
	}
	// Use the internal port from the app model for the binding
	internalPort := nat.Port(fmt.Sprintf("%d/tcp", app.Port))
	portMap := nat.PortMap{
		internalPort: []nat.PortBinding{portBinding},
	}

	return &container.HostConfig{
		PortBindings: portMap,
		RestartPolicy: container.RestartPolicy{
			Name: "always",
		},
	}
}

// buildContainerConfig creates the container configuration.
func (d *Client) buildContainerConfig(app *database.App, imageName string) *container.Config {
	internalPort := nat.Port(fmt.Sprintf("%d/tcp", app.Port))
	return &container.Config{
		Image: imageName,
		ExposedPorts: nat.PortSet{
			internalPort: struct{}{},
		},
		Env: []string{
			fmt.Sprintf("PORT=%d", app.Port),
		},
		Labels: map[string]string{
			"diplo.app.id":   app.ID,
			"diplo.app.name": app.Name,
		},
	}
}
