#!/bin/bash

# Script de gestión para Diplo en Raspberry Pi
# Usa los scripts copiados por make deploy

set -e

echo "🔧 Gestión de Diplo en Raspberry Pi"
echo "==================================="

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

# Verificar conexión SSH
check_ssh() {
    if ! ssh -o ConnectTimeout=5 mango@raspberrypi.local "echo 'OK'" 2>/dev/null; then
        print_status "error" "No se puede conectar a Raspberry Pi"
        exit 1
    fi
}

case "$1" in
    "start")
        print_status "info" "Iniciando Diplo..."
        check_ssh
        ssh mango@raspberrypi.local "cd ~/Mangoticket && ./diplo-rpi"
        ;;
    "stop")
        print_status "info" "Deteniendo Diplo..."
        check_ssh
        ssh mango@raspberrypi.local "pkill -f diplo-rpi"
        print_status "success" "Diplo detenido"
        ;;
    "restart")
        print_status "info" "Reiniciando Diplo..."
        check_ssh
        ssh mango@raspberrypi.local "pkill -f diplo-rpi; sleep 2; cd ~/Mangoticket && ./diplo-rpi &"
        print_status "success" "Diplo reiniciado"
        ;;
    "status")
        print_status "info" "Estado de Diplo..."
        check_ssh
        ssh mango@raspberrypi.local "ps aux | grep diplo-rpi | grep -v grep"
        ;;
    "logs")
        print_status "info" "Logs de Diplo..."
        check_ssh
        ssh mango@raspberrypi.local "tail -f ~/Mangoticket/diplo.log"
        ;;
    "deploy")
        print_status "info" "Desplegando nueva versión..."
        make deploy
        print_status "success" "Deploy completado"
        ;;
    "deploy-full")
        print_status "info" "Deploy completo (Diplo + Docker)..."
        make deploy-full
        print_status "success" "Deploy completo completado"
        ;;
    "verify")
        print_status "info" "Verificando instalación..."
        check_ssh
        ssh mango@raspberrypi.local "~/diplo-scripts/verify_docker_setup.sh"
        ;;
    "test")
        print_status "info" "Probando sistema..."
        check_ssh
        ssh mango@raspberrypi.local "~/diplo-scripts/test_docker.sh"
        ;;
    "setup-docker")
        print_status "info" "Configurando Docker..."
        check_ssh
        ssh mango@raspberrypi.local "cd ~/diplo-scripts && ./install_diplo_raspberry.sh"
        print_status "success" "Docker configurado"
        ;;
    "list")
        print_status "info" "Listando contenedores..."
        check_ssh
        ssh mango@raspberrypi.local "docker ps"
        ;;
    "cleanup")
        print_status "info" "Limpiando contenedores de prueba..."
        check_ssh
        ssh mango@raspberrypi.local "docker rm -f test-container 2>/dev/null || true"
        print_status "success" "Limpieza completada"
        ;;
    "health")
        print_status "info" "Verificando salud del sistema..."
        check_ssh
        
        # Verificar binario
        if ssh mango@raspberrypi.local "test -f ~/Mangoticket/diplo-rpi"; then
            print_status "success" "✅ Binario Diplo encontrado"
        else
            print_status "error" "❌ Binario Diplo no encontrado"
        fi
        
        # Verificar scripts
        if ssh mango@raspberrypi.local "test -d ~/diplo-scripts"; then
            print_status "success" "✅ Scripts de gestión encontrados"
        else
            print_status "warning" "⚠️  Scripts de gestión no encontrados"
        fi
        
        # Verificar proceso
        if ssh mango@raspberrypi.local "pgrep -f diplo-rpi" 2>/dev/null; then
            print_status "success" "✅ Diplo está ejecutándose"
        else
            print_status "warning" "⚠️  Diplo no está ejecutándose"
        fi
        
        # Verificar Docker
        if ssh mango@raspberrypi.local "command -v docker" 2>/dev/null; then
            print_status "success" "✅ Docker está instalado"
        else
            print_status "warning" "⚠️  Docker no está instalado"
        fi
        ;;
    *)
        echo "Uso: $0 {comando}"
        echo ""
        echo "Comandos de Diplo:"
        echo "  start       - Iniciar Diplo"
        echo "  stop        - Detener Diplo"
        echo "  restart     - Reiniciar Diplo"
        echo "  status      - Ver estado de Diplo"
        echo "  logs        - Ver logs de Diplo"
        echo ""
        echo "Comandos de Deploy:"
        echo "  deploy      - Desplegar nueva versión"
        echo "  deploy-full - Deploy completo (Diplo + Docker)"
        echo ""
        echo "Comandos de Docker:"
        echo "  setup-docker   - Configurar Docker"
        echo "  verify      - Verificar configuración Docker"
        echo "  test        - Probar sistema"
        echo "  list        - Listar contenedores"
        echo "  cleanup     - Limpiar contenedores"
        echo ""
        echo "Comandos de Diagnóstico:"
        echo "  health      - Verificar salud del sistema"
        ;;
esac 