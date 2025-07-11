package runtime

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/rodrwan/diplo/internal/docker"
	"github.com/sirupsen/logrus"
)

// DockerClient es un wrapper que adapta el cliente Docker existente a la interfaz ContainerRuntime
type DockerClient struct {
	client      *docker.Client
	runtimeType RuntimeType
}

// NewDockerClient crea una nueva instancia del cliente Docker
func NewDockerClient() (*DockerClient, error) {
	client, err := docker.NewClient()
	if err != nil {
		return nil, fmt.Errorf("error creando cliente Docker: %w", err)
	}

	return &DockerClient{
		client:      client,
		runtimeType: RuntimeTypeDocker,
	}, nil
}

// GetRuntimeType devuelve el tipo de runtime
func (d *DockerClient) GetRuntimeType() RuntimeType {
	return d.runtimeType
}

// GetRuntimeInfo devuelve información sobre el runtime Docker
func (d *DockerClient) GetRuntimeInfo() (*RuntimeInfo, error) {
	info := &RuntimeInfo{
		Type:         RuntimeTypeDocker,
		Version:      "integrated", // versión integrada del cliente existente
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Available:    true,
		Capabilities: []string{
			"build",
			"run",
			"logs",
			"networking",
			"volumes",
			"exec",
			"events",
		},
		Metadata: map[string]interface{}{
			"client_type": "docker_api",
			"backend":     "docker_daemon",
		},
	}

	return info, nil
}

// CreateContainer crea un nuevo contenedor (implementación básica)
func (d *DockerClient) CreateContainer(req *CreateContainerRequest) (*Container, error) {
	// Por ahora, devolver una respuesta simulada
	// En una implementación completa, esto usaría el cliente Docker existente
	container := &Container{
		ID:        generateContainerID(),
		Name:      req.Name,
		Image:     req.Image,
		Status:    ContainerStatusCreated,
		Runtime:   RuntimeTypeDocker,
		CreatedAt: time.Now(),
		Config: &ContainerConfig{
			Command:     req.Command,
			WorkingDir:  req.WorkingDir,
			Environment: req.Environment,
			Labels:      req.Labels,
		},
		Network: &NetworkConfig{
			Ports:       req.Ports,
			NetworkMode: req.NetworkMode,
		},
		Resources: req.Resources,
		Labels:    req.Labels,
		Metadata: map[string]interface{}{
			"created_by": "diplo_hybrid",
			"runtime":    "docker",
		},
	}

	logrus.Infof("Contenedor Docker creado (simulado): %s", container.ID)
	return container, nil
}

// StartContainer inicia un contenedor
func (d *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	logrus.Infof("Iniciando contenedor Docker: %s", containerID)
	// Implementación usando el cliente existente se haría aquí
	return nil
}

// StopContainer detiene un contenedor
func (d *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	logrus.Infof("Deteniendo contenedor Docker: %s", containerID)
	return d.client.StopContainer(containerID)
}

// RestartContainer reinicia un contenedor
func (d *DockerClient) RestartContainer(ctx context.Context, containerID string) error {
	logrus.Infof("Reiniciando contenedor Docker: %s", containerID)
	if err := d.StopContainer(ctx, containerID); err != nil {
		return err
	}
	return d.StartContainer(ctx, containerID)
}

// RemoveContainer elimina un contenedor
func (d *DockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	logrus.Infof("Eliminando contenedor Docker: %s", containerID)
	return d.client.StopContainer(containerID) // StopContainer ya hace remove en el cliente existente
}

// GetContainer obtiene información de un contenedor
func (d *DockerClient) GetContainer(containerID string) (*Container, error) {
	// Implementación simulada por ahora
	container := &Container{
		ID:        containerID,
		Name:      "docker-container",
		Image:     "unknown",
		Status:    ContainerStatusRunning,
		Runtime:   RuntimeTypeDocker,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"runtime": "docker",
		},
	}

	return container, nil
}

// ListContainers lista todos los contenedores
func (d *DockerClient) ListContainers(ctx context.Context) ([]*Container, error) {
	// Implementación simulada por ahora
	containers := []*Container{
		{
			ID:        "docker-container-1",
			Name:      "example-app",
			Image:     "diplo-app:latest",
			Status:    ContainerStatusRunning,
			Runtime:   RuntimeTypeDocker,
			CreatedAt: time.Now(),
		},
	}

	return containers, nil
}

// GetContainerLogs obtiene los logs de un contenedor
func (d *DockerClient) GetContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	logrus.Infof("Obteniendo logs del contenedor Docker: %s", containerID)
	return d.client.GetContainerLogsStream(containerID)
}

// ExecuteCommand ejecuta un comando en un contenedor
func (d *DockerClient) ExecuteCommand(ctx context.Context, containerID string, cmd []string) (*ExecResult, error) {
	logrus.Infof("Ejecutando comando en contenedor Docker %s: %v", containerID, cmd)

	// Implementación simulada por ahora
	result := &ExecResult{
		Output:   fmt.Sprintf("Comando ejecutado: %s", strings.Join(cmd, " ")),
		Error:    "",
		ExitCode: 0,
	}

	return result, nil
}

// GetContainerIP obtiene la IP de un contenedor
func (d *DockerClient) GetContainerIP(containerID string) (string, error) {
	logrus.Infof("Obteniendo IP del contenedor Docker: %s", containerID)

	// Implementación simulada por ahora
	return "172.17.0.2", nil
}

// SetEventCallback configura el callback para eventos
func (d *DockerClient) SetEventCallback(callback EventCallback) {
	logrus.Info("Configurando callback de eventos Docker")

	// Convertir el callback de Docker existente
	d.client.SetEventCallback(func(event docker.DockerEvent) {
		// Intentar extraer container_id del data si existe
		containerID := ""
		if event.Data != nil {
			if id, ok := event.Data["container_id"].(string); ok {
				containerID = id
			}
		}

		runtimeEvent := Event{
			Type:        event.Type,
			Message:     event.Message,
			Timestamp:   event.Time,
			ContainerID: containerID,
			Runtime:     RuntimeTypeDocker,
			Metadata: map[string]interface{}{
				"original_event": event,
				"data":           event.Data,
			},
		}

		callback(runtimeEvent)
	})
}

// Close cierra la conexión del cliente
func (d *DockerClient) Close() error {
	logrus.Info("Cerrando cliente Docker")
	return d.client.Close()
}

// Helper function para generar IDs de contenedor
func generateContainerID() string {
	return fmt.Sprintf("docker-%d", time.Now().Unix())
}
