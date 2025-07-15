package runtime

import (
	"fmt"
	"os"
	"os/exec"
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
	osInfo := f.getOSInfo()

	// En Raspberry Pi, priorizar containerd
	if osInfo.IsRaspberry {
		logrus.Info("Detectado Raspberry Pi - priorizando containerd")

		// Verificar containerd primero
		if f.isContainerdAvailable() {
			available = append(available, RuntimeTypeContainerd)
			logrus.Info("Containerd runtime detectado y disponible en Raspberry Pi")
		} else {
			logrus.Warn("Containerd no disponible en Raspberry Pi - se recomienda instalarlo")
			logrus.Info("Para instalar containerd en Raspberry Pi: sudo ./scripts/install_containerd.sh")
		}

		// Verificar Docker como fallback
		if f.isDockerAvailable() {
			available = append(available, RuntimeTypeDocker)
			logrus.Info("Docker runtime detectado como fallback en Raspberry Pi")
		} else {
			logrus.Debug("Docker runtime no disponible en Raspberry Pi")
		}

		// Si no hay runtimes disponibles, agregar containerd como simulado
		if len(available) == 0 {
			available = append(available, RuntimeTypeContainerd)
			logrus.Warn("No hay runtimes nativos disponibles en Raspberry Pi, usando containerd simulado")
			logrus.Info("Se recomienda instalar containerd: sudo ./scripts/install_containerd.sh")
		}
	} else {
		// Para otros sistemas, usar lógica estándar
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

		// Si no hay runtimes disponibles, agregar containerd como simulado
		if len(available) == 0 {
			available = append(available, RuntimeTypeContainerd)
			logrus.Info("No hay runtimes nativos disponibles, usando containerd simulado")
		}
	}

	f.availableRuntimes = available

	// Determinar runtime preferido
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
	osInfo := f.getOSInfo()

	// En macOS, detectar containerd a través de Docker Desktop
	if osInfo.OS == "darwin" {
		// Verificar que Docker esté disponible
		if !f.isDockerAvailable() {
			logrus.Debug("Docker no disponible en macOS")
			return false
		}

		// Verificar que containerd esté disponible a través de Docker
		cmd := exec.Command("docker", "info")
		output, err := cmd.Output()
		if err != nil {
			logrus.Debug("No se pudo obtener información de Docker")
			return false
		}

		outputStr := string(output)
		if strings.Contains(outputStr, "containerd version:") {
			// Extraer la versión de containerd
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				if strings.Contains(line, "containerd version:") {
					version := strings.TrimSpace(strings.TrimPrefix(line, "containerd version:"))
					logrus.Infof("Containerd detectado a través de Docker Desktop: %s", version)
					return true
				}
			}
		}

		logrus.Debug("Containerd no disponible a través de Docker")
		return false
	}

	// Para otros sistemas (Linux), usar la detección original
	// Verificar comando containerd
	if _, err := exec.LookPath("containerd"); err != nil {
		logrus.Debug("Containerd no está instalado")
		return false
	}

	// Verificar comando ctr (client de containerd)
	if _, err := exec.LookPath("ctr"); err != nil {
		logrus.Debug("ctr CLI no está disponible")
		return false
	}

	// Verificar que el daemon esté corriendo
	if err := f.checkContainerdDaemon(); err != nil {
		logrus.Debugf("Containerd daemon no está corriendo: %v", err)
		return false
	}

	return true
}

// checkContainerdDaemon verifica que el daemon de containerd esté corriendo
func (f *DefaultRuntimeFactory) checkContainerdDaemon() error {
	// Verificar que el socket existe
	socketPath := "/run/containerd/containerd.sock"
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return fmt.Errorf("socket de containerd no encontrado en %s", socketPath)
	}

	// Verificar que containerd responde
	cmd := exec.Command("ctr", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("containerd no está corriendo: %w", err)
	}

	return nil
}

// checkDockerDaemon verifica que el daemon de Docker esté corriendo
func (f *DefaultRuntimeFactory) checkDockerDaemon() error {
	cmd := exec.Command("docker", "version", "--format", "{{.Client.Version}}")
	return cmd.Run()
}

// determinePreferredRuntime determina el runtime preferido según el SO
func (f *DefaultRuntimeFactory) determinePreferredRuntime(osInfo *OSInfo) RuntimeType {
	// Reglas de selección de runtime:

	// 1. Si estamos en Raspberry Pi, SIEMPRE preferir containerd (mejor rendimiento ARM)
	if osInfo.IsRaspberry {
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			logrus.Info("Raspberry Pi detectado - usando containerd como runtime preferido")
			return RuntimeTypeContainerd
		} else {
			logrus.Warn("Raspberry Pi detectado pero containerd no disponible - usando Docker como fallback")
			if f.isRuntimeAvailable(RuntimeTypeDocker) {
				return RuntimeTypeDocker
			}
		}
	}

	// 2. Si estamos en un contenedor, preferir containerd
	if osInfo.IsContainer {
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	}

	// 3. En macOS, SIEMPRE preferir Docker
	if osInfo.OS == "darwin" {
		if f.isRuntimeAvailable(RuntimeTypeDocker) {
			logrus.Info("macOS detectado - usando Docker como runtime preferido")
			return RuntimeTypeDocker
		}
		// Si por alguna razón tienes ctr y containerd real en Mac, lo puedes agregar aquí
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			logrus.Info("macOS detectado - usando containerd como runtime preferido")
			return RuntimeTypeContainerd
		}
	}

	// 4. Si estamos en arquitectura ARM, preferir containerd
	if osInfo.IsARM {
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	}

	// 5. En distribuciones específicas, preferir según compatibilidad
	switch osInfo.Distribution {
	case "ubuntu", "debian":
		// Ubuntu/Debian tienen buen soporte para containerd
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	case "rhel", "centos", "fedora":
		// Red Hat family prefiere containerd
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
		}
	}

	// 6. Fallback general: containerd > Docker
	if f.isRuntimeAvailable(RuntimeTypeContainerd) {
		return RuntimeTypeContainerd
	}

	// 7. Fallback a Docker
	if f.isRuntimeAvailable(RuntimeTypeDocker) {
		return RuntimeTypeDocker
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
	// En Raspberry Pi y sistemas ARM, preferir containerd
	if osInfo.IsRaspberry || osInfo.IsARM {
		if f.isRuntimeAvailable(RuntimeTypeContainerd) {
			return RuntimeTypeContainerd
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
