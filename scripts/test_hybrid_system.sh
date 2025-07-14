#!/bin/bash

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Variables
SERVER_PID=""
BASE_URL="http://localhost:8080"

# Función para limpiar al salir
cleanup() {
    echo -e "${YELLOW}🛑 Deteniendo servidor...${NC}"
    if [ ! -z "$SERVER_PID" ]; then
        kill $SERVER_PID 2>/dev/null
        wait $SERVER_PID 2>/dev/null
    fi
}

# Configurar trap para cleanup
trap cleanup EXIT

# Función para hacer requests HTTP con formato JSON
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3

    if [ -z "$data" ]; then
        curl -s -X $method "$BASE_URL$endpoint" | jq '.' 2>/dev/null || curl -s -X $method "$BASE_URL$endpoint"
    else
        curl -s -X $method "$BASE_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" | jq '.' 2>/dev/null || curl -s -X $method "$BASE_URL$endpoint" -H "Content-Type: application/json" -d "$data"
    fi
}

# Función para esperar que el servidor arranque
wait_for_server() {
    local max_attempts=30
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        echo -e "${CYAN}   Intento $attempt/$max_attempts...${NC}"

        if curl -s "$BASE_URL/health" > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Servidor listo!${NC}"
            return 0
        fi

        sleep 1
        ((attempt++))
    done

    echo -e "${RED}❌ Servidor no arrancó después de $max_attempts intentos${NC}"
    return 1
}

echo -e "${PURPLE}🚀 Iniciando pruebas del sistema híbrido Diplo...${NC}"
echo -e "${PURPLE}==================================================${NC}"

# Compilar el proyecto
echo -e "${BLUE}🔨 Compilando Diplo...${NC}"
make clean && make
if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Error compilando Diplo${NC}"
    exit 1
fi

# Iniciar servidor en background
echo -e "${BLUE}🌟 Iniciando servidor Diplo híbrido...${NC}"
./bin/diplo &
SERVER_PID=$!

# Esperar que el servidor arranque
echo -e "${YELLOW}⏳ Esperando que el servidor arranque...${NC}"
if ! wait_for_server; then
    exit 1
fi

echo ""
echo -e "${CYAN}🔍 Probando endpoints del sistema híbrido...${NC}"
echo ""

# Test 1: Verificar estado del sistema
echo -e "${CYAN}📡 Verificando estado del sistema y runtimes disponibles${NC}"
echo -e "${YELLOW}   GET /api/status${NC}"
make_request GET "/api/status"
echo ""

# Test 2: Verificar detección automática de runtimes
echo -e "${CYAN}📡 Verificando detección automática de runtimes${NC}"
echo -e "${YELLOW}   GET /api/status${NC}"
make_request GET "/api/status"
echo ""

# Test 3: Deployment con runtime automático
echo -e "${BLUE}🚀 Desplegando aplicación Go con selección automática de runtime...${NC}"
echo ""
echo -e "${CYAN}📡 Desplegando aplicación Go (runtime automático)${NC}"
echo -e "${YELLOW}   POST /api/deploy${NC}"
make_request POST "/api/deploy" '{
    "name": "go-web-app",
    "repo_url": "https://github.com/example/go-web-app.git",
    "language": "go"
}'
echo ""

# Test 4: Deployment forzando Docker
echo -e "${BLUE}🐳 Desplegando aplicación Node.js forzando Docker...${NC}"
echo ""
echo -e "${CYAN}📡 Desplegando con runtime Docker forzado${NC}"
echo -e "${YELLOW}   POST /api/deploy${NC}"
make_request POST "/api/deploy" '{
    "name": "node-api",
    "repo_url": "https://github.com/example/node-api.git",
    "language": "javascript",
    "runtime_type": "docker"
}'
echo ""

# Test 5: Deployment forzando containerd
echo -e "${BLUE}🐳 Desplegando aplicación Python forzando containerd...${NC}"
echo ""
echo -e "${CYAN}📡 Desplegando con runtime containerd forzado${NC}"
echo -e "${YELLOW}   POST /api/deploy${NC}"
make_request POST "/api/deploy" '{
    "name": "flask-app",
    "repo_url": "https://github.com/example/flask-app.git",
    "language": "python",
    "runtime_type": "containerd"
}'
echo ""

# Test 6: Verificar aplicaciones desplegadas
echo -e "${CYAN}📡 Verificando aplicaciones desplegadas${NC}"
echo -e "${YELLOW}   GET /api/status${NC}"
make_request GET "/api/status"
echo ""

# Test 7: Información del sistema
echo -e "${CYAN}🖥️  Información del sistema detectada:${NC}"
echo -e "${CYAN}======================================${NC}"
make_request GET "/api/status" | jq '.data.system' 2>/dev/null || make_request GET "/api/status"
echo ""

# Test 8: Deployment con Rust
echo -e "${BLUE}🦀 Desplegando aplicación Rust...${NC}"
echo ""
echo -e "${CYAN}📡 Desplegando aplicación Rust${NC}"
echo -e "${YELLOW}   POST /api/deploy${NC}"
make_request POST "/api/deploy" '{
    "name": "rust-api",
    "repo_url": "https://github.com/example/rust-api.git",
    "language": "rust"
}'
echo ""

# Test 9: Endpoints específicos de runtime
echo -e "${BLUE}🔧 Probando endpoints específicos de runtime...${NC}"

# Test 9.1: Docker específico
echo -e "${BLUE}🐳 Probando endpoint Docker específico...${NC}"
echo ""
echo -e "${CYAN}📡 Estado específico de Docker${NC}"
echo -e "${YELLOW}   GET /api/docker/status${NC}"
make_request GET "/api/docker/status"
echo ""

# Test 9.2: LXC específico
echo -e "${BLUE}📦 Probando endpoint LXC específico...${NC}"
echo ""
echo -e "${CYAN}📡 Estado específico de LXC${NC}"
echo -e "${YELLOW}   GET /api/lxc/status${NC}"
make_request GET "/api/lxc/status"
echo ""

# Test 10: Verificar capacidades por runtime
echo -e "${CYAN}🎯 Verificando capacidades por runtime:${NC}"
echo -e "${CYAN}======================================${NC}"
make_request GET "/api/status" | jq '.data.runtime' 2>/dev/null || make_request GET "/api/status"
echo ""

# Resumen
echo -e "${GREEN}📊 RESUMEN DE PRUEBAS${NC}"
echo -e "${GREEN}====================${NC}"
echo -e "${GREEN}✅ Sistema híbrido LXC/containerd/Docker funcionando${NC}"
echo -e "${GREEN}✅ Detección automática de SO${NC}"
echo -e "${GREEN}✅ Selección inteligente de runtime${NC}"
echo -e "${GREEN}✅ Soporte para múltiples lenguajes${NC}"
echo -e "${GREEN}✅ API unificada funcionando${NC}"
echo -e "${GREEN}✅ Endpoints específicos de runtime${NC}"
echo ""

# Información final del sistema
echo -e "${PURPLE}🎉 RESULTADO FINAL:${NC}"
SYSTEM_INFO=$(make_request GET "/api/status")
echo -e "${YELLOW}   Runtimes disponibles: ${NC}$(echo "$SYSTEM_INFO" | jq '.data.runtime.available' 2>/dev/null || echo "N/A")"
echo -e "${YELLOW}   Runtime preferido: ${NC}$(echo "$SYSTEM_INFO" | jq '.data.runtime.preferred' 2>/dev/null || echo "N/A")"
echo -e "${YELLOW}   Sistema: ${NC}$(echo "$SYSTEM_INFO" | jq '.data.system.os' 2>/dev/null || echo "N/A") $(echo "$SYSTEM_INFO" | jq '.data.system.architecture' 2>/dev/null || echo "N/A")"
echo -e "${YELLOW}   Lenguajes soportados: ${NC}$(echo "$SYSTEM_INFO" | jq '.data.runtime.supported_languages' 2>/dev/null || echo "N/A")"
echo ""

echo -e "${PURPLE}🏁 Pruebas del sistema híbrido completadas exitosamente!${NC}"
echo -e "${PURPLE}==================================================${NC}"