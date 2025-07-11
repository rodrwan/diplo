package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/models"
	"github.com/sirupsen/logrus"
)

// RunContainer creates and starts a Docker container for a given application.
func (d *Client) RunContainer(app *database.App, imageName string, envVars []models.EnvVar) (string, error) {
	logrus.Infof("Running container for app %s from image %s on port %d", app.Name, imageName, app.Port)
	d.sendDockerEvent("container_start", "Starting container", map[string]interface{}{
		"image_name":     imageName,
		"port":           app.Port,
		"env_vars_count": len(envVars),
	})

	hostConfig := d.buildHostConfig(app)
	containerConfig := d.buildContainerConfig(app, imageName, envVars)

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
func (d *Client) buildContainerConfig(app *database.App, imageName string, envVars []models.EnvVar) *container.Config {
	internalPort := nat.Port(fmt.Sprintf("%d/tcp", app.Port))

	// Start with default environment variables
	env := []string{
		fmt.Sprintf("PORT=%d", app.Port),
		fmt.Sprintf("DIPLO_APP_ID=%s", app.ID),
		fmt.Sprintf("DIPLO_APP_NAME=%s", app.Name),
	}

	// Add user-defined environment variables
	for _, envVar := range envVars {
		// Validate environment variable name (basic security)
		if isValidEnvVarName(envVar.Name) {
			env = append(env, fmt.Sprintf("%s=%s", envVar.Name, envVar.Value))
		} else {
			logrus.Warnf("Skipping invalid environment variable name: %s", envVar.Name)
		}
	}

	return &container.Config{
		Image: imageName,
		ExposedPorts: nat.PortSet{
			internalPort: struct{}{},
		},
		Env: env,
		Labels: map[string]string{
			// Etiquetas de identificación
			"diplo.app.id":       app.ID,
			"diplo.app.name":     app.Name,
			"diplo.app.repo_url": app.RepoUrl,
			"diplo.app.language": app.Language.String,
			"diplo.app.port":     fmt.Sprintf("%d", app.Port),

			// Etiquetas de seguridad y aislamiento
			"diplo.tenant":            app.ID, // Para aislamiento de tenant
			"diplo.security.isolated": "true",
			"diplo.network.port":      fmt.Sprintf("%d", app.Port),

			// Etiquetas de gestión
			"diplo.managed":    "true",
			"diplo.created_by": "diplo-server",
			"diplo.version":    "1.0.0",

			// Etiquetas para filtering y limpieza
			"diplo.cleanup.enabled":    "true",
			"diplo.monitoring.enabled": "true",
		},
	}
}

// isValidEnvVarName validates environment variable names to prevent security issues
func isValidEnvVarName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// Prevent system environment variables from being overridden
	systemVars := []string{"PATH", "HOME", "USER", "SHELL", "TERM", "PWD", "LANG", "LC_ALL"}
	for _, sysVar := range systemVars {
		if name == sysVar {
			return false
		}
	}

	// Prevent Docker-specific variables
	if name == "HOSTNAME" || name == "DOCKER_HOST" {
		return false
	}

	// Prevent Diplo internal variables from being overridden
	if name == "DIPLO_APP_ID" || name == "DIPLO_APP_NAME" {
		return false
	}

	// Basic character validation (alphanumeric and underscore only)
	for _, char := range name {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_') {
			return false
		}
	}

	return true
}
