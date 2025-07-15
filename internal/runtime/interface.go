package runtime

import (
	"context"
	"io"
	"time"
)

// RuntimeType representa el tipo de runtime de contenedores
type RuntimeType string

const (
	RuntimeTypeContainerd RuntimeType = "containerd"
	RuntimeTypeDocker     RuntimeType = "docker"
)

// ContainerRuntime define la interfaz común para diferentes runtimes de contenedores
type ContainerRuntime interface {
	// Información del runtime
	GetRuntimeType() RuntimeType
	GetRuntimeInfo() (*RuntimeInfo, error)

	// Gestión de contenedores
	CreateContainer(req *CreateContainerRequest) (*Container, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	RestartContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string) error

	// Información de contenedores
	GetContainer(containerID string) (*Container, error)
	ListContainers(ctx context.Context) ([]*Container, error)
	GetContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error)

	// Ejecución de comandos
	ExecuteCommand(ctx context.Context, containerID string, cmd []string) (*ExecResult, error)

	// Gestión de red
	GetContainerIP(containerID string) (string, error)

	// Eventos
	SetEventCallback(callback EventCallback)

	// Limpieza
	Close() error
}

// RuntimeInfo contiene información sobre el runtime
type RuntimeInfo struct {
	Type         RuntimeType            `json:"type"`
	Version      string                 `json:"version"`
	OS           string                 `json:"os"`
	Architecture string                 `json:"architecture"`
	Available    bool                   `json:"available"`
	Capabilities []string               `json:"capabilities"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// Container representa un contenedor en cualquier runtime
type Container struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Image     string                 `json:"image"`
	Status    ContainerStatus        `json:"status"`
	Runtime   RuntimeType            `json:"runtime"`
	CreatedAt time.Time              `json:"created_at"`
	StartedAt *time.Time             `json:"started_at,omitempty"`
	StoppedAt *time.Time             `json:"stopped_at,omitempty"`
	Config    *ContainerConfig       `json:"config"`
	Network   *NetworkConfig         `json:"network"`
	Resources *ResourceConfig        `json:"resources"`
	Labels    map[string]string      `json:"labels"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ContainerStatus representa el estado de un contenedor
type ContainerStatus string

const (
	ContainerStatusCreated ContainerStatus = "created"
	ContainerStatusRunning ContainerStatus = "running"
	ContainerStatusStopped ContainerStatus = "stopped"
	ContainerStatusPaused  ContainerStatus = "paused"
	ContainerStatusExited  ContainerStatus = "exited"
	ContainerStatusError   ContainerStatus = "error"
)

// CreateContainerRequest contiene los parámetros para crear un contenedor
type CreateContainerRequest struct {
	Name          string                 `json:"name"`
	Image         string                 `json:"image"`
	Command       []string               `json:"command"`
	WorkingDir    string                 `json:"working_dir"`
	Environment   map[string]string      `json:"environment"`
	Labels        map[string]string      `json:"labels"`
	Ports         []PortMapping          `json:"ports"`
	Volumes       []VolumeMount          `json:"volumes"`
	Resources     *ResourceConfig        `json:"resources"`
	NetworkMode   string                 `json:"network_mode"`
	RestartPolicy string                 `json:"restart_policy"`
	AutoRemove    bool                   `json:"auto_remove"`
	Privileged    bool                   `json:"privileged"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ContainerConfig contiene la configuración del contenedor
type ContainerConfig struct {
	Command       []string               `json:"command"`
	WorkingDir    string                 `json:"working_dir"`
	Environment   map[string]string      `json:"environment"`
	Labels        map[string]string      `json:"labels"`
	RestartPolicy string                 `json:"restart_policy"`
	AutoRemove    bool                   `json:"auto_remove"`
	Privileged    bool                   `json:"privileged"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// NetworkConfig contiene la configuración de red del contenedor
type NetworkConfig struct {
	IPAddress   string        `json:"ip_address"`
	Gateway     string        `json:"gateway"`
	Ports       []PortMapping `json:"ports"`
	NetworkMode string        `json:"network_mode"`
}

// PortMapping define un mapeo de puertos
type PortMapping struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// VolumeMount define un montaje de volumen
type VolumeMount struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	ReadOnly   bool   `json:"read_only"`
	VolumeType string `json:"volume_type"`
}

// ResourceConfig define los recursos del contenedor
type ResourceConfig struct {
	Memory    int64 `json:"memory"`     // en bytes
	CPUShares int64 `json:"cpu_shares"` // shares de CPU
	CPULimit  int64 `json:"cpu_limit"`  // límite de CPU en nanosegundos
}

// ExecResult contiene el resultado de la ejecución de un comando
type ExecResult struct {
	Output   string `json:"output"`
	Error    string `json:"error"`
	ExitCode int    `json:"exit_code"`
}

// EventCallback es una función llamada cuando ocurre un evento
type EventCallback func(event Event)

// Event representa un evento del runtime
type Event struct {
	Type        string                 `json:"type"`
	Message     string                 `json:"message"`
	Timestamp   time.Time              `json:"timestamp"`
	ContainerID string                 `json:"container_id"`
	Runtime     RuntimeType            `json:"runtime"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// RuntimeFactory crea instancias de runtime según el tipo
type RuntimeFactory interface {
	CreateRuntime(runtimeType RuntimeType) (ContainerRuntime, error)
	GetAvailableRuntimes() []RuntimeType
	GetPreferredRuntime() RuntimeType
	GetOSInfo() *OSInfo
}

// OSInfo contiene información sobre el sistema operativo
type OSInfo struct {
	OS           string `json:"os"`
	Distribution string `json:"distribution"`
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	IsContainer  bool   `json:"is_container"`
	IsVM         bool   `json:"is_vm"`
	IsARM        bool   `json:"is_arm"`
	IsRaspberry  bool   `json:"is_raspberry"`
	Hostname     string `json:"hostname"`
}
