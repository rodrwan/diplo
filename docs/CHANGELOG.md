# Changelog - Diplo

## [1.1.0] - 2024-12-19

### 🎉 **Recuperación Automática de Contenedores**

#### **Nueva Funcionalidad Crítica:**

### ✅ **Mecanismo de Recuperación Automática**
- **Recuperación al Iniciar:** El servidor ahora recupera automáticamente contenedores perdidos al reiniciar
- **Verificación de Estado:** Compara contenedores en BD vs. contenedores realmente ejecutándose
- **Recreación Inteligente:** Recrea contenedores perdidos usando imágenes existentes
- **Manejo de Variables Secretas:** Descifra automáticamente variables de entorno secretas durante la recuperación

### ✅ **Soporte Multi-Runtime (Docker + Containerd)**
- **Detección Automática:** Detecta automáticamente si usar Docker o containerd
- **GetRunningContainers():** Implementado para ambos runtimes
- **GetContainerStatus():** Verificación de estado para Docker y containerd
- **Fallback Inteligente:** Usa Docker como fallback si containerd no está disponible
- **Recuperación Híbrida:** Soporte completo para ambos runtimes

### ✅ **Endpoint de Recuperación Manual**
- **POST /api/v1/maintenance/recover-containers:** Endpoint para recuperación manual de contenedores
- **Respuesta Detallada:** Incluye estadísticas de recuperación (recuperados, errores, saltados)
- **Información de Runtime:** Muestra qué runtime se está usando y cuáles están disponibles
- **Logs Informativos:** Proporciona información detallada del proceso de recuperación

### ✅ **Métodos Multi-Runtime**
- **Nuevo Método Docker:** `GetRunningContainers()` para listar contenedores ejecutándose
- **Nuevo Método Containerd:** `GetRunningContainers()` para listar contenedores containerd
- **Integración Completa:** Integrado con el sistema de recuperación automática
- **Eficiencia:** Optimizado para búsqueda rápida de contenedores en ambos runtimes

### ✅ **Funciones de Soporte**
- **updateAppStatus():** Actualización segura de estados de aplicaciones
- **recreateContainer():** Recreación de contenedores con variables de entorno
- **DecryptValue():** Función pública para descifrado de valores secretos
- **Detección de Runtime:** Sistema automático para determinar runtime preferido

### 🔧 **Mejoras Técnicas**
- **Manejo de Errores Robusto:** Rollback automático en caso de fallos
- **Logs Detallados:** Sistema de logging mejorado para debugging
- **Consistencia de Datos:** Verificación y actualización atómica de estados
- **Seguridad:** Manejo seguro de variables secretas durante recuperación
- **Multi-Runtime:** Soporte completo para Docker y containerd
- **Fallback Automático:** Cambio automático entre runtimes si es necesario

### 📊 **Beneficios del Usuario**
1. **🔄 Persistencia:** Los contenedores sobreviven reinicios del servidor
2. **⚡ Recuperación Automática:** No requiere intervención manual
3. **🛠️ Control Manual:** Endpoint para recuperación manual cuando sea necesario
4. **📊 Visibilidad:** Logs detallados del proceso de recuperación
5. **🔒 Seguridad:** Manejo seguro de variables secretas
6. **🎯 Precisión:** Verificación de estado real de contenedores
7. **🔧 Multi-Runtime:** Soporte para Docker y containerd
8. **🤖 Automático:** Detección automática del runtime preferido

---

## [1.0.0] - 2024-07-08

### 🎉 **Lanzamiento Inicial**

#### **Características Principales Implementadas:**

### ✅ **Sistema de Base de Datos SQLite**
- **Tabla `apps`:** Almacenamiento de aplicaciones desplegadas
- **Tabla `deployment_logs`:** Historial de eventos de deployment
- **CRUD completo:** Crear, leer, actualizar, eliminar aplicaciones
- **Logging automático:** Registro de todos los eventos importantes

### ✅ **API REST Completa**
- **POST /deploy:** Crear y desplegar aplicaciones
- **GET /apps:** Listar todas las aplicaciones
- **GET /apps/{id}:** Obtener detalles de aplicación específica
- **DELETE /apps/{id}:** Eliminar aplicación y contenedor
- **GET /:** Health check del servidor
- **CORS habilitado:** Soporte para peticiones desde navegador

### ✅ **Sistema Docker Avanzado**
- **Generación automática de Dockerfiles** para múltiples lenguajes:
  - **Go 1.24:** Multi-stage build optimizado
  - **Node.js 18:** Soporte para npm/yarn
  - **Python 3.11:** Soporte para pip/requirements.txt
- **Construcción de imágenes:** `docker build` automático
- **Ejecución de contenedores:** `docker run` con puertos asignados
- **Gestión de contenedores:** Stop, remove, cleanup automático

### ✅ **Sistema de Puertos Inteligente**
- **Asignación automática:** Puertos únicos al crear aplicación
- **Rango configurado:** 3000-9999 para aplicaciones web
- **Verificación de disponibilidad:** Comprobación de puertos en uso
- **URLs automáticas:** Generación de URLs de acceso (`http://localhost:PUERTO`)
- **Persistencia:** Puertos guardados en base de datos

### ✅ **Detección de Lenguajes**
- **Detección automática:** Basada en URL del repositorio
- **Soporte inicial:** Go, Node.js, Python
- **Fallback:** Go como lenguaje por defecto
- **Extensible:** Fácil agregar nuevos lenguajes

### ✅ **Sistema de Estados**
- **Estados de aplicación:** idle, deploying, running, error
- **Transiciones automáticas:** Seguimiento del ciclo de vida
- **Manejo de errores:** Mensajes descriptivos de fallos
- **Recuperación:** Limpieza automática en caso de error

### ✅ **Concurrencia y Threading**
- **Servidor HTTP multithreaded:** libmicrohttpd
- **Protección de recursos:** Mutex para acceso seguro
- **Threading seguro:** Operaciones concurrentes protegidas
- **Escalabilidad:** Preparado para múltiples conexiones

### ✅ **Logging y Monitoreo**
- **Logs estructurados:** Base de datos SQLite
- **Eventos rastreados:** created, deploy_start, deploy_success, deploy_error, deleted
- **Timestamps automáticos:** Registro de fechas y horas
- **Logs de consola:** Información en tiempo real

### ✅ **Gestión de Memoria**
- **Arrays dinámicos:** Redimensionamiento automático
- **Gestión manual:** malloc/free controlado
- **Cleanup automático:** Liberación de recursos
- **Bounds checking:** Verificación de límites

### ✅ **Manejo de Errores Robusto**
- **Códigos de retorno:** 0 = éxito, -1 = error
- **Logging de errores:** Registro detallado de fallos
- **Recuperación:** Limpieza en caso de error
- **Validación:** Verificación de parámetros de entrada

---

## **Detalles Técnicos Implementados:**

### **Estructuras de Datos:**
```c
// Aplicación desplegada
typedef struct {
    char id[64];           // ID único generado
    char name[128];        // Nombre de la aplicación
    char repo_url[512];    // URL del repositorio Git
    char language[32];     // Lenguaje detectado
    int port;             // Puerto asignado (3000-9999)
    char container_id[64]; // ID del contenedor Docker
    diplo_status_t status; // Estado actual
    char error_msg[256];   // Mensaje de error si aplica
    time_t created_at;     // Timestamp de creación
    time_t updated_at;     // Timestamp de última actualización
} diplo_app_t;
```

### **Funciones Principales:**
- `diplo_init()` - Inicialización del servidor
- `diplo_deploy_app()` - Deployment completo de aplicación
- `diplo_generate_dockerfile()` - Generación de Dockerfiles
- `diplo_build_image()` - Construcción de imágenes Docker
- `diplo_run_container()` - Ejecución de contenedores
- `diplo_find_free_port()` - Asignación de puertos únicos
- `diplo_db_save_app()` - Persistencia en base de datos

### **Endpoints REST:**
```bash
# Crear y desplegar aplicación
POST /deploy
{
  "repo_url": "https://github.com/user/repo.git",
  "name": "opcional"
}

# Listar aplicaciones
GET /apps

# Obtener detalles de aplicación
GET /apps/{id}

# Eliminar aplicación
DELETE /apps/{id}

# Health check
GET /
```

### **Respuestas JSON:**
```json
{
  "id": "app_1234567890_123456",
  "name": "mi-aplicacion",
  "repo_url": "https://github.com/user/repo.git",
  "port": 3456,
  "url": "http://localhost:3456",
  "status": "running",
  "language": "go",
  "container_id": "abc123def456",
  "created_at": 1720483200,
  "updated_at": 1720483260
}
```

---

## **Dependencias Externas:**
- **libmicrohttpd:** Servidor HTTP
- **jansson:** Parsing de JSON
- **sqlite3:** Base de datos
- **pthread:** Threading y concurrencia
- **Docker:** Construcción y ejecución de contenedores

## **Sistema Operativo:**
- **Desarrollado en:** macOS (Darwin)
- **Compatible con:** Linux (Raspberry Pi OS)
- **Arquitectura:** ARM (Raspberry Pi Zero)

## **Compilación:**
```bash
make          # Compilar proyecto
make clean    # Limpiar archivos generados
make run      # Compilar y ejecutar
make debug    # Compilar con símbolos de debug
```

---

## **Próximas Iteraciones Planificadas:**

### **v1.1.0 - Threading y Concurrencia**
- [ ] Deployment asíncrono en threads separados
- [ ] Health checks de contenedores
- [ ] Timeout para deployments largos
- [ ] Cancelación de deployments

### **v1.2.0 - UI Web**
- [ ] Interfaz gráfica en HTML/JS
- [ ] Dashboard en tiempo real
- [ ] Métricas de uso
- [ ] Gestión visual de aplicaciones

### **v1.3.0 - Características Avanzadas**
- [ ] Environment variables por aplicación
- [ ] Volúmenes persistentes
- [ ] SSL/TLS para aplicaciones
- [ ] Custom domains

---

*Changelog actualizado: Julio 2024*
*Versión: 1.0.0* 