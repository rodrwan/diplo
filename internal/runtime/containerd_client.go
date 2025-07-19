package runtime

import (
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

// ContainerdClient implementa ContainerRuntime usando containerd
type ContainerdClient struct {
	socketPath    string
	namespace     string
	logger        *logrus.Logger
	eventCallback EventCallback
	containers    map[string]*Container
	mu            sync.RWMutex
	runtimeType   RuntimeType
}

// NewContainerdClient crea una nueva instancia del cliente containerd
func NewContainerdClient(socketPath, namespace string) (*ContainerdClient, error) {
	if socketPath == "" {
		socketPath = "/run/containerd/containerd.sock"
	}
	if namespace == "" {
		namespace = "diplo"
	}

	client := &ContainerdClient{
		socketPath:  socketPath,
		namespace:   namespace,
		logger:      logrus.New(),
		containers:  make(map[string]*Container),
		runtimeType: RuntimeTypeContainerd,
	}

	// Verificar que containerd esté disponible
	if err := client.checkContainerdInstalled(); err != nil {
		return nil, fmt.Errorf("containerd no está disponible: %w", err)
	}

	return client, nil
}

// checkContainerdInstalled verifica si containerd está instalado y disponible
func (c *ContainerdClient) checkContainerdInstalled() error {
	// Verificar que ctr (containerd CLI) esté disponible
	if _, err := exec.LookPath("ctr"); err != nil {
		return fmt.Errorf("ctr (containerd CLI) no encontrado en PATH: %w", err)
	}

	// Verificar que el socket de containerd esté disponible
	if _, err := os.Stat(c.socketPath); os.IsNotExist(err) {
		return fmt.Errorf("socket de containerd no encontrado en %s: %w", c.socketPath, err)
	}

	// Verificar que containerd esté corriendo
	cmd := exec.Command("ctr", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("containerd no está corriendo: %w", err)
	}

	return nil
}

// GetRuntimeType devuelve el tipo de runtime
func (c *ContainerdClient) GetRuntimeType() RuntimeType {
	return c.runtimeType
}

// GetRuntimeInfo devuelve información sobre el runtime
func (c *ContainerdClient) GetRuntimeInfo() (*RuntimeInfo, error) {
	info := &RuntimeInfo{
		Type:         RuntimeTypeContainerd,
		Version:      "1.7.0", // Placeholder
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Available:    c.IsAvailable(),
		Capabilities: []string{
			"image-pull",
			"container-create",
			"container-start",
			"container-stop",
			"container-exec",
			"networking",
		},
		Metadata: map[string]interface{}{
			"socket_path": c.socketPath,
			"namespace":   c.namespace,
		},
	}

	return info, nil
}

// CreateContainer crea un nuevo contenedor usando containerd
func (c *ContainerdClient) CreateContainer(req *CreateContainerRequest) (*Container, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Creando container containerd: %s", req.Name)
	c.sendEvent("container_create_start", "Iniciando creación de container", req.Name, map[string]interface{}{
		"image": req.Image,
	})

	// Determinar imagen base según el lenguaje
	baseImage := c.getContainerdBaseImage(req.Image)

	// Crear el contenedor usando ctr
	containerID := fmt.Sprintf("diplo-%s", req.Name)

	// Pull de la imagen si es necesario
	pullCmd := exec.Command("ctr", "-n", c.namespace, "images", "pull", baseImage)
	if err := pullCmd.Run(); err != nil {
		logrus.Warnf("Error haciendo pull de imagen %s: %v", baseImage, err)
		// Continuar sin hacer pull, puede que la imagen ya exista
	}

	// Crear el contenedor
	createCmd := exec.Command("ctr", "-n", c.namespace, "run", "-d",
		"--net-host", // Usar red del host por simplicidad
		baseImage, containerID)

	output, err := createCmd.CombinedOutput()
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
		ID:        containerID,
		Name:      req.Name,
		Image:     baseImage,
		Status:    ContainerStatusCreated,
		Runtime:   RuntimeTypeContainerd,
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
			"created_by": "diplo_containerd",
			"runtime":    "containerd",
		},
	}

	// Actualizar información del container
	c.containers[req.Name] = container

	c.sendEvent("container_create_success", "Container creado exitosamente", req.Name, map[string]interface{}{
		"output": string(output),
	})

	logrus.Infof("Container containerd creado exitosamente: %s", req.Name)
	return container, nil
}

// GetRunningContainers returns a list of all running containers
func (c *ContainerdClient) GetRunningContainers() ([]*Container, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Actualizar estado de contenedores
	if err := c.refreshContainers(); err != nil {
		return nil, fmt.Errorf("error actualizando lista de containers: %w", err)
	}

	var runningContainers []*Container
	for _, container := range c.containers {
		if container.Status == ContainerStatusRunning {
			runningContainers = append(runningContainers, container)
		}
	}

	return runningContainers, nil
}

// GetContainerStatus returns the status of a container
func (c *ContainerdClient) GetContainerStatus(containerID string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Verificar estado real del contenedor
	cmd := exec.Command("ctr", "-n", c.namespace, "tasks", "list")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error verificando estado del contenedor %s: %w", containerID, err)
	}

	// Verificar si el contenedor está corriendo
	isRunning := strings.Contains(string(output), containerID)
	if isRunning {
		return "running", nil
	}

	// Verificar si el contenedor existe pero no está corriendo
	containersCmd := exec.Command("ctr", "-n", c.namespace, "containers", "list")
	containersOutput, err := containersCmd.Output()
	if err != nil {
		return "unknown", fmt.Errorf("error verificando existencia del contenedor %s: %w", containerID, err)
	}

	if strings.Contains(string(containersOutput), containerID) {
		return "stopped", nil
	}

	return "not_found", nil
}

// StartContainer inicia un contenedor containerd
func (c *ContainerdClient) StartContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Iniciando container containerd: %s", containerID)
	c.sendEvent("container_start", "Iniciando container", containerID, nil)

	// Verificar que el contenedor existe
	cmd := exec.Command("ctr", "-n", c.namespace, "tasks", "list")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error verificando contenedor: %w", err)
	}

	// Si el contenedor ya está corriendo, no hacer nada
	if strings.Contains(string(output), containerID) {
		logrus.Infof("Container %s ya está corriendo", containerID)
		return nil
	}

	// Iniciar el contenedor
	startCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "start", containerID)
	if err := startCmd.Run(); err != nil {
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
	logrus.Infof("Container containerd iniciado exitosamente: %s", containerID)
	return nil
}

// StopContainer detiene un contenedor containerd
func (c *ContainerdClient) StopContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Deteniendo container containerd: %s", containerID)
	c.sendEvent("container_stop", "Deteniendo container", containerID, nil)

	cmd := exec.Command("ctr", "-n", c.namespace, "tasks", "kill", containerID)
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
	logrus.Infof("Container containerd detenido exitosamente: %s", containerID)
	return nil
}

// RestartContainer reinicia un contenedor containerd
func (c *ContainerdClient) RestartContainer(ctx context.Context, containerID string) error {
	if err := c.StopContainer(ctx, containerID); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	if err := c.StartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// RemoveContainer elimina un contenedor containerd
func (c *ContainerdClient) RemoveContainer(ctx context.Context, containerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logrus.Infof("Eliminando container containerd: %s", containerID)
	c.sendEvent("container_remove", "Eliminando container", containerID, nil)

	// Estrategia 1: Detener el contenedor con SIGKILL (más agresivo)
	logrus.Debugf("Deteniendo container containerd con SIGKILL: %s", containerID)
	stopCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "kill", "--signal", "SIGKILL", containerID)
	if stopErr := stopCmd.Run(); stopErr != nil {
		logrus.Debugf("Container %s ya estaba detenido o no existe: %v", containerID, stopErr)
	}

	// Esperar más tiempo para que el contenedor se detenga completamente
	time.Sleep(2 * time.Second)

	// Estrategia 2: Intentar eliminar el contenedor directamente
	logrus.Debugf("Intentando eliminar container containerd: %s", containerID)
	cmd := exec.Command("ctr", "-n", c.namespace, "containers", "delete", containerID)
	if err := cmd.Run(); err == nil {
		// Éxito - contenedor eliminado
		delete(c.containers, containerID)
		c.sendEvent("container_remove_success", "Container eliminado exitosamente", containerID, nil)
		logrus.Infof("Container containerd eliminado exitosamente: %s", containerID)
		return nil
	}

	// Estrategia 3: Eliminar tarea con SIGKILL
	logrus.Debugf("Eliminando tarea del container containerd con SIGKILL: %s", containerID)
	taskKillCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "kill", "--signal", "SIGKILL", containerID)
	taskKillCmd.Run() // Ignorar errores

	// Esperar un momento
	time.Sleep(1 * time.Second)

	// Estrategia 4: Eliminar tarea
	logrus.Debugf("Eliminando tarea del container containerd: %s", containerID)
	taskCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "delete", containerID)
	if taskErr := taskCmd.Run(); taskErr != nil {
		logrus.Debugf("No se pudo eliminar tarea del container %s: %v", containerID, taskErr)
	}

	// Esperar un momento para que la tarea se elimine
	time.Sleep(1 * time.Second)

	// Estrategia 5: Intentar eliminar el contenedor nuevamente
	logrus.Debugf("Reintentando eliminación del container containerd: %s", containerID)
	cmd2 := exec.Command("ctr", "-n", c.namespace, "containers", "delete", containerID)
	if err := cmd2.Run(); err == nil {
		// Éxito - contenedor eliminado
		delete(c.containers, containerID)
		c.sendEvent("container_remove_success", "Container eliminado exitosamente", containerID, nil)
		logrus.Infof("Container containerd eliminado exitosamente: %s", containerID)
		return nil
	}

	// Estrategia 6: Verificar si el contenedor realmente existe
	logrus.Debugf("Verificando si el container containerd existe: %s", containerID)
	checkCmd := exec.Command("ctr", "-n", c.namespace, "containers", "list")
	output, checkErr := checkCmd.Output()
	if checkErr != nil {
		logrus.Debugf("Error verificando containers: %v", checkErr)
	} else {
		// Si el contenedor no aparece en la lista, considerarlo como eliminado
		if !strings.Contains(string(output), containerID) {
			logrus.Infof("Container %s no encontrado en la lista - considerado como eliminado", containerID)
			delete(c.containers, containerID)
			c.sendEvent("container_remove_success", "Container no encontrado - considerado eliminado", containerID, nil)
			return nil
		}
	}

	// Estrategia 7: Último intento con force (si está disponible)
	logrus.Debugf("Último intento de eliminación forzada del container containerd: %s", containerID)
	forceCmd := exec.Command("ctr", "-n", c.namespace, "containers", "delete", "--force", containerID)
	if err := forceCmd.Run(); err == nil {
		// Éxito - contenedor eliminado
		delete(c.containers, containerID)
		c.sendEvent("container_remove_success", "Container eliminado exitosamente con force", containerID, nil)
		logrus.Infof("Container containerd eliminado exitosamente con force: %s", containerID)
		return nil
	}

	// Estrategia 8: Eliminación agresiva (similar al script container_prune.sh)
	logrus.Debugf("Aplicando eliminación agresiva para container containerd: %s", containerID)
	if err := c.aggressiveContainerRemoval(containerID); err == nil {
		// Éxito - contenedor eliminado
		delete(c.containers, containerID)
		c.sendEvent("container_remove_success", "Container eliminado exitosamente con eliminación agresiva", containerID, nil)
		logrus.Infof("Container containerd eliminado exitosamente con eliminación agresiva: %s", containerID)
		return nil
	}

	// Si llegamos aquí, no se pudo eliminar el contenedor
	errorMsg := fmt.Sprintf("Error eliminando container: exit status 1")
	logrus.Error(errorMsg)
	c.sendEvent("container_remove_error", errorMsg, containerID, map[string]interface{}{
		"error": "No se pudo eliminar el contenedor después de múltiples intentos",
	})
	return fmt.Errorf("error eliminando container: %w", fmt.Errorf("exit status 1"))
}

// aggressiveContainerRemoval aplica una eliminación más agresiva similar al script container_prune.sh
func (c *ContainerdClient) aggressiveContainerRemoval(containerID string) error {
	logrus.Debugf("Aplicando eliminación agresiva para: %s", containerID)

	// 1. Matar procesos containerd-shim relacionados
	logrus.Debugf("Matando procesos containerd-shim relacionados...")
	shimCmd := exec.Command("pkill", "-f", "containerd-shim")
	shimCmd.Run() // Ignorar errores

	// 2. Esperar a que se detengan completamente
	time.Sleep(3 * time.Second)

	// 3. Intentar eliminar tarea con SIGKILL nuevamente
	killCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "kill", "--signal", "SIGKILL", containerID)
	killCmd.Run() // Ignorar errores

	time.Sleep(2 * time.Second)

	// 4. Eliminar tarea
	deleteTaskCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "delete", containerID)
	deleteTaskCmd.Run() // Ignorar errores

	time.Sleep(1 * time.Second)

	// 5. Eliminar contenedor
	deleteContainerCmd := exec.Command("ctr", "-n", c.namespace, "containers", "delete", containerID)
	if err := deleteContainerCmd.Run(); err == nil {
		return nil
	}

	// 6. Verificar si el contenedor ya no existe
	checkCmd := exec.Command("ctr", "-n", c.namespace, "containers", "list")
	output, err := checkCmd.Output()
	if err == nil && !strings.Contains(string(output), containerID) {
		return nil // Contenedor ya no existe
	}

	return fmt.Errorf("eliminación agresiva falló")
}

// GetContainer obtiene información de un contenedor específico
func (c *ContainerdClient) GetContainer(containerID string) (*Container, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Verificar estado real del contenedor
	cmd := exec.Command("ctr", "-n", c.namespace, "tasks", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("container %s no existe o no es accesible: %w", containerID, err)
	}

	// Verificar si el contenedor está corriendo
	isRunning := strings.Contains(string(output), containerID)
	containerStatus := ContainerStatusStopped
	if isRunning {
		containerStatus = ContainerStatusRunning
	}

	// Actualizar estado interno si existe
	if container, exists := c.containers[containerID]; exists {
		container.Status = containerStatus
		return container, nil
	}

	// Si no existe en la lista interna, crear un objeto temporal
	container := &Container{
		ID:        containerID,
		Name:      containerID,
		Image:     "unknown",
		Status:    containerStatus,
		Runtime:   RuntimeTypeContainerd,
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
			"runtime": "containerd",
		},
	}

	return container, nil
}

// ListContainers lista todos los contenedores containerd
func (c *ContainerdClient) ListContainers(ctx context.Context) ([]*Container, error) {
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

// GetContainerLogs obtiene los logs de un contenedor
func (c *ContainerdClient) GetContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	// containerd no tiene un sistema de logs integrado como Docker
	// Podemos usar journalctl para obtener logs del contenedor
	cmd := exec.Command("journalctl", "-u", fmt.Sprintf("containerd-%s", containerID), "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return io.NopCloser(strings.NewReader("")), nil
	}

	return io.NopCloser(strings.NewReader(string(output))), nil
}

// ExecuteCommand ejecuta un comando en un contenedor containerd
func (c *ContainerdClient) ExecuteCommand(ctx context.Context, containerID string, cmd []string) (*ExecResult, error) {
	logrus.Infof("Ejecutando comando en container %s: %v", containerID, cmd)

	c.sendEvent("container_exec", "Ejecutando comando", containerID, map[string]interface{}{
		"command": cmd,
	})

	// Construir el comando ctr exec
	args := []string{"-n", c.namespace, "tasks", "exec", "--exec-id", "diplo-exec", containerID}
	args = append(args, cmd...)

	command := exec.CommandContext(ctx, "ctr", args...)
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

// GetContainerIP obtiene la IP de un contenedor containerd
func (c *ContainerdClient) GetContainerIP(containerID string) (string, error) {
	// Para containerd, como estamos usando --net-host, la IP es localhost
	// En una implementación más completa, podríamos usar CNI para obtener la IP real
	return "127.0.0.1", nil
}

// SetEventCallback configura el callback de eventos
func (c *ContainerdClient) SetEventCallback(callback EventCallback) {
	c.eventCallback = callback
}

// IsAvailable verifica si containerd está disponible
func (c *ContainerdClient) IsAvailable() bool {
	return c.checkContainerdInstalled() == nil
}

// Close cierra la conexión con containerd
func (c *ContainerdClient) Close() error {
	c.logger.Debug("Closing containerd client")
	return nil
}

// Helper functions

func (c *ContainerdClient) getContainerdBaseImage(image string) string {
	// Mapear imágenes de Diplo a imágenes de containerd
	switch image {
	case "ubuntu:22.04":
		return "docker.io/library/ubuntu:22.04"
	case "alpine:latest":
		return "docker.io/library/alpine:latest"
	default:
		// Si ya es una imagen completa, usarla tal como está
		if strings.Contains(image, "/") {
			return image
		}
		// Agregar prefijo docker.io/library/ si es necesario
		return fmt.Sprintf("docker.io/library/%s", image)
	}
}

func (c *ContainerdClient) waitForContainerReady(containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempts := 0
	maxAttempts := int(timeout.Seconds())

	for time.Now().Before(deadline) {
		attempts++

		cmd := exec.Command("ctr", "-n", c.namespace, "tasks", "list")
		output, err := cmd.Output()
		if err != nil {
			logrus.Debugf("Intento %d/%d: Error obteniendo estado del contenedor %s: %v", attempts, maxAttempts, containerID, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if strings.Contains(string(output), containerID) {
			logrus.Infof("Contenedor %s está corriendo después de %d intentos", containerID, attempts)
			return nil
		}

		logrus.Infof("Contenedor no está corriendo aún, esperando... (intento %d/%d)", attempts, maxAttempts)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout esperando que el container %s esté listo después de %d intentos", containerID, attempts)
}

func (c *ContainerdClient) refreshContainers() error {
	cmd := exec.Command("ctr", "-n", c.namespace, "containers", "list")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error listando containers: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		name := fields[0]
		image := fields[1]

		// Verificar si está corriendo
		taskCmd := exec.Command("ctr", "-n", c.namespace, "tasks", "list")
		taskOutput, _ := taskCmd.Output()
		isRunning := strings.Contains(string(taskOutput), name)

		status := ContainerStatusStopped
		if isRunning {
			status = ContainerStatusRunning
		}

		// Actualizar o crear container
		if container, exists := c.containers[name]; exists {
			container.Status = status
		} else {
			container := &Container{
				ID:        name,
				Name:      name,
				Image:     image,
				Status:    status,
				Runtime:   RuntimeTypeContainerd,
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
					"runtime": "containerd",
				},
			}

			c.containers[name] = container
		}
	}

	return nil
}

func (c *ContainerdClient) sendEvent(eventType, message, containerID string, metadata map[string]interface{}) {
	if c.eventCallback != nil {
		event := Event{
			Type:        eventType,
			Message:     message,
			ContainerID: containerID,
			Runtime:     c.runtimeType,
			Metadata:    metadata,
			Timestamp:   time.Now(),
		}
		c.eventCallback(event)
	}
}
