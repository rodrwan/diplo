# 🧪 Guía de Testing de la API de Diplo

## Endpoints Disponibles

### 1. Health Check del Servidor
```bash
GET /health
```

### 2. Aplicaciones
```bash
GET /api/v1/apps              # Listar todas las aplicaciones
GET /api/v1/apps/{id}         # Obtener detalles de una aplicación
DELETE /api/v1/apps/{id}      # Eliminar una aplicación
POST /api/v1/deploy           # Desplegar nueva aplicación
```

### 3. Logs en Tiempo Real
```bash
GET /api/v1/apps/{id}/logs    # SSE stream de logs
```

### 4. Health Check de Aplicaciones
```bash
GET /api/v1/apps/{id}/health  # Verificar salud de la aplicación
```

**Nuevo Endpoint:** Este endpoint resuelve el problema de CORS al hacer healthchecks desde el frontend. En lugar de que el navegador haga llamadas directas a `localhost:<puerto>`, se hace la llamada a través de nuestra API.

#### Respuesta del Health Check:
```json
{
  "code": 200,
  "data": {
    "healthy": true,
    "status": "healthy",
    "message": "Servicio respondió con código 200",
    "details": {
      "url": "http://localhost:3000",
      "http_status_code": 200,
      "container_id": "abc123",
      "container_status": "running",
      "response_time_ms": 45,
      "timestamp": "2024-01-10T10:30:00Z"
    }
  }
}
```

#### Posibles Estados:
- **healthy**: Aplicación responde correctamente (HTTP 200-399)
- **unhealthy**: Aplicación responde con errores (HTTP 400+)
- **container_not_running**: El contenedor no está ejecutándose
- **connection_error**: No se puede conectar al servicio
- **error**: Error verificando estado del contenedor

### 5. Mantenimiento
```bash
POST /api/v1/maintenance/prune-images  # Limpiar imágenes no utilizadas
```

### 6. Sistema Híbrido
```bash
GET /api/status       # Estado completo del sistema híbrido
POST /api/deploy      # Deployment con selección automática de runtime
GET /api/docker/status        # Estado específico de Docker
GET /api/lxc/status          # Estado específico de LXC
```

## Ejemplos de Uso

### Deployment Básico
```bash
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/gin-gonic/gin.git",
    "name": "gin-api"
  }'
```

### Health Check de Aplicación
```bash
# Primero obtener el ID de la aplicación
APP_ID=$(curl -s http://localhost:8080/api/v1/apps | jq -r '.data[0].id')

# Luego hacer health check
curl -s http://localhost:8080/api/v1/apps/$APP_ID/health | jq '.'
```

### Monitoring con Watch
```bash
# Monitorear el estado de una aplicación cada 5 segundos
watch -n 5 "curl -s http://localhost:8080/api/v1/apps/$APP_ID/health | jq '.data.healthy'"
```

## Scripts de Testing

### Test Completo
```bash
./scripts/test_api.sh
```

### Test del Sistema Híbrido
```bash
./scripts/test_hybrid_system.sh
```

### Test Específico de LXC
```bash
./scripts/test_lxc_deploy.sh
```

## Ventajas del Nuevo Health Check

1. **Sin CORS**: El endpoint funciona desde cualquier origen
2. **Información Completa**: Incluye estado del contenedor y métricas
3. **Timeout Configurado**: Evita esperas infinitas
4. **Manejo de Errores**: Respuestas estructuradas para todos los casos
5. **Integración con Frontend**: Funciona perfectamente con el botón de Health Check

## Troubleshooting

### Aplicación no responde al Health Check
1. Verificar que el contenedor esté ejecutándose
2. Verificar que el puerto esté correctamente mapeado
3. Verificar que la aplicación esté escuchando en el puerto correcto
4. Revisar los logs de la aplicación

### Error de CORS (Legacy)
Si aún experimentas problemas de CORS:
1. Usa el nuevo endpoint `/api/v1/apps/{id}/health`
2. No hagas llamadas directas desde el frontend a `localhost:<puerto>`
3. Actualiza el frontend para usar la nueva API 