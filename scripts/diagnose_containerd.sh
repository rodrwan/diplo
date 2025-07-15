#!/bin/bash

# Script de diagnóstico para problemas de containerd en Diplo
# Este script ayuda a identificar y resolver problemas comunes con contenedores containerd

set -e

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Función para imprimir mensajes con colores
print_status() {
    local level=$1
    local message=$2
    case $level in
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

print_header() {
    echo -e "${BLUE}================================${NC}"
    echo -e "${BLUE}  Diagnóstico de Containerd${NC}"
    echo -e "${BLUE}================================${NC}"
    echo
}

# Verificar si ctr está disponible
check_ctr() {
    print_status "info" "Verificando disponibilidad de ctr..."
    if command -v ctr >/dev/null 2>&1; then
        print_status "success" "ctr está disponible"
        CTR_VERSION=$(ctr version 2>/dev/null | head -n 1 || echo "versión desconocida")
        print_status "info" "Versión: $CTR_VERSION"
    else
        print_status "error" "ctr no está disponible"
        print_status "info" "Instala containerd: sudo ./scripts/install_containerd.sh"
        return 1
    fi
}

# Verificar estado del daemon de containerd
check_containerd_daemon() {
    print_status "info" "Verificando estado del daemon de containerd..."
    
    if systemctl is-active --quiet containerd; then
        print_status "success" "Daemon de containerd está activo"
    else
        print_status "warning" "Daemon de containerd no está activo"
        print_status "info" "Intentando iniciar containerd..."
        sudo systemctl start containerd
        sleep 2
        if systemctl is-active --quiet containerd; then
            print_status "success" "Containerd iniciado exitosamente"
        else
            print_status "error" "No se pudo iniciar containerd"
            return 1
        fi
    fi
}

# Verificar namespace diplo
check_diplo_namespace() {
    print_status "info" "Verificando namespace diplo..."
    
    if ctr namespaces list 2>/dev/null | grep -q "diplo"; then
        print_status "success" "Namespace diplo existe"
    else
        print_status "warning" "Namespace diplo no existe"
        print_status "info" "Creando namespace diplo..."
        ctr namespaces create diplo
        print_status "success" "Namespace diplo creado"
    fi
}

# Listar contenedores en namespace diplo
list_containers() {
    print_status "info" "Listando contenedores en namespace diplo..."
    
    echo
    echo "Contenedores:"
    ctr -n diplo containers list 2>/dev/null || {
        print_status "warning" "No se pudieron listar contenedores"
        return 1
    }
    
    echo
    echo "Tareas:"
    ctr -n diplo tasks list 2>/dev/null || {
        print_status "warning" "No se pudieron listar tareas"
        return 1
    }
}

# Verificar contenedores huérfanos
check_orphaned_containers() {
    print_status "info" "Verificando contenedores huérfanos..."
    
    local containers=$(ctr -n diplo containers list 2>/dev/null | grep "diplo-" | wc -l)
    local tasks=$(ctr -n diplo tasks list 2>/dev/null | grep "diplo-" | wc -l)
    
    if [ "$containers" -gt 0 ]; then
        print_status "info" "Encontrados $containers contenedores"
        print_status "info" "Encontradas $tasks tareas"
        
        if [ "$containers" -gt "$tasks" ]; then
            print_status "warning" "Posibles contenedores huérfanos detectados"
            print_status "info" "Ejecuta: curl -X POST http://localhost:8080/api/v1/maintenance/cleanup-orphaned-containers"
        fi
    else
        print_status "success" "No se encontraron contenedores huérfanos"
    fi
}

# Limpiar contenedores huérfanos manualmente
cleanup_orphaned_containers() {
    print_status "info" "Limpiando contenedores huérfanos manualmente..."
    
    # Listar contenedores
    local containers=$(ctr -n diplo containers list 2>/dev/null | grep "diplo-" | awk '{print $1}')
    
    if [ -z "$containers" ]; then
        print_status "success" "No hay contenedores para limpiar"
        return 0
    fi
    
    local cleaned=0
    for container in $containers; do
        print_status "info" "Procesando contenedor: $container"
        
        # Verificar si hay tarea asociada
        if ! ctr -n diplo tasks list 2>/dev/null | grep -q "$container"; then
            print_status "warning" "Contenedor huérfano detectado: $container"
            
            # Intentar eliminar contenedor
            if ctr -n diplo containers delete "$container" 2>/dev/null; then
                print_status "success" "Contenedor eliminado: $container"
                ((cleaned++))
            else
                print_status "error" "No se pudo eliminar contenedor: $container"
            fi
        else
            print_status "info" "Contenedor tiene tarea asociada: $container"
        fi
    done
    
    if [ $cleaned -gt 0 ]; then
        print_status "success" "Limpieza completada: $cleaned contenedores eliminados"
    else
        print_status "info" "No se eliminaron contenedores"
    fi
}

# Verificar permisos y configuración
check_permissions() {
    print_status "info" "Verificando permisos..."
    
    # Verificar socket de containerd
    if [ -S "/run/containerd/containerd.sock" ]; then
        print_status "success" "Socket de containerd encontrado"
        
        # Verificar permisos del socket
        local socket_perms=$(stat -c "%a" /run/containerd/containerd.sock)
        if [ "$socket_perms" = "666" ] || [ "$socket_perms" = "660" ]; then
            print_status "success" "Permisos del socket correctos: $socket_perms"
        else
            print_status "warning" "Permisos del socket pueden ser restrictivos: $socket_perms"
        fi
    else
        print_status "error" "Socket de containerd no encontrado"
        return 1
    fi
    
    # Verificar si el usuario está en el grupo docker
    if groups $USER | grep -q docker; then
        print_status "success" "Usuario está en grupo docker"
    else
        print_status "warning" "Usuario no está en grupo docker"
        print_status "info" "Ejecuta: sudo usermod -aG docker $USER"
    fi
}

# Función principal
main() {
    print_header
    
    local step=1
    local total_steps=6
    
    print_status "info" "Iniciando diagnóstico de containerd..."
    echo
    
    # Paso 1: Verificar ctr
    print_status "info" "[$step/$total_steps] Verificando ctr..."
    if check_ctr; then
        print_status "success" "✅ ctr verificado"
    else
        print_status "error" "❌ Problema con ctr"
        return 1
    fi
    echo
    ((step++))
    
    # Paso 2: Verificar daemon
    print_status "info" "[$step/$total_steps] Verificando daemon..."
    if check_containerd_daemon; then
        print_status "success" "✅ Daemon verificado"
    else
        print_status "error" "❌ Problema con daemon"
        return 1
    fi
    echo
    ((step++))
    
    # Paso 3: Verificar namespace
    print_status "info" "[$step/$total_steps] Verificando namespace..."
    if check_diplo_namespace; then
        print_status "success" "✅ Namespace verificado"
    else
        print_status "error" "❌ Problema con namespace"
        return 1
    fi
    echo
    ((step++))
    
    # Paso 4: Verificar permisos
    print_status "info" "[$step/$total_steps] Verificando permisos..."
    if check_permissions; then
        print_status "success" "✅ Permisos verificados"
    else
        print_status "warning" "⚠️  Problemas con permisos"
    fi
    echo
    ((step++))
    
    # Paso 5: Listar contenedores
    print_status "info" "[$step/$total_steps] Listando contenedores..."
    if list_containers; then
        print_status "success" "✅ Contenedores listados"
    else
        print_status "warning" "⚠️  Problemas listando contenedores"
    fi
    echo
    ((step++))
    
    # Paso 6: Verificar contenedores huérfanos
    print_status "info" "[$step/$total_steps] Verificando contenedores huérfanos..."
    check_orphaned_containers
    echo
    
    print_status "success" "Diagnóstico completado"
    echo
    
    # Preguntar si quiere limpiar contenedores huérfanos
    read -p "¿Deseas limpiar contenedores huérfanos manualmente? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        cleanup_orphaned_containers
    fi
    
    echo
    print_status "info" "Para más información, consulta:"
    print_status "info" "- Logs de containerd: sudo journalctl -u containerd"
    print_status "info" "- API de limpieza: curl -X POST http://localhost:8080/api/v1/maintenance/cleanup-orphaned-containers"
    print_status "info" "- Script de instalación: sudo ./scripts/install_containerd.sh"
}

# Ejecutar función principal
main "$@" 