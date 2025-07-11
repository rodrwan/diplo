#!/bin/bash

set -e

echo "🧪 Iniciando tests de API..."

SERVER_URL="http://localhost:8080"

# Función para hacer peticiones HTTP
http_request() {
    local method="$1"
    local endpoint="$2"
    local data="$3"
    
    if [ -n "$data" ]; then
        curl -s -X "$method" -H "Content-Type: application/json" -d "$data" "$SERVER_URL$endpoint"
    else
        curl -s -X "$method" "$SERVER_URL$endpoint"
    fi
}

echo "📊 1. Testing health endpoint..."
HEALTH_RESPONSE=$(http_request "GET" "/health")
echo "Health response: $HEALTH_RESPONSE"

echo "📱 2. Testing apps list..."
APPS_RESPONSE=$(http_request "GET" "/api/v1/apps")
echo "Apps response: $APPS_RESPONSE"

echo "🔧 3. Testing unified status..."
UNIFIED_STATUS=$(http_request "GET" "/api/v1/status")
echo "Unified status: $UNIFIED_STATUS"

echo "🐳 4. Testing Docker status..."
DOCKER_STATUS=$(http_request "GET" "/api/docker/status")
echo "Docker status: $DOCKER_STATUS"

echo "📦 5. Testing LXC status..."
LXC_STATUS=$(http_request "GET" "/api/lxc/status")
echo "LXC status: $LXC_STATUS"

echo "🚀 6. Testing app deployment..."
DEPLOY_DATA='{"repo_url": "https://github.com/gorilla/mux.git", "name": "test-app"}'
DEPLOY_RESPONSE=$(http_request "POST" "/api/v1/deploy" "$DEPLOY_DATA")
echo "Deploy response: $DEPLOY_RESPONSE"

# Extraer app ID del deploy response para los próximos tests
APP_ID=$(echo "$DEPLOY_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$APP_ID" ]; then
    echo "📋 7. Testing app details..."
    APP_DETAILS=$(http_request "GET" "/api/v1/apps/$APP_ID")
    echo "App details: $APP_DETAILS"
    
    echo "🔍 8. Testing app health check..."
    HEALTH_CHECK=$(http_request "GET" "/api/v1/apps/$APP_ID/health")
    echo "Health check response: $HEALTH_CHECK"
    
    echo "⚠️  9. Testing app deletion..."
    DELETE_RESPONSE=$(http_request "DELETE" "/api/v1/apps/$APP_ID")
    echo "Delete response: $DELETE_RESPONSE"
else
    echo "⚠️  No se pudo extraer APP_ID del deploy response"
fi

echo "✅ Tests completados exitosamente!" 