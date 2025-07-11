package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// DefaultRuntimeFactory implementa RuntimeFactory
type DefaultRuntimeFactory struct {
	availableRuntimes []RuntimeType
	preferredRuntime  RuntimeType
	mu                sync.RWMutex
}

// NewRuntimeFactory crea una nueva instancia del factory
func NewRuntimeFactory() *DefaultRuntimeFactory {
	factory := &DefaultRuntimeFactory{
		availableRuntimes: make([]RuntimeType, 0),
	}

	// Detectar SO y runtimes disponibles
	factory.detectAvailableRuntimes()

	return factory
}

// NewDefaultRuntimeFactory creates a new instance of DefaultRuntimeFactory
func NewDefaultRuntimeFactory() RuntimeFactory {
	factory := &DefaultRuntimeFactory{}
	factory.detectAvailableRuntimes()
	return factory
}

// detectAvailableRuntimes detecta qué runtimes están disponibles en el sistema
func (f *DefaultRuntimeFactory) detectAvailableRuntimes() {
	f.mu.Lock()
	defer f.mu.Unlock()

	var available []RuntimeType

	// Verificar LXC
	if f.isLXCAvailable() {
		available = append(available, RuntimeTypeLXC)
		logrus.Info("LXC runtime detectado y disponible")
	} else {
		logrus.Debug("LXC runtime no disponible")
	}

	// Verificar Docker
	if f.isDockerAvailable() {
		available = append(available, RuntimeTypeDocker)
		logrus.Info("Docker runtime detectado y disponible")
	} else {
		logrus.Debug("Docker runtime no disponible")
	}

	// Verificar containerd
	if f.isContainerdAvailable() {
		available = append(available, RuntimeTypeContainerd)
		logrus.Info("containerd runtime detectado y disponible")
	} else {
		logrus.Debug("containerd runtime no disponible")
	}

	// Si no hay runtimes disponibles, al menos agregar containerd como simulado
	if len(available) == 0 {
		available = append(available, RuntimeTypeContainerd)
		logrus.Info("No hay runtimes nativos disponibles, usando containerd simulado")
	}

	f.availableRuntimes = available

	// Determinar runtime preferido
	osInfo := f.getOSInfo()
	f.preferredRuntime = f.determinePreferredRuntime(osInfo)

	logrus.Infof("Runtimes disponibles: %v, preferido: %s", available, f.preferredRuntime)
}

// getOSInfo obtiene información del sistema operativo
func (f *DefaultRuntimeFactory) getOSInfo() *OSInfo {
	hostname, _ := os.Hostname()

	info := &OSInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Hostname:     hostname,
	}

	// Detectar si es ARM
	info.IsARM = runtime.GOARCH == "arm64" || runtime.GOARCH == "arm"

	// En Linux, obtener información adicional
	if runtime.GOOS == "linux" {
		info.Distribution = f.detectLinuxDistribution()
		info.Version = f.detectLinuxVersion()
		info.IsRaspberry = f.isRaspberryPi()
	}

	// Detectar si estamos en un contenedor
	info.IsContainer = f.isRunningInContainer()

	// Detectar si estamos en una VM
	info.IsVM = f.isRunningInVM()

	return info
}

// detectLinuxDistribution detecta la distribución de Linux
func (f *DefaultRuntimeFactory) detectLinuxDistribution() string {
	// Intentar leer /etc/os-release
	if content, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "ID=") {
				return strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
			}
		}
	}

	// Fallback: intentar otros métodos
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return "debian"
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return "rhel"
	}
	if _, err := os.Stat("/etc/arch-release"); err == nil {
		return "arch"
	}

	return "unknown"
}

// detectLinuxVersion detecta la versión de Linux
func (f *DefaultRuntimeFactory) detectLinuxVersion() string {
	if content, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "VERSION_ID=") {
				return strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
	}
	return "unknown"
}

// isRaspberryPi detecta si estamos ejecutando en una Raspberry Pi
func (f *DefaultRuntimeFactory) isRaspberryPi() bool {
	// Verificar /proc/cpuinfo
	if content, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		contentStr := strings.ToLower(string(content))
		return strings.Contains(contentStr, "raspberry") || strings.Contains(contentStr, "bcm283")
	}

	// Verificar /proc/device-tree/model
	if content, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		contentStr := strings.ToLower(string(content))
		return strings.Contains(contentStr, "raspberry")
	}

	return false
}

// isRunningInContainer detecta si estamos ejecutando dentro de un contenedor
func (f *DefaultRuntimeFactory) isRunningInContainer() bool {
	// Verificar /.dockerenv
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Verificar /proc/1/cgroup
	if content, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		contentStr := string(content)
		return strings.Contains(contentStr, "docker") || strings.Contains(contentStr, "lxc")
	}

	return false
}

// isRunningInVM detecta si estamos ejecutando en una máquina virtual
func (f *DefaultRuntimeFactory) isRunningInVM() bool {
	// Verificar DMI
	if content, err := os.ReadFile("/sys/class/dmi/id/product_name"); err == nil {
		contentStr := strings.ToLower(string(content))
		vmIndicators := []string{"vmware", "virtualbox", "qemu", "kvm", "xen", "hyper-v"}
		for _, indicator := range vmIndicators {
			if strings.Contains(contentStr, indicator) {
				return true
			}
		}
	}

	return false
}

// isLXCAvailable verifica si LXC está disponible
func (f *DefaultRuntimeFactory) isLXCAvailable() bool {
	// Verificar comandos LXC
	lxcCommands := []string{"lxc-create", "lxc-start", "lxc-stop", "lxc-info"}

	for _, cmd := range lxcCommands {
		if _, err := exec.LookPath(cmd); err != nil {
			return false
		}
	}

	// Verificar permisos y configuración
	if runtime.GOOS == "linux" {
		// Verificar si el usuario puede usar LXC
		if os.Geteuid() != 0 {
			// Usuario no root, verificar si está en grupo lxd/lxc
			if !f.isUserInLXCGroup() {
				return false
			}
		}
	}

	return true
}

// isDockerAvailable verifica si Docker está disponible
func (f *DefaultRuntimeFactory) isDockerAvailable() bool {
	// Verificar comando docker
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}

	// Verificar que el daemon esté corriendo
	if err := f.checkDockerDaemon(); err != nil {
		return false
	}

	return true
}

// isContainerdAvailable verifica si containerd está disponible
func (f *DefaultRuntimeFactory) isContainerdAvailable() bool {
	// Verificar comando containerd
	if _, err := exec.LookPath("containerd"); err != nil {
		return false
	}

	// Verificar comando ctr (client de containerd)
	if _, err := exec.LookPath("ctr"); err != nil {
		return false
	}

	// Verificar que el daemon esté corriendo
	if err := f.checkContainerdDaemon(); err != nil {
		return false
	}

	return true
}

// isUserInLXCGroup verifica si el usuario está en un grupo LXC
func (f *DefaultRuntimeFactory) isUserInLXCGroup() bool {
	cmd := exec.Command("groups")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	groups := strings.ToLower(string(output))
	return strings.Contains(groups, "lxd") || strings.Contains(groups, "lxc")
}

// checkContainerdDaemon verifica que el daemon de containerd esté corriendo
func (f *DefaultRuntimeFactory) checkContainerdDaemon() error {
	cmd := exec.Command("ctr", "version")
	return cmd.Run()
}

// checkDockerDaemon verifica que el daemon de Docker esté corriendo
func (f *DefaultRuntimeFactory) checkDockerDaemon() error {
	cmd := exec.Command("docker", "version", "--format", "{{.Client.Version}}")
	return cmd.Run()
}

// determinePreferredRuntime determina el runtime preferido según el SO
func (f *DefaultRuntimeFactory) determinePreferredRuntime(osInfo *OSInfo) RuntimeType {
	// Reglas de selección de runtime:

	// 1. Si estamos en Raspberry Pi, preferir LXC (mejor rendimiento ARM)
	if osInfo.IsRaspberry {
		if f.isRuntimeAvailable(RuntimeTypeLXC) {
			return RuntimeTypeLXC
		}
	}

	// 2. Si estamos en un contenedor, preferir containerd
	if osInfo.IsContainer {
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	}

	// 3. En macOS y Windows, preferir Docker (mejor compatibilidad)
	if osInfo.OS == "darwin" || osInfo.OS == "windows" {
		if f.isRuntimeAvailable(RuntimeTypeDocker) {
			return RuntimeTypeDocker
		}
	}

	// 4. Si estamos en arquitectura ARM, preferir LXC
	if osInfo.IsARM {
		if f.isRuntimeAvailable(RuntimeTypeLXC) {
			return RuntimeTypeLXC
		}
	}

	// 5. En distribuciones específicas, preferir según compatibilidad
	switch osInfo.Distribution {
	case "ubuntu", "debian":
		// Ubuntu/Debian tienen buen soporte para LXC
		if f.isRuntimeAvailable(RuntimeTypeLXC) {
			return RuntimeTypeLXC
		}
	case "rhel", "centos", "fedora":
		// Red Hat family prefiere containerd
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	}

	// 6. Fallback general: Docker > containerd > LXC
	if f.isRuntimeAvailable(RuntimeTypeDocker) {
		return RuntimeTypeDocker
	}

	// 7. Fallback a containerd
	if f.isRuntimeAvailable(RuntimeTypeContainerd) {
		return RuntimeTypeContainerd
	}

	// 8. Fallback a LXC
	if f.isRuntimeAvailable(RuntimeTypeLXC) {
		return RuntimeTypeLXC
	}

	// Si no hay ninguno disponible, devolver Docker como default
	return RuntimeTypeDocker
}

// isRuntimeAvailable verifica si un runtime específico está disponible
func (f *DefaultRuntimeFactory) isRuntimeAvailable(runtimeType RuntimeType) bool {
	for _, rt := range f.availableRuntimes {
		if rt == runtimeType {
			return true
		}
	}
	return false
}

// CreateRuntime crea una instancia de runtime según el tipo especificado
func (f *DefaultRuntimeFactory) CreateRuntime(runtimeType RuntimeType) (ContainerRuntime, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Verificar que el runtime esté disponible
	if !f.isRuntimeAvailable(runtimeType) {
		return nil, fmt.Errorf("runtime %s is not available", runtimeType)
	}

	switch runtimeType {
	case RuntimeTypeLXC:
		return NewLXCClient()

	case RuntimeTypeDocker:
		return NewDockerClient()

	case RuntimeTypeContainerd:
		return NewContainerdClient("", "")

	default:
		return nil, fmt.Errorf("unsupported runtime type: %s", runtimeType)
	}
}

// GetAvailableRuntimes devuelve los runtimes disponibles en el sistema
func (f *DefaultRuntimeFactory) GetAvailableRuntimes() []RuntimeType {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.availableRuntimes
}

// GetPreferredRuntime devuelve el runtime preferido para el SO actual
func (f *DefaultRuntimeFactory) GetPreferredRuntime() RuntimeType {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.preferredRuntime
}

// RefreshRuntimes actualiza la detección de runtimes
func (f *DefaultRuntimeFactory) RefreshRuntimes() {
	f.detectAvailableRuntimes()
}

// GetOSInfo devuelve información del sistema operativo
func (f *DefaultRuntimeFactory) GetOSInfo() *OSInfo {
	return f.getOSInfo()
}

// GetRuntimeForOS devuelve el runtime más apropiado para un SO específico
func (f *DefaultRuntimeFactory) GetRuntimeForOS(osInfo *OSInfo) RuntimeType {
	// En Raspberry Pi y sistemas ARM, preferir LXC
	if osInfo.IsRaspberry || osInfo.IsARM {
		if f.isRuntimeAvailable(RuntimeTypeLXC) {
			return RuntimeTypeLXC
		}
	}

	// En sistemas x86_64 modernos, preferir containerd
	if osInfo.Architecture == "amd64" || osInfo.Architecture == "x86_64" {
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	}

	// Fallback al runtime preferido
	return f.preferredRuntime
}

// UpdateAvailableRuntimes actualiza la lista de runtimes disponibles
func (f *DefaultRuntimeFactory) UpdateAvailableRuntimes() {
	f.mu.Lock()
	defer f.mu.Unlock()

	var available []RuntimeType

	// Verificar LXC
	if err := validateLXCRuntime(); err == nil {
		available = append(available, RuntimeTypeLXC)
		logrus.Info("LXC runtime detected and available")
	} else {
		logrus.Debugf("LXC runtime not available: %v", err)
	}

	// Verificar Docker
	if err := validateDockerRuntime(); err == nil {
		available = append(available, RuntimeTypeDocker)
		logrus.Info("Docker runtime detected and available")
	} else {
		logrus.Debugf("Docker runtime not available: %v", err)
	}

	// Verificar containerd
	if err := validateContainerdRuntime(); err == nil {
		available = append(available, RuntimeTypeContainerd)
		logrus.Info("containerd runtime detected and available")
	} else {
		logrus.Debugf("containerd runtime not available: %v", err)
	}

	f.availableRuntimes = available

	// Actualizar runtime preferido si el actual ya no está disponible
	if !f.isRuntimeAvailable(f.preferredRuntime) && len(available) > 0 {
		f.preferredRuntime = available[0]
		logrus.Infof("Updated preferred runtime to %s", f.preferredRuntime)
	}
}

// SetPreferredRuntime configura el runtime preferido manualmente
func (f *DefaultRuntimeFactory) SetPreferredRuntime(runtimeType RuntimeType) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.isRuntimeAvailable(runtimeType) {
		return fmt.Errorf("runtime %s is not available", runtimeType)
	}

	f.preferredRuntime = runtimeType
	logrus.Infof("Preferred runtime set to %s", runtimeType)
	return nil
}

// GetRuntimeInfo devuelve información detallada sobre todos los runtimes
func (f *DefaultRuntimeFactory) GetRuntimeInfo() map[RuntimeType]*RuntimeInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	info := make(map[RuntimeType]*RuntimeInfo)

	for _, runtimeType := range f.availableRuntimes {
		runtime, err := f.CreateRuntime(runtimeType)
		if err != nil {
			continue
		}

		runtimeInfo, err := runtime.GetRuntimeInfo()
		if err != nil {
			logrus.Warnf("Failed to get info for runtime %s: %v", runtimeType, err)
			continue
		}

		info[runtimeType] = runtimeInfo
		runtime.Close()
	}

	return info
}

// validateLXCRuntime verifica si LXC está disponible en el sistema
func validateLXCRuntime() error {
	// Verificar si lxc-info está disponible
	if err := exec.Command("lxc-info", "--version").Run(); err != nil {
		return fmt.Errorf("lxc-info command not found: %w", err)
	}

	// Verificar si el usuario tiene permisos para usar LXC
	if os.Geteuid() != 0 {
		// Verificar si existe configuración de usuario no privilegiado
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not get home directory: %w", err)
		}

		lxcConfigPath := filepath.Join(homeDir, ".config", "lxc")
		if _, err := os.Stat(lxcConfigPath); os.IsNotExist(err) {
			return fmt.Errorf("LXC not configured for unprivileged use")
		}
	}

	return nil
}

// validateDockerRuntime verifica si Docker está disponible en el sistema
func validateDockerRuntime() error {
	// Verificar si docker está disponible
	if err := exec.Command("docker", "--version").Run(); err != nil {
		return fmt.Errorf("docker command not found: %w", err)
	}

	// Verificar que el daemon esté corriendo
	if err := exec.Command("docker", "version").Run(); err != nil {
		return fmt.Errorf("docker daemon not running: %w", err)
	}

	return nil
}

// validateContainerdRuntime verifica si containerd está disponible
func validateContainerdRuntime() error {
	// Verificar si el socket de containerd existe
	socketPath := "/run/containerd/containerd.sock"
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return fmt.Errorf("containerd socket not found at %s", socketPath)
	}

	// Verificar si tenemos permisos para acceder al socket
	if err := exec.Command("containerd", "--version").Run(); err != nil {
		return fmt.Errorf("containerd command not available: %w", err)
	}

	return nil
}
