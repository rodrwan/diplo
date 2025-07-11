package runtime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// LXCClient implementa la interfaz ContainerRuntime para LXC
type LXCClient struct {
	eventCallback EventCallback
	containers    map[string]*Container
	mu            sync.RWMutex
	runtimeType   RuntimeType
}

// NewLXCClient crea un nuevo cliente LXC
func NewLXCClient() (*LXCClient, error) {
	// Verificar que LXC esté instalado
	if err := checkLXCInstalled(); err != nil {
		return nil, fmt.Errorf("LXC no está disponible: %w", err)
	}

	client := &LXCClient{
		containers:  make(map[string]*Container),
		runtimeType: RuntimeTypeLXC,
	}

	// Inicializar containers existentes
	if err := client.refreshContainers(); err != nil {
		logrus.Warnf("Error cargando containers existentes: %v", err)
	}

	return client, nil
}

// checkLXCInstalled verifica si LXC está instalado y disponible
func checkLXCInstalled() error {
	if _, err := exec.LookPath("lxc-create"); err != nil {
		return fmt.Errorf("lxc-create no encontrado en PATH: %w", err)
	}
	if _, err := exec.LookPath("lxc-start"); err != nil {
		return fmt.Errorf("lxc-start no encontrado en PATH: %w", err)
	}
	if _, err := exec.LookPath("lxc-stop"); err != nil {
		return fmt.Errorf("lxc-stop no encontrado en PATH: %w", err)
	}
	return nil
}

// GetRuntimeType devuelve el tipo de runtime
func (c *LXCClient) GetRuntimeType() RuntimeType {
	return c.runtimeType
}

// GetRuntimeInfo devuelve información sobre el runtime LXC
func (c *LXCClient) GetRuntimeInfo() (*RuntimeInfo, error) {
	info := &RuntimeInfo{
		Type:         RuntimeTypeLXC,
		Version:      "native",
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Available:    true,
		Capabilities: []string{
			"create",
			"start",
			"stop",
			"destroy",
			"exec",
			"logs",
			"networking",
		},
		Metadata: map[string]interface{}{
			"client_type": "lxc_native",
			"backend":     "lxc_commands",
		},
	}

	return info, nil
}

// CreateContainer crea un nuevo contenedor LXC
func (c *LXCClient) CreateContainer(req *CreateContainerRequest) (*Container, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Creando container LXC: %s", req.Name)
	c.sendEvent("container_create_start", "Iniciando creación de container", req.Name, map[string]interface{}{
		"image": req.Image,
	})

	// Determinar template y distro basado en la imagen
	template, distro, release := c.parseImage(req.Image)

	// Crear el container
	cmd := exec.Command("lxc-create", "-n", req.Name, "-t", template)
	if distro != "" {
		cmd.Args = append(cmd.Args, "--", "--dist", distro)
	}
	if release != "" {
		cmd.Args = append(cmd.Args, "--release", release)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		errorMsg := fmt.Sprintf("Error creando container: %v, output: %s", err, string(output))
		logrus.Error(errorMsg)
		c.sendEvent("container_create_error", errorMsg, req.Name, map[string]interface{}{
			"error":  err.Error(),
			"output": string(output),
		})
		return nil, fmt.Errorf("error creando container: %w", err)
	}

	// Crear objeto Container
	container := &Container{
		ID:        req.Name,
		Name:      req.Name,
		Image:     req.Image,
		Status:    ContainerStatusCreated,
		Runtime:   RuntimeTypeLXC,
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
			"created_by": "diplo_lxc",
			"runtime":    "lxc",
		},
	}

	// Configurar el container
	if err := c.configureContainer(container, req); err != nil {
		logrus.Errorf("Error configurando container: %v", err)
		// Intentar limpiar el container creado
		c.removeContainer(req.Name)
		return nil, fmt.Errorf("error configurando container: %w", err)
	}

	// Actualizar información del container
	c.containers[req.Name] = container

	c.sendEvent("container_create_success", "Container creado exitosamente", req.Name, map[string]interface{}{
		"output": string(output),
	})

	logrus.Infof("Container LXC creado exitosamente: %s", req.Name)
	return container, nil
}

// StartContainer inicia un contenedor LXC
func (c *LXCClient) StartContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Iniciando container LXC: %s", containerID)
	c.sendEvent("container_start", "Iniciando container", containerID, nil)

	cmd := exec.Command("lxc-start", "-n", containerID, "-d")
	if err := cmd.Run(); err != nil {
		errorMsg := fmt.Sprintf("Error iniciando container: %v", err)
		logrus.Error(errorMsg)
		c.sendEvent("container_start_error", errorMsg, containerID, map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("error iniciando container: %w", err)
	}

	// Esperar a que el container esté completamente iniciado
	if err := c.waitForContainerReady(containerID, 30*time.Second); err != nil {
		errorMsg := fmt.Sprintf("Container no se inició completamente: %v", err)
		logrus.Error(errorMsg)
		c.sendEvent("container_start_timeout", errorMsg, containerID, map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("timeout esperando que el container esté listo: %w", err)
	}

	// Actualizar estado del container
	if container, exists := c.containers[containerID]; exists {
		container.Status = ContainerStatusRunning
		now := time.Now()
		container.StartedAt = &now
	}

	c.sendEvent("container_start_success", "Container iniciado exitosamente", containerID, nil)
	logrus.Infof("Container LXC iniciado exitosamente: %s", containerID)
	return nil
}

// StopContainer detiene un contenedor LXC
func (c *LXCClient) StopContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Deteniendo container LXC: %s", containerID)
	c.sendEvent("container_stop", "Deteniendo container", containerID, nil)

	cmd := exec.Command("lxc-stop", "-n", containerID)
	if err := cmd.Run(); err != nil {
		errorMsg := fmt.Sprintf("Error deteniendo container: %v", err)
		logrus.Error(errorMsg)
		c.sendEvent("container_stop_error", errorMsg, containerID, map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("error deteniendo container: %w", err)
	}

	// Actualizar estado del container
	if container, exists := c.containers[containerID]; exists {
		container.Status = ContainerStatusStopped
		now := time.Now()
		container.StoppedAt = &now
	}

	c.sendEvent("container_stop_success", "Container detenido exitosamente", containerID, nil)
	logrus.Infof("Container LXC detenido exitosamente: %s", containerID)
	return nil
}

// RestartContainer reinicia un contenedor LXC
func (c *LXCClient) RestartContainer(ctx context.Context, containerID string) error {
	if err := c.StopContainer(ctx, containerID); err != nil {
		return fmt.Errorf("error deteniendo container: %w", err)
	}
	return c.StartContainer(ctx, containerID)
}

// RemoveContainer elimina un contenedor LXC
func (c *LXCClient) RemoveContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Eliminando container LXC: %s", containerID)
	c.sendEvent("container_remove", "Eliminando container", containerID, nil)

	// Primero intentar detener el container si está corriendo
	if container, exists := c.containers[containerID]; exists {
		if container.Status == ContainerStatusRunning {
			if err := c.StopContainer(ctx, containerID); err != nil {
				logrus.Warnf("Error deteniendo container antes de eliminarlo: %v", err)
			}
		}
	}

	if err := c.removeContainer(containerID); err != nil {
		return err
	}

	// Remover de la lista local
	delete(c.containers, containerID)

	c.sendEvent("container_remove_success", "Container eliminado exitosamente", containerID, nil)
	logrus.Infof("Container LXC eliminado exitosamente: %s", containerID)
	return nil
}

// GetContainer obtiene información de un contenedor
func (c *LXCClient) GetContainer(containerID string) (*Container, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if container, exists := c.containers[containerID]; exists {
		return container, nil
	}

	return nil, fmt.Errorf("container %s no encontrado", containerID)
}

// ListContainers lista todos los contenedores LXC
func (c *LXCClient) ListContainers(ctx context.Context) ([]*Container, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Actualizar estado de contenedores
	if err := c.refreshContainers(); err != nil {
		logrus.Warnf("Error actualizando lista de containers: %v", err)
	}

	var containers []*Container
	for _, container := range c.containers {
		containers = append(containers, container)
	}

	return containers, nil
}

// GetContainerLogs obtiene los logs de un contenedor LXC
func (c *LXCClient) GetContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	logPath := fmt.Sprintf("/var/lib/lxc/%s/console.log", containerID)

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		// Si no existe el archivo de logs, crear un stream vacío
		return io.NopCloser(strings.NewReader("")), nil
	}

	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("error abriendo archivo de logs: %w", err)
	}

	return file, nil
}

// ExecuteCommand ejecuta un comando dentro de un contenedor LXC
func (c *LXCClient) ExecuteCommand(ctx context.Context, containerID string, cmd []string) (*ExecResult, error) {
	logrus.Infof("Ejecutando comando en container %s: %v", containerID, cmd)

	c.sendEvent("container_exec", "Ejecutando comando", containerID, map[string]interface{}{
		"command": cmd,
	})

	// Construir el comando lxc-attach
	args := []string{"-n", containerID, "--"}
	args = append(args, cmd...)

	command := exec.CommandContext(ctx, "lxc-attach", args...)
	output, err := command.CombinedOutput()

	result := &ExecResult{
		Output: string(output),
	}

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1

		errorMsg := fmt.Sprintf("Error ejecutando comando: %v", err)
		logrus.Error(errorMsg)
		c.sendEvent("container_exec_error", errorMsg, containerID, map[string]interface{}{
			"error":   err.Error(),
			"output":  string(output),
			"command": cmd,
		})
		return result, nil // No devolver error, sino resultado con error
	}

	result.ExitCode = 0
	c.sendEvent("container_exec_success", "Comando ejecutado exitosamente", containerID, map[string]interface{}{
		"output":  string(output),
		"command": cmd,
	})

	return result, nil
}

// GetContainerIP obtiene la IP de un contenedor LXC
func (c *LXCClient) GetContainerIP(containerID string) (string, error) {
	cmd := exec.Command("lxc-info", "-n", containerID, "-iH")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error obteniendo IP del container: %w", err)
	}

	ip := strings.TrimSpace(string(output))
	if ip == "" {
		return "", fmt.Errorf("IP no disponible para el container %s", containerID)
	}

	return ip, nil
}

// SetEventCallback configura el callback para eventos
func (c *LXCClient) SetEventCallback(callback EventCallback) {
	c.eventCallback = callback
}

// Close cierra el cliente LXC
func (c *LXCClient) Close() error {
	// Cleanup si es necesario
	return nil
}

// Helper methods

// sendEvent envía un evento LXC si hay un callback configurado
func (c *LXCClient) sendEvent(eventType, message, containerID string, metadata map[string]interface{}) {
	if c.eventCallback != nil {
		event := Event{
			Type:        eventType,
			Message:     message,
			Timestamp:   time.Now(),
			ContainerID: containerID,
			Runtime:     RuntimeTypeLXC,
			Metadata:    metadata,
		}
		c.eventCallback(event)
	}
}

// parseImage convierte una imagen Docker-style a template LXC
func (c *LXCClient) parseImage(image string) (template, distro, release string) {
	// Convertir imágenes comunes a templates LXC
	switch {
	case strings.Contains(image, "ubuntu"):
		return "ubuntu", "ubuntu", "focal"
	case strings.Contains(image, "debian"):
		return "debian", "debian", "bullseye"
	case strings.Contains(image, "alpine"):
		return "alpine", "alpine", "3.18"
	case strings.Contains(image, "golang"):
		return "ubuntu", "ubuntu", "focal" // Usaremos Ubuntu para Go
	case strings.Contains(image, "node"):
		return "ubuntu", "ubuntu", "focal" // Usaremos Ubuntu para Node.js
	case strings.Contains(image, "python"):
		return "ubuntu", "ubuntu", "focal" // Usaremos Ubuntu para Python
	default:
		return "ubuntu", "ubuntu", "focal" // Default a Ubuntu
	}
}

// configureContainer configura un container LXC después de la creación
func (c *LXCClient) configureContainer(container *Container, req *CreateContainerRequest) error {
	// Configurar variables de entorno
	if len(req.Environment) > 0 {
		configPath := fmt.Sprintf("/var/lib/lxc/%s/config", container.Name)

		// Leer configuración existente
		config, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("error leyendo configuración: %w", err)
		}

		// Agregar variables de entorno
		envConfig := "\n# Environment variables\n"
		for key, value := range req.Environment {
			envConfig += fmt.Sprintf("lxc.environment.%s = %s\n", key, value)
		}

		// Escribir configuración actualizada
		newConfig := string(config) + envConfig
		if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
			return fmt.Errorf("error escribiendo configuración: %w", err)
		}
	}

	return nil
}

// removeContainer elimina físicamente un container LXC
func (c *LXCClient) removeContainer(containerID string) error {
	cmd := exec.Command("lxc-destroy", "-n", containerID)
	if err := cmd.Run(); err != nil {
		errorMsg := fmt.Sprintf("Error eliminando container: %v", err)
		logrus.Error(errorMsg)
		c.sendEvent("container_remove_error", errorMsg, containerID, map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("error eliminando container: %w", err)
	}
	return nil
}

// waitForContainerReady espera hasta que un container esté completamente listo
func (c *LXCClient) waitForContainerReady(containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("lxc-info", "-n", containerID, "-s")
		output, err := cmd.Output()
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if strings.Contains(string(output), "RUNNING") {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout esperando que el container esté listo")
}

// refreshContainers actualiza la lista de containers desde el sistema
func (c *LXCClient) refreshContainers() error {
	cmd := exec.Command("lxc-ls", "-f")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error listando containers: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))

	// Saltar la primera línea (header)
	if scanner.Scan() {
		// Skip header line
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		name := fields[0]
		state := fields[1]

		// Actualizar o crear container
		if container, exists := c.containers[name]; exists {
			container.Status = c.mapLXCStateToContainerStatus(state)
		} else {
			container := &Container{
				ID:        name,
				Name:      name,
				Image:     "unknown",
				Status:    c.mapLXCStateToContainerStatus(state),
				Runtime:   RuntimeTypeLXC,
				CreatedAt: time.Now(),
				Config: &ContainerConfig{
					Environment: make(map[string]string),
					Labels:      make(map[string]string),
				},
				Network: &NetworkConfig{
					Ports: make([]PortMapping, 0),
				},
				Labels: make(map[string]string),
				Metadata: map[string]interface{}{
					"runtime": "lxc",
				},
			}

			// Obtener IP si está disponible
			if len(fields) > 2 {
				container.Network.IPAddress = fields[2]
			}

			c.containers[name] = container
		}
	}

	return nil
}

// mapLXCStateToContainerStatus mapea estados LXC a estados de Container
func (c *LXCClient) mapLXCStateToContainerStatus(lxcState string) ContainerStatus {
	switch strings.ToUpper(lxcState) {
	case "RUNNING":
		return ContainerStatusRunning
	case "STOPPED":
		return ContainerStatusStopped
	case "FROZEN":
		return ContainerStatusPaused
	default:
		return ContainerStatusStopped
	}
}
