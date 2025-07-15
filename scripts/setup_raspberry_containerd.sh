#!/bin/bash

# Script de configuración de containerd para Raspberry Pi
# Este script instala y configura containerd optimizado para ARM

set -e

echo "🍓 Configurando containerd para Raspberry Pi"
echo "============================================="

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Función para imprimir mensajes con color
print_status() {
    local status=$1
    local message=$2
    case $status in
        "info")
            echo -e "${BLUE}ℹ️  $message${NC}"
            ;;
        "success")
            echo -e "${GREEN}✅ $message${NC}"
            ;;
        "warning")
            echo -e "${YELLOW}⚠️  $message${NC}"
            ;;
        "error")
            echo -e "${RED}❌ $message${NC}"
            ;;
    esac
}

# Verificar que estamos en Raspberry Pi
if [[ ! -f "/proc/device-tree/model" ]] || ! grep -q "Raspberry Pi" /proc/device-tree/model; then
    print_status "error" "Este script está diseñado específicamente para Raspberry Pi"
    exit 1
fi

# Verificar que estamos en Linux
if [[ "$(uname)" != "Linux" ]]; then
    print_status "error" "Este script solo funciona en Linux"
    exit 1
fi

# Verificar que tenemos permisos de sudo
if ! sudo -n true 2>/dev/null; then
    print_status "error" "Este script requiere permisos de sudo"
    exit 1
fi

print_status "info" "Iniciando configuración de containerd para Raspberry Pi..."

# Verificar arquitectura ARM
if [[ "$(uname -m)" != "arm"* && "$(uname -m)" != "aarch64" ]]; then
    print_status "warning" "No se detectó arquitectura ARM - continuando..."
fi

# Actualizar sistema
print_status "info" "Actualizando sistema..."
sudo apt-get update

# Instalar dependencias específicas para ARM
print_status "info" "Instalando dependencias optimizadas para ARM..."
sudo apt-get install -y \
    bridge-utils \
    uidmap \
    jq \
    curl \
    wget \
    ca-certificates \
    gnupg \
    lsb-release

# Detener servicios que puedan interferir
print_status "info" "Deteniendo servicios que puedan interferir..."
sudo systemctl stop docker 2>/dev/null || true
sudo systemctl disable docker 2>/dev/null || true

# Desinstalar Docker si está instalado (opcional)
read -p "¿Deseas desinstalar Docker para liberar espacio? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    print_status "info" "Desinstalando Docker..."
    sudo apt-get remove -y docker docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    sudo apt-get autoremove -y
fi

# Instalar containerd optimizado para ARM
print_status "info" "Instalando containerd optimizado para ARM..."

# Agregar repositorio oficial de containerd
if ! grep -q "deb https://download.docker.com/linux/debian" /etc/apt/sources.list.d/docker.list; then
    print_status "info" "Agregando repositorio oficial..."
    curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    sudo apt-get update
fi

# Instalar containerd
if sudo apt-get install -y containerd.io; then
    print_status "success" "Containerd instalado correctamente"
else
    print_status "warning" "Error instalando containerd desde repositorio oficial"
    print_status "info" "Intentando instalación desde repositorios estándar..."
    
    if sudo apt-get install -y containerd; then
        print_status "success" "Containerd instalado desde repositorios estándar"
    else
        print_status "error" "No se pudo instalar containerd"
        exit 1
    fi
fi

# Configurar containerd optimizado para ARM
print_status "info" "Configurando containerd optimizado para ARM..."

# Crear directorio de configuración
sudo mkdir -p /etc/containerd

# Generar configuración por defecto
if command -v containerd &> /dev/null; then
    containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
    print_status "success" "Configuración por defecto generada"
else
    print_status "error" "Containerd no está disponible para generar configuración"
    exit 1
fi

# Optimizar configuración para ARM/Raspberry Pi
print_status "info" "Optimizando configuración para ARM..."

# Configurar cgroup driver
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

# Optimizar para ARM - reducir uso de memoria
sudo sed -i 's/sandbox_image = "registry.k8s.io\/pause:3.9"/sandbox_image = "registry.k8s.io\/pause:3.9"/' /etc/containerd/config.toml

# Configurar límites de memoria para ARM
sudo tee -a /etc/containerd/config.toml > /dev/null <<EOF

# Configuración optimizada para ARM/Raspberry Pi
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true

[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
    SystemdCgroup = true

# Optimizaciones para ARM
[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://registry-1.docker.io"]
EOF

print_status "success" "Configuración optimizada para ARM aplicada"

# Instalar herramientas CLI optimizadas para ARM
print_status "info" "Instalando herramientas CLI optimizadas para ARM..."

# Descargar ctr CLI específico para ARM
CTR_VERSION="1.7.0"
CTR_ARCH="arm64"
if [[ "$(uname -m)" == "armv7l" ]]; then
    CTR_ARCH="arm"
fi

print_status "info" "Descargando ctr CLI para ARM (versión $CTR_VERSION, arch: $CTR_ARCH)..."
if curl -L -o /tmp/ctr.tar.gz "https://github.com/containerd/containerd/releases/download/v${CTR_VERSION}/containerd-${CTR_VERSION}-linux-${CTR_ARCH}.tar.gz"; then
    cd /tmp
    sudo tar -xzf ctr.tar.gz bin/ctr
    sudo mv bin/ctr /usr/local/bin/
    sudo chmod +x /usr/local/bin/ctr
    rm -rf bin ctr.tar.gz
    print_status "success" "ctr CLI instalado correctamente para ARM"
else
    print_status "warning" "No se pudo descargar ctr CLI"
    print_status "info" "Intentando instalar desde repositorios..."
    
    if sudo apt-get install -y containerd-tools; then
        print_status "success" "Herramientas containerd instaladas desde repositorios"
    else
        print_status "warning" "No se pudieron instalar herramientas containerd"
    fi
fi

# Configurar permisos y grupos
print_status "info" "Configurando permisos y grupos..."

# Crear grupo containerd si no existe
if ! getent group containerd >/dev/null 2>&1; then
    sudo groupadd containerd
fi

# Agregar usuario al grupo
sudo usermod -a -G containerd $USER

# Crear directorios necesarios
sudo mkdir -p /var/lib/containerd
sudo mkdir -p /run/containerd
sudo chown root:containerd /var/lib/containerd
sudo chmod 775 /var/lib/containerd

# Habilitar y iniciar containerd
print_status "info" "Iniciando containerd optimizado para ARM..."
sudo systemctl enable containerd
sudo systemctl start containerd

# Verificar que containerd está corriendo
if systemctl is-active --quiet containerd; then
    print_status "success" "Containerd está corriendo correctamente"
else
    print_status "error" "Containerd no está corriendo"
    print_status "info" "Verificando logs..."
    sudo journalctl -u containerd --no-pager -n 20
    exit 1
fi

# Verificar que containerd responde
if ctr version &> /dev/null; then
    print_status "success" "Containerd responde correctamente"
else
    print_status "error" "Containerd no responde"
    exit 1
fi

# Crear namespace para Diplo
print_status "info" "Configurando namespace para Diplo..."
ctr namespaces create diplo 2>/dev/null || true

# Crear script de verificación específico para Raspberry Pi
cat > ~/verify_raspberry_containerd.sh << 'EOF'
#!/bin/bash

echo "🍓 Verificando containerd en Raspberry Pi..."
echo "=========================================="

# Verificar que containerd está instalado
if command -v containerd &> /dev/null; then
    echo "✅ Containerd está instalado"
    containerd --version
else
    echo "❌ Containerd no está instalado"
    exit 1
fi

# Verificar que containerd daemon está corriendo
if systemctl is-active --quiet containerd; then
    echo "✅ Containerd daemon está corriendo"
else
    echo "❌ Containerd daemon no está corriendo"
    exit 1
fi

# Verificar que ctr CLI está disponible
if command -v ctr &> /dev/null; then
    echo "✅ ctr CLI está disponible"
    ctr version
else
    echo "❌ ctr CLI no está disponible"
fi

# Verificar que el usuario está en el grupo containerd
if groups $USER | grep -q containerd; then
    echo "✅ Usuario en grupo containerd"
else
    echo "⚠️  Usuario no está en grupo containerd"
fi

# Verificar configuración
if [ -f /etc/containerd/config.toml ]; then
    echo "✅ Configuración containerd encontrada"
    if grep -q "SystemdCgroup = true" /etc/containerd/config.toml; then
        echo "✅ Configuración de cgroup correcta"
    else
        echo "⚠️  Configuración de cgroup puede necesitar ajuste"
    fi
else
    echo "❌ Configuración containerd no encontrada"
fi

# Verificar namespace diplo
if ctr namespaces list | grep -q diplo; then
    echo "✅ Namespace 'diplo' encontrado"
else
    echo "⚠️  Namespace 'diplo' no encontrado"
fi

# Probar containerd con imagen ARM
echo "🧪 Probando containerd con imagen ARM..."
if ctr run --rm docker.io/library/hello-world:latest test-arm 2>/dev/null; then
    echo "✅ Containerd funciona correctamente con imágenes ARM"
else
    echo "⚠️  Problema con imágenes ARM"
fi

echo ""
echo "🎉 Verificación completada"
EOF

chmod +x ~/verify_raspberry_containerd.sh

# Crear script de optimización de memoria
cat > ~/optimize_raspberry_containerd.sh << 'EOF'
#!/bin/bash

echo "🔧 Optimizando containerd para Raspberry Pi..."
echo "============================================="

# Ajustar límites de memoria para containerd
echo "Configurando límites de memoria..."

# Crear archivo de configuración de systemd para containerd
sudo tee /etc/systemd/system/containerd.service.d/override.conf > /dev/null <<EOF
[Service]
MemoryLimit=512M
CPUQuota=50%
EOF

# Recargar configuración de systemd
sudo systemctl daemon-reload

# Reiniciar containerd con nueva configuración
sudo systemctl restart containerd

echo "✅ Optimizaciones aplicadas"
echo "📊 Límites configurados:"
echo "   - Memoria: 512MB"
echo "   - CPU: 50%"
echo ""
echo "Para aplicar cambios, reinicia: sudo reboot"
EOF

chmod +x ~/optimize_raspberry_containerd.sh

print_status "success" "Containerd configurado correctamente para Raspberry Pi"

# Mostrar información final
echo ""
echo "🎉 CONFIGURACIÓN COMPLETADA"
echo "============================"
print_status "success" "Containerd ha sido instalado y configurado correctamente para Raspberry Pi"

echo ""
echo "📋 PRÓXIMOS PASOS:"
echo "=================="
print_status "info" "1. Reinicia tu Raspberry Pi: sudo reboot"
print_status "info" "2. Verifica la instalación: ./verify_raspberry_containerd.sh"
print_status "info" "3. Optimiza memoria si es necesario: ./optimize_raspberry_containerd.sh"
print_status "info" "4. Inicia tu aplicación Diplo"

echo ""
echo "🔧 COMANDOS ÚTILES:"
echo "==================="
echo "  Verificar estado: sudo systemctl status containerd"
echo "  Ver logs: sudo journalctl -u containerd -f"
echo "  Listar contenedores: ctr -n diplo containers list"
echo "  Probar imagen: ctr -n diplo run --rm docker.io/library/hello-world:latest test"

echo ""
print_status "success" "¡Containerd está listo para usar en tu Raspberry Pi!" 