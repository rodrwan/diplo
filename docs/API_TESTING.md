# Guía de Pruebas de la API de Diplo

## Configuración de Postman

### 1. Importar la Colección

1. Abre Postman
2. Haz clic en "Import" 
3. Selecciona el archivo `docs/postman_collection.json`
4. La colección "Diplo API - PaaS Local" se importará automáticamente

### 2. Configurar Variables

La colección incluye una variable `app_id` que se usa para las requests que necesitan un ID específico. Para usarla:

1. Después de crear una aplicación con `POST /deploy`, copia el `id` de la respuesta
2. En Postman, ve a la pestaña "Variables" de la colección
3. Actualiza el valor de `app_id` con el ID real de tu aplicación

## Endpoints Disponibles

### 1. Health Check
- **GET** `http://localhost:8080/`
- **Descripción**: Verificar que el servidor esté funcionando
- **Respuesta esperada**: Mensaje de bienvenida

### 2. Deploy Application
- **POST** `http://localhost:8080/api/v1/deploy`
- **Headers**: `Content-Type: application/json`
- **Body**:
```json
{
  "repo_url": "https://github.com/example/go-app",
  "name": "mi-aplicacion-go"
}
```
- **Respuesta esperada**:
```json
{
  "id": "app_1234567890_123456",
  "name": "mi-aplicacion-go",
  "repo_url": "https://github.com/example/go-app",
  "port": 8081,
  "url": "http://localhost:8081",
  "status": "deploying",
  "message": "Aplicación creada y deployment iniciado"
}
```

### 3. Get All Applications
- **GET** `http://localhost:8080/api/v1/apps`
- **Descripción**: Obtener todas las aplicaciones desplegadas
- **Respuesta esperada**:
```json
[
  {
    "id": "app_1234567890_123456",
    "name": "mi-aplicacion-go",
    "repo_url": "https://github.com/example/go-app",
    "language": "go",
    "port": 8081,
    "url": "http://localhost:8081",
    "container_id": "abc123def456",
    "status": "running",
    "error_msg": "",
    "created_at": 1703123456,
    "updated_at": 1703123456
  }
]
```

### 4. Get Application by ID
- **GET** `http://localhost:8080/api/v1/apps/{{app_id}}`
- **Descripción**: Obtener detalles de una aplicación específica
- **Respuesta esperada**: Objeto con información completa de la app

### 5. Delete Application
- **DELETE** `http://localhost:8080/api/v1/apps/{{app_id}}`
- **Descripción**: Eliminar aplicación y su contenedor
- **Respuesta esperada**: Confirmación de eliminación

### 6. 🆕 Logs en Tiempo Real (SSE)
- **GET** `http://localhost:8080/api/v1/apps/{{app_id}}/logs`
- **Descripción**: Obtener logs en tiempo real usando Server-Sent Events
- **Headers**: `Accept: text/event-stream`
- **Respuesta**: Stream de eventos SSE con logs en tiempo real

#### Ejemplo de uso con JavaScript:
```javascript
const eventSource = new EventSource('http://localhost:8080/api/v1/apps/app_1234567890_123456/logs');

eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log(`[${data.type}] ${data.message}`);
};

eventSource.onerror = function(event) {
    console.error('Error en conexión SSE');
};
```

#### Tipos de mensajes SSE:
- `connected`: Conexión establecida
- `info`: Información general del proceso
- `success`: Operación exitosa
- `error`: Error en el proceso
- `log`: Log del contenedor en ejecución

## Testing con Herramientas

### 1. Postman
- Usa la colección importada para probar endpoints REST
- Para SSE, usa herramientas como curl o el navegador

### 2. cURL
```bash
# Health check
curl http://localhost:8080/

# Deploy app
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{"repo_url": "https://github.com/user/repo.git"}'

# Get apps
curl http://localhost:8080/api/v1/apps

# SSE logs (en otra terminal)
curl -N http://localhost:8080/api/v1/apps/app_1234567890_123456/logs
```

### 3. Página de Testing SSE
- Abre `docs/sse_test.html` en tu navegador
- Ingresa el ID de una aplicación
- Haz clic en "Conectar" para ver logs en tiempo real

## Flujo de Testing Completo

### 1. Deploy y Monitoreo
```bash
# 1. Deploy una aplicación
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{"repo_url": "https://github.com/rodrwan/simple-go-app.git"}'

# 2. Obtener el ID de la respuesta
# 3. Abrir SSE en otra terminal o navegador
# 4. Monitorear logs en tiempo real
```

### 2. Verificar Estado
```bash
# Ver todas las apps
curl http://localhost:8080/api/v1/apps

# Ver app específica
curl http://localhost:8080/api/v1/apps/app_1234567890_123456
```

### 3. Limpiar
```bash
# Eliminar aplicación
curl -X DELETE http://localhost:8080/api/v1/apps/app_1234567890_123456
```

## Troubleshooting

### Problemas Comunes

1. **Error de CORS**: Asegúrate de que el servidor esté configurado con CORS
2. **SSE no funciona**: Verifica que el navegador soporte EventSource
3. **Logs no aparecen**: Confirma que la aplicación esté ejecutándose
4. **Puerto ocupado**: Verifica que el puerto 8080 esté libre

### Debugging

1. **Logs del servidor**: Revisa la salida del servidor Diplo
2. **Logs de Docker**: `docker logs <container_id>`
3. **Estado de contenedores**: `docker ps`
4. **Logs de la aplicación**: Usa el endpoint SSE 