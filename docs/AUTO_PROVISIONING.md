# Auto-Provisionamiento LXC en Diplo

Diplo incluye un sistema de auto-provisionamiento que configura automáticamente LXC cuando se ejecuta en Raspberry Pi o sistemas Linux compatibles.

## 🚀 Características

- **Instalación automática**: Instala LXC si no está presente
- **Configuración automática**: Configura subuid/subgid, red y cgroups
- **Verificación automática**: Verifica que todo esté funcionando correctamente
- **Compatibilidad**: Optimizado para Raspberry Pi pero funciona en cualquier Linux

## 📋 Requisitos

- Sistema Linux (preferiblemente Raspberry Pi)
- Permisos de sudo
- Conexión a internet para descargar dependencias

## 🔧 Instalación Rápida

### Para Raspberry Pi

```bash
# Descargar e ejecutar el script de instalación
curl -sSL https://raw.githubusercontent.com/rodrwan/diplo/main/scripts/install_diplo_raspberry.sh | bash
```

### Para otros sistemas Linux

```bash
# Clonar el repositorio
git clone https://github.com/rodrwan/diplo.git
cd diplo

# Ejecutar el script de instalación
chmod +x scripts/install_diplo_raspberry.sh
./scripts/install_diplo_raspberry.sh
```

## 🔍 Verificación

Después de la instalación, verifica que todo esté funcionando:

```bash
# Verificar instalación
diplo-verify

# Probar deployment
diplo-test

# Verificar estado del servicio
sudo systemctl status diplo
```

## 🏗️ Cómo Funciona el Auto-Provisionamiento

### 1. Detección Automática

Cuando Diplo se inicia, detecta automáticamente:

- Si LXC está instalado
- Si está configurado para usuario no privilegiado
- Si los subuid/subgid están configurados
- Si la red está configurada

### 2. Instalación Automática

Si LXC no está instalado, Diplo:

1. Detecta el gestor de paquetes del sistema (apt, yum, dnf)
2. Instala LXC y dependencias necesarias
3. Configura templates básicos

### 3. Configuración Automática

Diplo configura automáticamente:

#### Subuid/Subgid
```bash
# Configuración automática de UID mappings
sudo usermod --add-subuids 100000-165536 $USER
sudo usermod --add-subgids 100000-165536 $USER
```

#### Red LXC
```bash
# Configuración de bridge y DHCP
USE_LXC_BRIDGE="true"
LXC_BRIDGE="lxcbr0"
LXC_ADDR="10.0.3.1"
LXC_NETMASK="255.255.255.0"
LXC_NETWORK="10.0.3.0/24"
LXC_DHCP_RANGE="10.0.3.2,10.0.3.254"
```

#### Cgroups (Raspberry Pi)
```bash
# Configuración en /boot/cmdline.txt
cgroup_enable=cpuset cgroup_enable=memory cgroup_memory=1
```

#### Configuración de Usuario
```bash
# Crear configuración en ~/.config/lxc/default.conf
lxc.include = /etc/lxc/default.conf
lxc.idmap = u 0 100000 65536
lxc.idmap = g 0 100000 65536
lxc.network.type = veth
lxc.network.link = lxcbr0
lxc.network.flags = up
```

### 4. Verificación Automática

Diplo verifica que todo esté funcionando:

1. Comprueba que los comandos LXC estén disponibles
2. Ejecuta `lxc-checkconfig` para verificar configuración
3. Crea un contenedor de prueba para validar funcionamiento
4. Limpia el contenedor de prueba

## 🔌 Endpoints de API

### GET /api/lxc/status

Verifica el estado de LXC y ejecuta auto-provisionamiento si es necesario.

**Respuesta:**
```json
{
  "runtime_type": "lxc",
  "available": true,
  "version": "auto-provisioned",
  "timestamp": "2024-01-15T10:30:00Z",
  "auto_provisioned": true,
  "message": "LXC auto-provisionado exitosamente",
  "provision_status": {
    "installed": true,
    "configured": true,
    "subid_setup": true,
    "network_setup": true,
    "cgroups_setup": true,
    "ready": true
  }
}
```

### POST /api/lxc/provision

Fuerza el provisionamiento manual de LXC.

**Respuesta:**
```json
{
  "message": "LXC provisionado exitosamente",
  "status": {
    "installed": true,
    "configured": true,
    "subid_setup": true,
    "network_setup": true,
    "cgroups_setup": true,
    "ready": true
  },
  "ready": true
}
```

## 🛠️ Configuración Manual

Si prefieres configurar LXC manualmente:

### 1. Instalar LXC
```bash
# Debian/Ubuntu/Raspberry Pi
sudo apt-get update
sudo apt-get install -y lxc lxc-templates bridge-utils uidmap

# CentOS/RHEL
sudo yum install -y lxc lxc-templates

# Fedora
sudo dnf install -y lxc lxc-templates
```

### 2. Configurar Usuario No Privilegiado
```bash
# Crear grupo lxd
sudo groupadd lxd

# Agregar usuario al grupo
sudo usermod -a -G lxd $USER

# Configurar subuid/subgid
sudo usermod --add-subuids 100000-165536 $USER
sudo usermod --add-subgids 100000-165536 $USER
```

### 3. Configurar Red
```bash
# Crear configuración de red
sudo tee /etc/default/lxc-net > /dev/null << EOF
USE_LXC_BRIDGE="true"
LXC_BRIDGE="lxcbr0"
LXC_ADDR="10.0.3.1"
LXC_NETMASK="255.255.255.0"
LXC_NETWORK="10.0.3.0/24"
LXC_DHCP_RANGE="10.0.3.2,10.0.3.254"
LXC_DHCP_MAX="253"
EOF

# Habilitar forwarding de IP
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
sudo sysctl -w net.ipv4.ip_forward=1
```

### 4. Configurar Cgroups (Raspberry Pi)
```bash
# Crear backup
sudo cp /boot/cmdline.txt /boot/cmdline.txt.backup

# Agregar configuración de cgroups
CMDLINE=$(cat /boot/cmdline.txt)
echo "$CMDLINE cgroup_enable=cpuset cgroup_enable=memory cgroup_memory=1" | sudo tee /boot/cmdline.txt
```

### 5. Crear Configuración de Usuario
```bash
# Crear directorios
mkdir -p ~/.config/lxc
mkdir -p ~/.local/share/lxc

# Crear configuración
cat > ~/.config/lxc/default.conf << EOF
lxc.include = /etc/lxc/default.conf
lxc.idmap = u 0 100000 65536
lxc.idmap = g 0 100000 65536
lxc.network.type = veth
lxc.network.link = lxcbr0
lxc.network.flags = up
lxc.network.hwaddr = 00:16:3e:xx:xx:xx
lxc.rootfs.path = dir:/home/$USER/.local/share/lxc/\${LXC_NAME}/rootfs
lxc.apparmor.profile = unconfined
lxc.cap.drop = 
lxc.cgroup.devices.allow = a
lxc.cgroup.devices.deny = 
EOF
```

## 🔍 Diagnóstico de Problemas

### Verificar Instalación LXC
```bash
# Verificar comandos
lxc-create --version
lxc-start --version
lxc-info --version

# Verificar configuración
lxc-checkconfig
```

### Verificar Configuración de Usuario
```bash
# Verificar subuid/subgid
grep "^$USER:" /etc/subuid
grep "^$USER:" /etc/subgid

# Verificar configuración
ls -la ~/.config/lxc/
cat ~/.config/lxc/default.conf
```

### Verificar Red
```bash
# Verificar bridge
ip addr show lxcbr0

# Verificar forwarding
sysctl net.ipv4.ip_forward
```

### Verificar Cgroups (Raspberry Pi)
```bash
# Verificar configuración en cmdline
grep cgroup /boot/cmdline.txt

# Verificar cgroups montados
mount | grep cgroup
```

## 🧪 Pruebas

### Prueba Básica de LXC
```bash
# Crear contenedor de prueba
lxc-create -n test-container -t ubuntu -- -r focal

# Iniciar contenedor
lxc-start -n test-container

# Verificar estado
lxc-info -n test-container

# Detener y eliminar
lxc-stop -n test-container
lxc-destroy -n test-container
```

### Prueba de Auto-Provisionamiento
```bash
# Ejecutar script de prueba
./scripts/test_auto_provision.sh
```

## 📝 Logs

Los logs del auto-provisionamiento se pueden encontrar en:

- **Systemd logs**: `sudo journalctl -u diplo -f`
- **Aplicación logs**: `~/.local/share/diplo/diplo.log`
- **LXC logs**: `/var/log/lxc/`

## 🔧 Configuración Avanzada

### Configurar Imágenes Base Personalizadas

Puedes configurar imágenes base personalizadas editando la función `getLXCBaseImage()` en `internal/runtime/lxc_provisioner.go`:

```go
func getLXCBaseImage(language string) string {
    switch language {
    case "go":
        return "ubuntu:22.04"
    case "javascript", "node":
        return "ubuntu:22.04"
    case "python":
        return "ubuntu:22.04"
    case "rust":
        return "ubuntu:22.04"
    default:
        return "ubuntu:22.04"
    }
}
```

### Configurar Recursos de Contenedores

Puedes ajustar los recursos asignados a los contenedores LXC en `internal/server/handlers/hybrid_handlers.go`:

```go
Resources: &runtimePkg.ResourceConfig{
    Memory:    512 * 1024 * 1024, // 512MB
    CPUShares: 512,
},
```

## 🚨 Solución de Problemas

### Error: "Permission denied"
```bash
# Verificar que el usuario esté en el grupo lxd
groups $USER

# Agregar usuario al grupo si no está
sudo usermod -a -G lxd $USER

# Reiniciar sesión o ejecutar
newgrp lxd
```

### Error: "No subuid ranges found"
```bash
# Configurar subuid/subgid
sudo usermod --add-subuids 100000-165536 $USER
sudo usermod --add-subgids 100000-165536 $USER

# Reiniciar sistema
sudo reboot
```

### Error: "Network bridge not found"
```bash
# Verificar configuración de red
cat /etc/default/lxc-net

# Reiniciar servicio de red LXC
sudo systemctl restart lxc-net
```

### Error: "Cgroups not available"
```bash
# Verificar configuración en cmdline
grep cgroup /boot/cmdline.txt

# Si no está configurado, agregar manualmente
sudo nano /boot/cmdline.txt
# Agregar: cgroup_enable=cpuset cgroup_enable=memory cgroup_memory=1

# Reiniciar sistema
sudo reboot
```

## 📚 Referencias

- [LXC Documentation](https://linuxcontainers.org/lxc/documentation/)
- [LXC User Namespaces](https://linuxcontainers.org/lxc/documentation/#user-namespaces)
- [Raspberry Pi LXC Setup](https://www.raspberrypi.org/documentation/linux/containers/)
- [Systemd Service Configuration](https://www.freedesktop.org/software/systemd/man/systemd.service.html)

## 🤝 Contribuir

Si encuentras problemas o quieres mejorar el auto-provisionamiento:

1. Abre un issue en GitHub
2. Describe el problema y el entorno
3. Incluye logs relevantes
4. Propón una solución si es posible

## 📄 Licencia

Este proyecto está bajo la licencia MIT. Ver `LICENSE` para más detalles. 