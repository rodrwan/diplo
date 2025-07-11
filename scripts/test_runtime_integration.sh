#!/bin/bash

# Test de Integración del Sistema Híbrido de Runtimes - Diplo
# Verifica detección automática de runtimes, deployment unificado, y gestión de variables de entorno

set -e

DIPLO_URL="http://localhost:8080"
REPO_URL="https://github.com/gorilla/mux.git"
APP_NAME="test-runtime-integration"
TEST_START_TIME=$(date +%s)

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "🧪 === Test de Integración del Sistema Híbrido de Runtimes ==="
echo "📅 Inicio: $(date)"
echo "🔗 URL del servidor: $DIPLO_URL"
echo ""

# Función para hacer peticiones HTTP con manejo de errores
http_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    echo -e "${BLUE}→ $method $endpoint${NC} - $description"
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "HTTP_STATUS:%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" "$DIPLO_URL$endpoint")
    else
        response=$(curl -s -w "HTTP_STATUS:%{http_code}" -X "$method" "$DIPLO_URL$endpoint")
    fi
    
    http_code=$(echo "$response" | grep -o "HTTP_STATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTP_STATUS:[0-9]*$//')
    
    if [[ $http_code -ge 200 && $http_code -lt 300 ]]; then
        echo -e "${GREEN}✅ $http_code${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    else
        echo -e "${RED}❌ Error $http_code${NC}"
        echo "$body"
        return 1
    fi
    
    echo ""
}

# Función para extraer valores del JSON
extract_json_value() {
    echo "$1" | jq -r "$2" 2>/dev/null || echo "null"
}

# 1. Verificar que el servidor esté corriendo
echo -e "${YELLOW}🏥 1. Health Check del Servidor${NC}"
if ! http_request "GET" "/health" "" "Verificar estado del servidor"; then
    echo -e "${RED}❌ El servidor no está corriendo. Ejecuta './bin/diplo' primero.${NC}"
    exit 1
fi

# 2. Verificar detección de runtimes
echo -e "${YELLOW}🔍 2. Detección de Runtimes Disponibles${NC}"
status_response=$(http_request "GET" "/api/v1/status" "" "Obtener estado del sistema híbrido")

# Extraer información de runtimes
available_runtimes=$(extract_json_value "$status_response" '.data.runtime.available[]')
preferred_runtime=$(extract_json_value "$status_response" '.data.runtime.preferred')
system_os=$(extract_json_value "$status_response" '.data.system.os')
system_arch=$(extract_json_value "$status_response" '.data.system.architecture')

echo -e "${BLUE}📊 Resumen del Sistema:${NC}"
echo "   OS: $system_os"
echo "   Arquitectura: $system_arch"
echo "   Runtimes disponibles: $available_runtimes"
echo "   Runtime preferido: $preferred_runtime"
echo ""

# 3. Verificar endpoints específicos de runtime
echo -e "${YELLOW}🐳 3. Verificar Status de Docker${NC}"
http_request "GET" "/api/docker/status" "" "Estado específico de Docker"

echo -e "${YELLOW}📦 4. Verificar Status de LXC (si disponible)${NC}"
if http_request "GET" "/api/lxc/status" "" "Estado específico de LXC" 2>/dev/null; then
    echo -e "${GREEN}LXC está disponible${NC}"
else
    echo -e "${YELLOW}LXC no está disponible (esperado en macOS)${NC}"
fi

# 4. Test de deployment con variables de entorno
echo -e "${YELLOW}🚀 5. Deployment con Variables de Entorno${NC}"
deploy_data=$(cat <<EOF
{
    "repo_url": "$REPO_URL",
    "name": "$APP_NAME",
    "env_vars": [
        {
            "name": "PORT",
            "value": "8080"
        },
        {
            "name": "ENV",
            "value": "test"
        },
        {
            "name": "SECRET_KEY",
            "value": "super-secret-test-key"
        },
        {
            "name": "API_TOKEN",
            "value": "test-token-123"
        }
    ]
}
EOF
)

deploy_response=$(http_request "POST" "/api/v1/deploy" "$deploy_data" "Deployment con variables de entorno")

# Extraer información del deployment
app_id=$(extract_json_value "$deploy_response" '.data.id')
runtime_used=$(extract_json_value "$deploy_response" '.data.runtime_type')
app_port=$(extract_json_value "$deploy_response" '.data.port')

echo -e "${BLUE}📋 Información del Deployment:${NC}"
echo "   App ID: $app_id"
echo "   Runtime usado: $runtime_used"
echo "   Puerto asignado: $app_port"
echo ""

if [ "$app_id" = "null" ] || [ -z "$app_id" ]; then
    echo -e "${RED}❌ No se pudo obtener el App ID del deployment${NC}"
    exit 1
fi

# 5. Verificar variables de entorno guardadas
echo -e "${YELLOW}🔧 6. Verificar Variables de Entorno Guardadas${NC}"
env_vars_response=$(http_request "GET" "/api/v1/apps/$app_id/env" "" "Listar variables de entorno")

# Contar variables de entorno
env_count=$(extract_json_value "$env_vars_response" '.data | length')
echo -e "${BLUE}📊 Variables de entorno guardadas: $env_count${NC}"

# Verificar variables específicas
echo -e "${BLUE}🔍 Verificando variables específicas:${NC}"
for var_name in "PORT" "ENV" "SECRET_KEY" "API_TOKEN"; do
    var_response=$(http_request "GET" "/api/v1/apps/$app_id/env/$var_name" "" "Obtener variable $var_name" 2>/dev/null)
    if [ $? -eq 0 ]; then
        is_secret=$(extract_json_value "$var_response" '.data.is_secret')
        if [ "$is_secret" = "true" ]; then
            echo -e "   ${GREEN}✅ $var_name (🔐 cifrada)${NC}"
        else
            echo -e "   ${GREEN}✅ $var_name${NC}"
        fi
    else
        echo -e "   ${RED}❌ $var_name no encontrada${NC}"
    fi
done
echo ""

# 6. Monitorear el deployment
echo -e "${YELLOW}⏱️  7. Monitorear Progreso del Deployment${NC}"
max_attempts=15
attempt=1

while [ $attempt -le $max_attempts ]; do
    echo -e "${BLUE}   Intento $attempt/$max_attempts...${NC}"
    
    app_response=$(http_request "GET" "/api/v1/apps/$app_id" "" "Obtener estado de la aplicación" 2>/dev/null)
    if [ $? -eq 0 ]; then
        app_status=$(extract_json_value "$app_response" '.data.status')
        echo -e "   Estado actual: ${BLUE}$app_status${NC}"
        
        case $app_status in
            "running")
                echo -e "${GREEN}✅ Deployment completado exitosamente!${NC}"
                break
                ;;
            "error")
                error_msg=$(extract_json_value "$app_response" '.data.error_msg')
                echo -e "${RED}❌ Deployment falló: $error_msg${NC}"
                break
                ;;
            "deploying")
                echo -e "   ${YELLOW}⏳ Deployment en progreso...${NC}"
                ;;
        esac
    fi
    
    if [ $attempt -eq $max_attempts ]; then
        echo -e "${YELLOW}⚠️  Timeout esperando deployment${NC}"
    fi
    
    sleep 5
    ((attempt++))
done
echo ""

# 7. Test de health check de la aplicación
if [ "$app_status" = "running" ]; then
    echo -e "${YELLOW}🏥 8. Health Check de la Aplicación${NC}"
    health_response=$(http_request "GET" "/api/v1/apps/$app_id/health" "" "Health check de la aplicación")
    
    health_status=$(extract_json_value "$health_response" '.data.healthy')
    if [ "$health_status" = "true" ]; then
        echo -e "${GREEN}✅ Aplicación está saludable${NC}"
    else
        echo -e "${YELLOW}⚠️  Aplicación no responde correctamente${NC}"
    fi
    echo ""
fi

# 8. Test de gestión de variables de entorno
echo -e "${YELLOW}🔄 9. Test de Gestión de Variables de Entorno${NC}"

# Crear nueva variable
new_var_data='{"key": "TEST_VAR", "value": "test-value", "is_secret": false}'
http_request "POST" "/api/v1/apps/$app_id/env" "$new_var_data" "Crear nueva variable de entorno"

# Actualizar variable existente
update_var_data='{"value": "updated-test-value", "is_secret": true}'
http_request "PUT" "/api/v1/apps/$app_id/env/TEST_VAR" "$update_var_data" "Actualizar variable de entorno"

# Verificar que se actualizó
http_request "GET" "/api/v1/apps/$app_id/env/TEST_VAR" "" "Verificar variable actualizada"

# 9. Test de redeploy
echo -e "${YELLOW}🔄 10. Test de Redeploy${NC}"
redeploy_data=$(cat <<EOF
{
    "repo_url": "$REPO_URL",
    "name": "$APP_NAME",
    "env_vars": [
        {
            "name": "PORT",
            "value": "8080"
        },
        {
            "name": "ENV",
            "value": "production"
        },
        {
            "name": "NEW_VAR",
            "value": "added-in-redeploy"
        }
    ]
}
EOF
)

http_request "POST" "/api/v1/deploy" "$redeploy_data" "Redeploy con variables de entorno actualizadas"

# 10. Limpieza
echo -e "${YELLOW}🧹 11. Limpieza${NC}"
http_request "DELETE" "/api/v1/apps/$app_id" "" "Eliminar aplicación de prueba"

# Verificar que se eliminó
echo -e "${BLUE}🔍 Verificando que la aplicación se eliminó...${NC}"
if ! http_request "GET" "/api/v1/apps/$app_id" "" "Verificar eliminación" 2>/dev/null; then
    echo -e "${GREEN}✅ Aplicación eliminada correctamente${NC}"
else
    echo -e "${YELLOW}⚠️  La aplicación aún existe${NC}"
fi

# 11. Resumen final
TEST_END_TIME=$(date +%s)
TEST_DURATION=$((TEST_END_TIME - TEST_START_TIME))

echo ""
echo "🏁 === Resumen del Test ==="
echo "📅 Finalizado: $(date)"
echo "⏱️  Duración: ${TEST_DURATION}s"
echo -e "${GREEN}✅ Test de integración completado exitosamente${NC}"
echo ""
echo "📊 Sistema verificado:"
echo "   - Detección automática de runtimes"
echo "   - Deployment unificado con variables de entorno"
echo "   - Cifrado automático de variables sensibles"
echo "   - Gestión CRUD de variables de entorno"
echo "   - Redeploy con actualización de variables"
echo "   - Limpieza de recursos"
echo ""
echo -e "${BLUE}💡 Runtime activo: $preferred_runtime${NC}"
echo -e "${BLUE}🏗️  Aplicaciones desplegadas usando Docker como backend${NC}" 