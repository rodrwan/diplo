#!/bin/bash

# Script de configuración post-deploy para Raspberry Pi
# Este script se ejecuta automáticamente después del deploy

set -e

echo "🍓 Configuración post-deploy para Raspberry Pi"
echo "=============================================="

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
    print_status "info" "Continuando con configuración genérica..."
fi

print_status "info" "Iniciando configuración post-deploy..."

# Verificar si containerd está disponible
if command -v containerd &> /dev/null && systemctl is-active --quiet containerd; then
    print_status "success" "Containerd ya está instalado y funcionando"
else
    print_status "info" "Containerd no está disponible, configurando..."
    
    # Verificar si tenemos el script de configuración
    if [ -f "./diplo-scripts/setup_raspberry_containerd.sh" ]; then
        print_status "info" "Ejecutando configuración de containerd..."
        cd ~/Mangoticket/diplo-scripts
        sudo ./setup_raspberry_containerd.sh
    else
        print_status "warning" "Script de configuración no encontrado"
        print_status "info" "Para configurar containerd manualmente:"
        echo "  sudo ./diplo-scripts/setup_raspberry_containerd.sh"
    fi
fi

# Verificar que Diplo puede ejecutarse
if [ -f "./diplo-rpi" ]; then
    print_status "info" "Verificando que Diplo puede ejecutarse..."
    chmod +x ./diplo-rpi
    
    # Verificar dependencias básicas
    if command -v sqlite3 &> /dev/null; then
        print_status "success" "SQLite3 está disponible"
    else
        print_status "warning" "SQLite3 no está instalado"
        print_status "info" "Instalando SQLite3..."
        sudo apt-get update
        sudo apt-get install -y sqlite3
    fi
else
    print_status "error" "Binario de Diplo no encontrado"
    exit 1
fi

# Crear directorio de datos si no existe
if [ ! -d "./data" ]; then
    print_status "info" "Creando directorio de datos..."
    mkdir -p ./data
fi

# Verificar permisos de red
print_status "info" "Verificando configuración de red..."

# Verificar que el puerto 8080 esté disponible
if netstat -tuln | grep -q ":8080 "; then
    print_status "warning" "Puerto 8080 ya está en uso"
    print_status "info" "Verificando qué está usando el puerto..."
    netstat -tuln | grep ":8080"
else
    print_status "success" "Puerto 8080 está disponible"
fi

# Crear script de inicio automático
print_status "info" "Configurando inicio automático..."

cat > ~/start_diplo.sh << 'EOF'
#!/bin/bash

cd ~/Mangoticket

# Verificar que containerd esté corriendo
if ! systemctl is-active --quiet containerd; then
    echo "⚠️  Containerd no está corriendo, iniciando..."
    sudo systemctl start containerd
fi

# Verificar que el puerto esté libre
if netstat -tuln | grep -q ":8080 "; then
    echo "⚠️  Puerto 8080 en uso, deteniendo proceso anterior..."
    sudo pkill -f diplo-rpi || true
    sleep 2
fi

# Iniciar Diplo
echo "🚀 Iniciando Diplo..."
./diplo-rpi
EOF

chmod +x ~/start_diplo.sh

# Crear servicio systemd para inicio automático (opcional)
read -p "¿Deseas configurar inicio automático de Diplo? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    print_status "info" "Configurando servicio systemd..."
    
    sudo tee /etc/systemd/system/diplo.service > /dev/null <<EOF
[Unit]
Description=Diplo PaaS
After=network.target containerd.service

[Service]
Type=simple
User=mango
WorkingDirectory=/home/mango/Mangoticket
ExecStart=/home/mango/Mangoticket/diplo-rpi
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable diplo.service
    
    print_status "success" "Servicio systemd configurado"
    print_status "info" "Para iniciar: sudo systemctl start diplo"
    print_status "info" "Para ver estado: sudo systemctl status diplo"
fi

# Mostrar información final
echo ""
echo "🎉 CONFIGURACIÓN POST-DEPLOY COMPLETADA"
echo "======================================="
print_status "success" "Diplo está listo para usar en tu Raspberry Pi"

echo ""
echo "📋 COMANDOS ÚTILES:"
echo "==================="
echo "  Iniciar Diplo: ./start_diplo.sh"
echo "  Verificar containerd: ./diplo-scripts/diagnose_containerd.sh"
echo "  Ver logs: sudo journalctl -u containerd -f"
echo "  Verificar estado: sudo systemctl status containerd"

echo ""
echo "🌐 ACCESO WEB:"
echo "=============="
echo "  Una vez iniciado, accede a: http://raspberrypi.local:8080"

echo ""
echo "📚 DOCUMENTACIÓN:"
echo "================"
echo "  Guía completa: cat ~/Mangoticket/docs/RASPBERRY_PI_SETUP.md"

echo ""
print_status "success" "¡Tu Raspberry Pi está configurado y listo!" 