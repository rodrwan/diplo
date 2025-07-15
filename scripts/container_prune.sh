#!/bin/bash
# Script de limpieza agresiva de containerd para Raspberry Pi
# Basado en las mejores prácticas para sistemas ARM

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
    echo -e "${BLUE}  LIMPIEZA AGRESIVA CONTAINERD${NC}"
    echo -e "${BLUE}================================${NC}"
    echo
}

# Verificar que ctr esté disponible
check_ctr() {
    if ! command -v ctr >/dev/null 2>&1; then
        print_status "error" "ctr no está disponible"
        exit 1
    fi
}

# Verificar namespace diplo
check_namespace() {
    if ! ctr namespaces list 2>/dev/null | grep -q "diplo"; then
        print_status "warning" "Namespace diplo no existe, creando..."
        ctr namespaces create diplo
    fi
}

# Función principal de limpieza
main() {
    print_header
    print_status "info" "Iniciando limpieza agresiva de containerd..."
    echo

    # Verificaciones previas
    check_ctr
    check_namespace

    # 1. Detener todas las tareas con SIGKILL
    print_status "info" "🔪 Deteniendo todas las tareas con SIGKILL..."
    sudo ctr -n diplo tasks ls | awk 'NR>1 {print $1}' | while read task; do
        echo "   Deteniendo tarea: $task"
        sudo ctr -n diplo tasks kill --signal SIGKILL "$task" 2>/dev/null || true
    done

    # 2. Matar procesos containerd-shim relacionados
    print_status "info" "🔪 Matando procesos containerd-shim..."
    sudo pkill -f "containerd-shim" 2>/dev/null || true

    # 3. Esperar a que se detengan completamente
    print_status "info" "⏳ Esperando a que se detengan..."
    sleep 5

    # 4. Verificar que las tareas se detuvieron
    print_status "info" "📋 Verificando tareas..."
    sudo ctr -n diplo tasks ls 2>/dev/null || print_status "warning" "No hay tareas activas"

    # 5. Eliminar contenedores uno por uno
    print_status "info" "🗑️  Eliminando contenedores..."
    sudo ctr -n diplo containers ls | awk 'NR>1 {print $1}' | while read container; do
        echo "   Eliminando contenedor: $container"
        sudo ctr -n diplo containers rm "$container" 2>/dev/null || true
    done

    # 6. Eliminar imágenes
    print_status "info" "🖼️  Eliminando imágenes..."
    sudo ctr -n diplo images ls | awk 'NR>1 {print $1}' | while read image; do
        echo "   Eliminando imagen: $image"
        sudo ctr -n diplo images rm "$image" 2>/dev/null || true
    done

    # 7. Limpiar snapshots
    print_status "info" "🧽 Limpiando snapshots..."
    sudo ctr -n diplo snapshots prune 2>/dev/null || true

    # 8. Limpieza adicional para Raspberry Pi
    print_status "info" "🍓 Limpieza específica para Raspberry Pi..."

    # Matar cualquier proceso containerd restante
    sudo pkill -f "containerd" 2>/dev/null || true
    sleep 2

    # Reiniciar containerd si es necesario
    if ! systemctl is-active --quiet containerd; then
        print_status "warning" "Containerd no está activo, reiniciando..."
        sudo systemctl restart containerd
        sleep 3
    fi

    # 9. Verificar resultado final
    print_status "info" "📋 Estado final:"
    echo "   Tareas:"
    sudo ctr -n diplo tasks ls 2>/dev/null || echo "   No hay tareas"
    echo "   Contenedores:"
    sudo ctr -n diplo containers ls 2>/dev/null || echo "   No hay contenedores"
    echo "   Imágenes:"
    sudo ctr -n diplo images ls 2>/dev/null || echo "   No hay imágenes"

    print_status "success" "Limpieza agresiva completada!"
    echo
    print_status "info" "Para verificar el estado:"
    print_status "info" "- sudo ctr -n diplo tasks ls"
    print_status "info" "- sudo ctr -n diplo containers ls"
    print_status "info" "- sudo systemctl status containerd"
}

# Ejecutar función principal
main "$@"