#!/bin/bash

# Script de instalación de containerd para Raspberry Pi
# Este script instala containerd y sus herramientas CLI

set -e

echo "🔧 Instalando containerd en Raspberry Pi..."
echo "==========================================="

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

print_status "info" "Iniciando instalación de containerd..."

# Actualizar sistema
print_status "info" "Actualizando sistema..."
if sudo apt-get update 2>/dev/null; then
    print_status "success" "Sistema actualizado"
else
    print_status "warning" "Error al actualizar repositorios (puede ser normal)"
    print_status "info" "Continuando con instalación..."
fi

# Instalar dependencias básicas
print_status "info" "Instalando dependencias básicas..."
if sudo apt-get install -y bridge-utils uidmap jq curl wget 2>/dev/null; then
    print_status "success" "Dependencias básicas instaladas"
else
    print_status "error" "Error instalando dependencias básicas"
    exit 1
fi

# Detener y deshabilitar containerd si ya está instalado
if systemctl is-active --quiet containerd; then
    print_status "info" "Deteniendo containerd existente..."
    sudo systemctl stop containerd
    sudo systemctl disable containerd
fi

# Desinstalar containerd existente si está instalado
if dpkg -l | grep -q containerd; then
    print_status "info" "Desinstalando containerd existente..."
    sudo apt-get remove -y containerd
    sudo apt-get autoremove -y
fi

# Instalar containerd desde los repositorios oficiales
print_status "info" "Instalando containerd desde repositorios oficiales..."

# Agregar repositorio oficial de containerd
if ! grep -q "deb https://download.docker.com/linux/debian" /etc/apt/sources.list.d/docker.list; then
    print_status "info" "Agregando repositorio Docker (contiene containerd)..."
    curl -fsSL https://download.docker.com/linux/debian/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    sudo apt-get update
fi

# Instalar containerd
if sudo apt-get install -y containerd.io 2>/dev/null; then
    print_status "success" "Containerd instalado correctamente"
else
    print_status "warning" "Error instalando containerd desde repositorio Docker"
    print_status "info" "Intentando instalación desde repositorios estándar..."
    
    if sudo apt-get install -y containerd 2>/dev/null; then
        print_status "success" "Containerd instalado desde repositorios estándar"
    else
        print_status "error" "No se pudo instalar containerd"
        exit 1
    fi
fi

# Configurar containerd
print_status "info" "Configurando containerd..."

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

# Modificar configuración para usar systemd cgroup driver
if grep -q "SystemdCgroup = false" /etc/containerd/config.toml; then
    sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
    print_status "success" "Configuración de cgroup actualizada"
fi

# Habilitar y iniciar containerd
print_status "info" "Iniciando containerd..."
sudo systemctl enable containerd
sudo systemctl start containerd

# Verificar que containerd está corriendo
if systemctl is-active --quiet containerd; then
    print_status "success" "Containerd está corriendo"
else
    print_status "error" "Containerd no está corriendo"
    print_status "info" "Verificando logs..."
    sudo journalctl -u containerd --no-pager -n 20
    exit 1
fi

# Instalar herramientas CLI de containerd
print_status "info" "Instalando herramientas CLI de containerd..."

# Crear directorio para herramientas
sudo mkdir -p /usr/local/bin

# Descargar e instalar ctr (CLI de containerd)
CTR_VERSION="1.7.0"
CTR_ARCH="arm64"
if [[ "$(uname -m)" == "x86_64" ]]; then
    CTR_ARCH="amd64"
fi

print_status "info" "Descargando ctr CLI (versión $CTR_VERSION)..."
if curl -L -o /tmp/ctr.tar.gz "https://github.com/containerd/containerd/releases/download/v${CTR_VERSION}/containerd-${CTR_VERSION}-linux-${CTR_ARCH}.tar.gz"; then
    cd /tmp
    sudo tar -xzf ctr.tar.gz bin/ctr
    sudo mv bin/ctr /usr/local/bin/
    sudo chmod +x /usr/local/bin/ctr
    rm -rf bin ctr.tar.gz
    print_status "success" "ctr CLI instalado correctamente"
else
    print_status "warning" "No se pudo descargar ctr CLI"
    print_status "info" "Intentando instalar desde repositorios..."
    
    # Intentar instalar desde repositorios
    if sudo apt-get install -y containerd-tools 2>/dev/null; then
        print_status "success" "Herramientas containerd instaladas desde repositorios"
    else
        print_status "warning" "No se pudieron instalar herramientas containerd"
        print_status "info" "Continuando sin herramientas CLI..."
    fi
fi

# Agregar usuario al grupo containerd
print_status "info" "Configurando permisos de usuario..."

# Crear grupo containerd si no existe
if ! getent group containerd >/dev/null 2>&1; then
    sudo groupadd containerd
fi

# Agregar usuario al grupo
sudo usermod -a -G containerd $USER

# Crear directorio de datos para containerd
sudo mkdir -p /var/lib/containerd
sudo chown root:containerd /var/lib/containerd
sudo chmod 775 /var/lib/containerd

print_status "success" "Permisos de usuario configurados"

# Crear script de verificación containerd
cat > ~/verify_containerd_setup.sh << 'EOF'
#!/bin/bash

echo "🔍 Verificando configuración containerd..."
echo "========================================="

# Verificar que containerd está instalado
if command -v containerd &> /dev/null; then
    echo "✅ Containerd está instalado"
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
else
    echo "⚠️  ctr CLI no está disponible"
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
else
    echo "❌ Configuración containerd no encontrada"
fi

echo ""
echo "🎉 Verificación containerd completada"
EOF

chmod +x ~/verify_containerd_setup.sh

# Crear script de prueba containerd
cat > ~/test_containerd.sh << 'EOF'
#!/bin/bash

echo "🧪 Probando containerd..."
echo "=========================="

# Verificar que containerd responde
if ctr version &> /dev/null; then
    echo "✅ Containerd responde correctamente"
else
    echo "❌ Containerd no responde"
    echo "Intentando con sudo..."
    if sudo ctr version &> /dev/null; then
        echo "✅ Containerd responde con sudo"
    else
        echo "❌ Containerd no responde ni con sudo"
        exit 1
    fi
fi

# Listar namespaces
echo "Listando namespaces..."
if ctr namespaces list &> /dev/null; then
    echo "✅ Namespaces listados correctamente"
else
    echo "⚠️  No se pudieron listar namespaces"
fi

echo ""
echo "🎉 Prueba containerd completada"
EOF

chmod +x ~/test_containerd.sh

print_status "success" "Containerd configurado correctamente"

# Mostrar información final
echo ""
echo "🎉 INSTALACIÓN CONTAINERD COMPLETADA"
echo "===================================="
print_status "success" "Containerd ha sido instalado y configurado correctamente"

echo ""
echo "📋 PRÓXIMOS PASOS:"
echo "=================="
print_status "info" "1. Reinicia tu Raspberry Pi: sudo reboot"
print_status "info" "2. Verifica containerd: ~/verify_containerd_setup.sh"
print_status "info" "3. Prueba containerd: ~/test_containerd.sh"
print_status "info" "4. Despliega Diplo usando: make deploy"

echo ""
echo "🔧 COMANDOS ÚTILES:"
echo "==================="
print_status "info" "• Verificar containerd: ~/verify_containerd_setup.sh"
print_status "info" "• Probar containerd: ~/test_containerd.sh"
print_status "info" "• Ver namespaces: ctr namespaces list"
print_status "info" "• Ver imágenes: ctr images list"

echo ""
print_status "info" "IMPORTANTE: Este script SOLO configuró containerd"
print_status "info" "Para instalar Diplo, usa: make deploy"
print_status "info" "El auto-provisionamiento containerd funcionará cuando Diplo esté desplegado"

echo ""
print_status "success" "¡Instalación containerd completada! 🚀" 