#!/bin/bash

# Script para probar la API de Diplo
# Uso: ./scripts/test_api.sh

BASE_URL="http://localhost:8080"
APP_ID=""

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Función para imprimir con colores
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Función para hacer requests HTTP
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    print_status "$description"
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$BASE_URL$endpoint")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            "$BASE_URL$endpoint")
    fi
    
    # Separar respuesta y código HTTP
    http_code=$(echo "$response" | tail -n1)
    response_body=$(echo "$response" | head -n -1)
    
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_success "Request exitoso (HTTP $http_code)"
        echo "$response_body" | jq '.' 2>/dev/null || echo "$response_body"
        echo
    else
        print_error "Request falló (HTTP $http_code)"
        echo "$response_body"
        echo
    fi
}

# Verificar que curl esté instalado
if ! command -v curl &> /dev/null; then
    print_error "curl no está instalado. Por favor instálalo primero."
    exit 1
fi

# Verificar que jq esté instalado (opcional, para formatear JSON)
if ! command -v jq &> /dev/null; then
    print_warning "jq no está instalado. Las respuestas JSON no se formatearán."
fi

print_status "Iniciando pruebas de la API de Diplo..."
echo

# 1. Health Check
make_request "GET" "/" "" "Verificando que el servidor esté funcionando"

# 2. Deploy una aplicación Go de ejemplo
print_status "Desplegando aplicación Go de ejemplo..."
deploy_response=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d '{
        "repo_url": "https://github.com/gin-gonic/examples",
        "name": "gin-example"
    }' \
    "$BASE_URL/deploy")

# Extraer app_id de la respuesta
APP_ID=$(echo "$deploy_response" | jq -r '.id' 2>/dev/null)
if [ "$APP_ID" = "null" ] || [ -z "$APP_ID" ]; then
    print_error "No se pudo obtener el ID de la aplicación"
    echo "Respuesta completa:"
    echo "$deploy_response"
    exit 1
fi

print_success "Aplicación desplegada con ID: $APP_ID"

# 3. Obtener todas las aplicaciones
make_request "GET" "/apps" "" "Obteniendo todas las aplicaciones"

# 4. Obtener aplicación específica
make_request "GET" "/apps/$APP_ID" "" "Obteniendo detalles de la aplicación específica"

# 5. Esperar un poco para que el deployment termine
print_status "Esperando 10 segundos para que el deployment termine..."
sleep 10

# 6. Verificar estado de la aplicación
make_request "GET" "/apps/$APP_ID" "" "Verificando estado final de la aplicación"

# 7. Eliminar la aplicación
print_status "Eliminando la aplicación de prueba..."
make_request "DELETE" "/apps/$APP_ID" "" "Eliminando aplicación"

# 8. Verificar que se eliminó
make_request "GET" "/apps" "" "Verificando que la aplicación se eliminó correctamente"

print_success "Todas las pruebas completadas exitosamente!"
echo
print_status "Para probar con diferentes lenguajes, puedes usar estos ejemplos:"
echo
echo "Node.js:"
echo 'curl -X POST http://localhost:8080/deploy \'
echo '  -H "Content-Type: application/json" \'
echo '  -d '"'"'{"repo_url": "https://github.com/expressjs/express", "name": "express-example"}'"'"''
echo
echo "Python:"
echo 'curl -X POST http://localhost:8080/deploy \'
echo '  -H "Content-Type: application/json" \'
echo '  -d '"'"'{"repo_url": "https://github.com/pallets/flask", "name": "flask-example"}'"'"''
echo 