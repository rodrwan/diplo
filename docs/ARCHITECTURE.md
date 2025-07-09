# Diplo - Arquitectura del Sistema

## 📋 **Resumen del Proyecto**

Diplo es un **PaaS local** desarrollado en **C puro** que permite desplegar aplicaciones web desde repositorios Git. Funciona como un daemon que expone una API REST y gestiona contenedores Docker automáticamente.

## 🏗️ **Arquitectura General**

### **Componentes Principales:**

1. **Servidor HTTP** (`libmicrohttpd`) - API REST
2. **Base de Datos** (`SQLite3`) - Persistencia de aplicaciones
3. **Sistema Docker** - Construcción y ejecución de contenedores
4. **Gestión de Puertos** - Asignación automática de puertos únicos
5. **Sistema de Logging** - Registro de eventos en BD

### **Flujo de Deployment:**

```
POST /deploy → Crear App → Asignar Puerto → Generar Dockerfile → 
Build Image → Run Container → Actualizar BD → Retornar URL
```

## 📁 **Estructura del Código**

### **Archivos Principales:**

- `src/main.c` - Punto de entrada, servidor HTTP
- `src/handlers.c` - Endpoints REST (POST/GET/DELETE)
- `src/database.c` - Operaciones SQLite (CRUD apps, logs)
- `src/docker.c` - Gestión Docker (build, run, stop)
- `src/utils.c` - Utilidades (puertos, IDs, comandos)
- `include/diplo.h` - Headers y estructuras de datos

### **Estructuras de Datos:**

```c
// Aplicación desplegada
typedef struct {
    char id[64];           // ID único
    char name[128];        // Nombre de la app
    char repo_url[512];    // URL del repositorio
    char language[32];     // Lenguaje detectado
    int port;             // Puerto asignado
    char container_id[64]; // ID del contenedor Docker
    diplo_status_t status; // Estado (idle/deploying/running/error)
    char error_msg[256];   // Mensaje de error si aplica
    time_t created_at;     // Timestamp de creación
    time_t updated_at;     // Timestamp de última actualización
} diplo_app_t;

// Servidor principal
typedef struct {
    struct MHD_Daemon *daemon;  // Servidor HTTP
    int port;                   // Puerto del servidor (8080)
    int running;                // Estado del servidor
    pthread_mutex_t apps_mutex; // Mutex para concurrencia
    diplo_app_t *apps;         // Array de aplicaciones
    int apps_count;            // Número de apps
    int apps_capacity;         // Capacidad del array
} diplo_server_t;
```

## 🌐 **API REST**

### **Endpoints Implementados:**

#### **POST /deploy**
- **Propósito:** Crear y desplegar una nueva aplicación
- **Body:** `{"repo_url": "https://github.com/user/repo.git", "name": "opcional"}`
- **Respuesta:** 
```json
{
  "id": "app_1234567890_123456",
  "name": "repo-name",
  "repo_url": "https://github.com/user/repo.git",
  "port": 3456,
  "url": "http://localhost:3456",
  "status": "deploying",
  "message": "Aplicación creada y deployment iniciado"
}
```

#### **GET /apps**
- **Propósito:** Listar todas las aplicaciones
- **Respuesta:** Array de aplicaciones con metadata completa

#### **GET /apps/{id}**
- **Propósito:** Obtener detalles de una aplicación específica
- **Respuesta:** Objeto con información completa de la app

#### **DELETE /apps/{id}**
- **Propósito:** Eliminar aplicación y su contenedor
- **Respuesta:** Confirmación de eliminación

#### **GET /**
- **Propósito:** Health check del servidor
- **Respuesta:** Estado del servidor y versión

## 🗄️ **Base de Datos SQLite**

### **Tablas:**

#### **apps**
```sql
CREATE TABLE apps (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    language TEXT,
    port INTEGER,
    container_id TEXT,
    status TEXT DEFAULT 'idle',
    error_msg TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

#### **deployment_logs**
```sql
CREATE TABLE deployment_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id TEXT,
    action TEXT,
    message TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (app_id) REFERENCES apps(id)
);
```

## 🐳 **Sistema Docker**

### **Lenguajes Soportados:**

#### **Go 1.24**
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone <repo_url> .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
```

#### **Node.js 18**
```dockerfile
FROM node:18-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone <repo_url> .
RUN npm ci --only=production

FROM node:18-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE 3000
CMD ["npm", "start"]
```

#### **Python 3.11**
```dockerfile
FROM python:3.11-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone <repo_url> .
RUN pip install -r requirements.txt

FROM python:3.11-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE 8000
CMD ["python", "app.py"]
```

### **Funciones Docker:**

- `diplo_generate_dockerfile()` - Generar Dockerfile según lenguaje
- `diplo_build_image()` - Construir imagen Docker
- `diplo_run_container()` - Ejecutar contenedor
- `diplo_stop_container()` - Detener contenedor
- `diplo_remove_image()` - Eliminar imagen

## 🔌 **Sistema de Puertos**

### **Características:**

- **Rango:** 3000-9999 (configurable)
- **Asignación:** Aleatoria con verificación de disponibilidad
- **Persistencia:** Guardado en BD desde la creación
- **URLs:** Generadas automáticamente (`http://localhost:PUERTO`)

### **Funciones:**

- `diplo_find_free_port()` - Encontrar puerto libre
- `diplo_is_port_in_use()` - Verificar si puerto está ocupado

## 🔄 **Estados de Aplicación**

```c
typedef enum {
    DIPLO_STATUS_IDLE,      // Creada, esperando deployment
    DIPLO_STATUS_DEPLOYING, // En proceso de deployment
    DIPLO_STATUS_RUNNING,   // Ejecutándose correctamente
    DIPLO_STATUS_ERROR      // Error en deployment
} diplo_status_t;
```

## 🧵 **Concurrencia**

### **Threading:**
- **Servidor HTTP:** Multithreaded (libmicrohttpd)
- **Mutex:** Protección de acceso a array de apps
- **Deployment:** Síncrono (futura mejora: threads separados)

### **Protección de Recursos:**
- `pthread_mutex_lock()` / `pthread_mutex_unlock()`
- Acceso seguro a variables compartidas

## 📊 **Logging y Monitoreo**

### **Logs en Base de Datos:**
- **Acciones:** created, deploy_start, deploy_success, deploy_error, deleted
- **Mensajes:** Descriptivos con contexto
- **Timestamps:** Automáticos

### **Logs en Consola:**
- **INFO:** Operaciones exitosas
- **ERROR:** Errores y fallos
- **WARNING:** Situaciones de advertencia

## 🚀 **Compilación y Dependencias**

### **Dependencias Externas:**
- `libmicrohttpd` - Servidor HTTP
- `jansson` - Parsing JSON
- `sqlite3` - Base de datos
- `libcurl` - Clonado de repositorios (futuro)
- `pthread` - Threading

### **Compilación:**
```bash
make          # Compilar
make clean    # Limpiar
make run      # Compilar y ejecutar
make debug    # Compilar con debug
```

## 🔧 **Configuración**

### **Constantes Importantes:**
```c
#define DIPLO_PORT 8080           // Puerto del servidor
#define DIPLO_DB_PATH "diplo.db"  // Archivo de BD
#define MIN_PORT 3000             // Puerto mínimo para apps
#define MAX_PORT 9999             // Puerto máximo para apps
```

## 📈 **Métricas y Rendimiento**

### **Límites Actuales:**
- **Apps simultáneas:** Limitado por memoria
- **Puertos:** 7000 disponibles (3000-9999)
- **Tiempo de deployment:** Depende del tamaño del repo

### **Optimizaciones Futuras:**
- **Threading:** Deployment asíncrono
- **Caching:** Imágenes Docker reutilizables
- **Load Balancing:** Distribución de carga
- **Health Checks:** Monitoreo de contenedores

## 🔮 **Roadmap Futuro**

### **Corto Plazo:**
1. **Threading:** Deployment en background
2. **Health Checks:** Monitoreo de contenedores
3. **UI Web:** Interfaz gráfica
4. **SSL/TLS:** HTTPS para apps

### **Mediano Plazo:**
1. **Load Balancing:** Múltiples instancias
2. **Persistent Storage:** Volúmenes Docker
3. **Environment Variables:** Configuración por app
4. **Custom Domains:** Dominios personalizados

### **Largo Plazo:**
1. **Clustering:** Múltiples nodos
2. **Auto-scaling:** Escalado automático
3. **CI/CD Integration:** Webhooks
4. **Monitoring:** Métricas avanzadas

## 🐛 **Debugging y Troubleshooting**

### **Comandos Útiles:**
```bash
# Ver logs de deployment
sqlite3 diplo.db "SELECT * FROM deployment_logs ORDER BY timestamp DESC;"

# Ver apps en BD
sqlite3 diplo.db "SELECT * FROM apps;"

# Ver contenedores Docker
docker ps

# Ver imágenes Docker
docker images | grep diplo
```

### **Logs Importantes:**
- **Base de datos:** `diplo.db`
- **Docker:** `docker logs <container_id>`
- **Servidor:** Salida estándar del proceso

## 📝 **Notas de Desarrollo**

### **Convenciones de Código:**
- **Nombres de funciones:** `diplo_<modulo>_<accion>()`
- **Variables globales:** `g_<nombre>`
- **Constantes:** `DIPLO_<NOMBRE>`
- **Comentarios:** `//` para líneas, `/* */` para bloques

### **Manejo de Errores:**
- **Códigos de retorno:** 0 = éxito, -1 = error
- **Logging:** Siempre registrar errores
- **Cleanup:** Liberar recursos en caso de error

### **Memoria:**
- **malloc/free:** Gestión manual de memoria
- **Valgrind:** Usar para detectar leaks
- **Bounds checking:** Verificar límites de arrays

---

*Documentación actualizada: Julio 2024*
*Versión: 1.0.0* 