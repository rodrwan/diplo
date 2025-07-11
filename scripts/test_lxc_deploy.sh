#!/bin/bash

# Script de prueba para Diplo LXC
# Uso: ./test_lxc_deploy.sh

set -e

DIPLO_URL="http://localhost:8080"
REPO_URL="https://github.com/golang/example"
APP_NAME="test-go-app"

echo "=== Probando Diplo LXC ==="

# Función para hacer peticiones HTTP
make_request() {
    local method=$1
    local endpoint=$2
    local data=$3
    
    echo "→ $method $endpoint"
    
    if [ -n "$data" ]; then
        curl -s -X $method "$DIPLO_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" | jq '.'
    else
        curl -s -X $method "$DIPLO_URL$endpoint" | jq '.'
    fi
}

# 1. Desplegar aplicación
echo "1. Desplegando aplicación..."
DEPLOY_RESPONSE=$(make_request POST "/deploy/lxc" '{
    "repo_url": "'$REPO_URL'",
    "name": "'$APP_NAME'"
}')

APP_ID=$(echo $DEPLOY_RESPONSE | jq -r '.data.id')
echo "   App ID: $APP_ID"

# 2. Esperar deployment
echo "2. Esperando deployment..."
for i in {1..30}; do
    echo "   Intento $i/30..."
    STATUS_RESPONSE=$(make_request GET "/status/lxc?app_id=$APP_ID")
    STATUS=$(echo $STATUS_RESPONSE | jq -r '.data.deployment_status // .data.status')
    
    echo "   Estado: $STATUS"
    
    if [ "$STATUS" = "deployed" ] || [ "$STATUS" = "running" ]; then
        echo "   ✅ Deployment exitoso!"
        break
    elif [ "$STATUS" = "failed" ] || [ "$STATUS" = "error" ]; then
        echo "   ❌ Deployment falló!"
        echo $STATUS_RESPONSE | jq '.data.error // .data.error_msg'
        exit 1
    fi
    
    sleep 10
done

# 3. Verificar estado
echo "3. Verificando estado final..."
FINAL_STATUS=$(make_request GET "/status/lxc?app_id=$APP_ID")
echo $FINAL_STATUS | jq '.data | {id, name, status, port, url}'

# 4. Probar conectividad
PORT=$(echo $FINAL_STATUS | jq -r '.data.port')
if [ "$PORT" != "null" ] && [ "$PORT" != "0" ]; then
    echo "4. Probando conectividad en puerto $PORT..."
    
    # Probar health check
    if curl -s -f "http://localhost:$PORT/_diplo/health" > /dev/null; then
        echo "   ✅ Health check OK"
    else
        echo "   ⚠️  Health check falló"
    fi
    
    # Probar endpoint principal
    if curl -s -f "http://localhost:$PORT/" > /dev/null; then
        echo "   ✅ Aplicación responde"
    else
        echo "   ⚠️  Aplicación no responde"
    fi
else
    echo "   ⚠️  Puerto no asignado"
fi

# 5. Listar aplicaciones
echo "5. Listando aplicaciones..."
make_request GET "/list/lxc"

# 6. Detener aplicación
echo "6. Deteniendo aplicación..."
make_request POST "/stop/lxc?app_id=$APP_ID"

echo "=== Prueba completada ===" 