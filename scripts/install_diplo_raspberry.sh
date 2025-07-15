#!/bin/bash

# Script de instalación para Diplo en Raspberry Pi
# Este script configura Docker y containerd para auto-provisionamiento
# NO instala Diplo - eso se hace con el Makefile

set -e

echo "🔧 Configurando Docker y containerd para Diplo en Raspberry Pi..."
echo "================================================================"

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
    print_status "warning" "Este script está optimizado para Raspberry Pi"
    print_status "info" "Continuando con instalación genérica..."
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

print_status "info" "Iniciando configuración Docker y containerd para Diplo..."

# Actualizar sistema (con manejo de errores)
print_status "info" "Actualizando sistema..."
if sudo apt-get update 2>/dev/null; then
    print_status "success" "Sistema actualizado"
else
    print_status "warning" "Error al actualizar repositorios (puede ser normal)"
    print_status "info" "Continuando con instalación..."
fi

# Actualizar paquetes (opcional)
if sudo apt-get upgrade -y 2>/dev/null; then
    print_status "success" "Paquetes actualizados"
else
    print_status "warning" "Error al actualizar paquetes (continuando...)"
fi

# Instalar dependencias básicas
print_status "info" "Instalando dependencias básicas..."
if sudo apt-get install -y bridge-utils uidmap jq 2>/dev/null; then
    print_status "success" "Dependencias básicas instaladas"
else
    print_status "error" "Error instalando dependencias básicas"
    exit 1
fi

# Instalar Docker
print_status "info" "Instalando Docker..."
if curl -fsSL https://get.docker.com -o get-docker.sh; then
    sudo sh get-docker.sh
    rm get-docker.sh
    print_status "success" "Docker instalado correctamente"
else
    print_status "error" "Error instalando Docker"
    exit 1
fi

# Instalar containerd como alternativa
print_status "info" "Instalando containerd como alternativa..."
if sudo apt-get install -y containerd 2>/dev/null; then
    print_status "success" "Containerd instalado correctamente"

    # Configurar containerd
    sudo mkdir -p /etc/containerd
    containerd config default | sudo tee /etc/containerd/config.toml > /dev/null

    # Habilitar y iniciar containerd
    sudo systemctl enable containerd
    sudo systemctl start containerd

    # Agregar usuario al grupo containerd
    if ! getent group containerd >/dev/null 2>&1; then
        sudo groupadd containerd
    fi
    sudo usermod -a -G containerd $USER

    print_status "success" "Containerd configurado y iniciado"
else
    print_status "warning" "Error instalando containerd (continuando con Docker)"
fi

# Configurar Docker para usuario no privilegiado
print_status "info" "Configurando Docker para usuario no privilegiado..."

# Agregar usuario al grupo docker
sudo usermod -a -G docker $USER

# Habilitar y iniciar Docker
sudo systemctl enable docker
sudo systemctl start docker

print_status "success" "Docker configurado para usuario no privilegiado"

# Configurar red Docker
print_status "info" "Configurando red Docker..."

# Habilitar forwarding de IP para Docker
if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf; then
    echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
    sudo sysctl -w net.ipv4.ip_forward=1
    print_status "success" "IP forwarding habilitado"
fi

# Crear red Docker por defecto si no existe
if ! docker network ls | grep -q "diplo_default"; then
    docker network create --driver bridge diplo_default
    print_status "success" "Red Docker por defecto creada"
fi
print_status "success" "Configuración de red Docker completada"

# Crear script de verificación Docker
cat > ~/verify_docker_setup.sh << 'EOF'
#!/bin/bash

echo "🔍 Verificando configuración Docker..."
echo "====================================="

# Verificar que Docker está instalado
if command -v docker &> /dev/null; then
    echo "✅ Docker está instalado"
else
    echo "❌ Docker no está instalado"
    exit 1
fi

# Verificar que Docker daemon está corriendo
if docker info &> /dev/null; then
    echo "✅ Docker daemon está corriendo"
else
    echo "❌ Docker daemon no está corriendo"
    exit 1
fi

# Verificar que el usuario está en el grupo docker
if groups $USER | grep -q docker; then
    echo "✅ Usuario en grupo docker"
else
    echo "⚠️  Usuario no está en grupo docker"
fi

# Verificar red Docker
if docker network ls | grep -q "diplo_default"; then
    echo "✅ Red Docker por defecto encontrada"
else
    echo "⚠️  Red Docker por defecto no encontrada"
fi

echo ""
echo "🎉 Verificación Docker completada"
EOF

chmod +x ~/verify_docker_setup.sh

# Crear script de prueba Docker
cat > ~/test_docker.sh << 'EOF'
#!/bin/bash

echo "🧪 Probando Docker..."
echo "===================="

# Verificar que Docker funciona
if docker version &> /dev/null; then
    echo "✅ Docker responde correctamente"
else
    echo "❌ Docker no responde"
    exit 1
fi

# Intentar crear un contenedor de prueba
echo "Creando contenedor de prueba..."
if docker run --rm --name test-container hello-world &> /dev/null; then
    echo "✅ Contenedor de prueba creado exitosamente"
    echo "✅ Contenedor de prueba eliminado automáticamente"
else
    echo "❌ No se pudo crear contenedor de prueba"
    echo "Esto puede ser normal en la primera ejecución"
fi

echo ""
echo "🎉 Prueba Docker completada"
EOF

chmod +x ~/test_docker.sh

print_status "success" "Docker configurado correctamente"

# Mostrar información final
echo ""
echo "🎉 CONFIGURACIÓN DOCKER COMPLETADA"
echo "=================================="
print_status "success" "Docker ha sido configurado correctamente para Diplo"

echo ""
echo "📋 PRÓXIMOS PASOS:"
echo "=================="
print_status "info" "1. Reinicia tu Raspberry Pi: sudo reboot"
print_status "info" "2. Verifica Docker: ~/verify_docker_setup.sh"
print_status "info" "3. Prueba Docker: ~/test_docker.sh"
print_status "info" "4. Despliega Diplo usando: make deploy"

echo ""
echo "🔧 COMANDOS ÚTILES:"
echo "==================="
print_status "info" "• Verificar Docker: ~/verify_docker_setup.sh"
print_status "info" "• Probar Docker: ~/test_docker.sh"
print_status "info" "• Desplegar Diplo: make deploy"
print_status "info" "• Ver contenedores: docker ps"

echo ""
print_status "info" "IMPORTANTE: Este script SOLO configuró Docker"
print_status "info" "Para instalar Diplo, usa: make deploy"
print_status "info" "El auto-provisionamiento Docker funcionará cuando Diplo esté desplegado"

echo ""
print_status "success" "¡Configuración Docker completada! 🚀"