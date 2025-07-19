# Recuperación Automática de Contenedores

## 🎯 **Problema Resuelto**

Antes de esta implementación, cuando el servidor Diplo se reiniciaba, todos los contenedores que estaban ejecutándose se perdían porque no había un mecanismo de persistencia o recuperación automática.

## ✅ **Solución Implementada**

### **1. Recuperación Automática al Iniciar**

El servidor ahora incluye un mecanismo de recuperación automática que se ejecuta al iniciar:

```go
// En internal/server/server.go
func (s *Server) recoverContainers() error {
    // 1. Obtener todas las aplicaciones de la BD
    // 2. Obtener runtime preferido (Docker o containerd)
    // 3. Obtener contenedores ejecutándose según el runtime
    // 4. Comparar y recuperar contenedores perdidos
}
```

**Flujo de Recuperación:**
1. **Lectura de BD:** Obtiene todas las aplicaciones marcadas como "running"
2. **Detección de Runtime:** Determina si usar Docker o containerd
3. **Verificación de Estado:** Lista contenedores realmente ejecutándose
4. **Comparación:** Identifica contenedores perdidos
5. **Recreación:** Recrea contenedores usando imágenes existentes
6. **Actualización:** Actualiza estado en BD

### **2. Soporte Multi-Runtime**

El sistema ahora soporta tanto **Docker** como **containerd**:

#### **Docker Runtime:**
- ✅ **GetRunningContainers():** Lista contenedores Docker ejecutándose
- ✅ **GetContainerStatus():** Verifica estado de contenedores Docker
- ✅ **Recuperación Completa:** Recreación automática de contenedores Docker

#### **Containerd Runtime:**
- ✅ **GetRunningContainers():** Lista contenedores containerd ejecutándose
- ✅ **GetContainerStatus():** Verifica estado de contenedores containerd
- ✅ **Detección Automática:** Detecta automáticamente si containerd está disponible
- ⚠️ **Recreación:** Usa Docker como fallback (en desarrollo)

### **3. Endpoint Manual de Recuperación**

Nuevo endpoint para recuperación manual:

```bash
POST /api/v1/maintenance/recover-containers
```

**Respuesta:**
```json
{
  "success": true,
  "message": "Recuperación de contenedores completada",
  "recovered": 3,
  "errors": 0,
  "skipped": 1,
  "total_apps": 4,
  "running_containers": 5,
  "runtime_used": "containerd",
  "available_runtimes": ["docker", "containerd"]
}
```

### **4. Métodos Multi-Runtime**

Nuevos métodos en ambos clientes:

```go
// En internal/docker/client.go
func (d *Client) GetRunningContainers() ([]types.Container, error)
func (d *Client) GetContainerStatus(containerID string) (string, error)

// En internal/runtime/containerd_client.go
func (c *ContainerdClient) GetRunningContainers() ([]*Container, error)
func (c *ContainerdClient) GetContainerStatus(containerID string) (string, error)
```

## 🔧 **Implementación Técnica**

### **Funciones Principales:**

#### **1. `recoverContainers()` - Recuperación Automática**
- Se ejecuta al iniciar el servidor
- Detecta automáticamente el runtime preferido
- Verifica estado de contenedores vs. BD
- Recrea contenedores perdidos automáticamente

#### **2. `updateAppStatus()` - Actualización de Estado**
- Actualiza estado de aplicaciones en BD
- Maneja errores de recuperación
- Mantiene consistencia de datos

#### **3. `recreateContainer()` - Recreación de Contenedores**
- Obtiene variables de entorno de BD
- Descifra valores secretos si es necesario
- Recrea contenedor usando imagen existente
- Actualiza información en BD

### **Detección Automática de Runtime:**

```go
// El sistema detecta automáticamente qué runtime usar
preferredRuntime := s.runtimeFactory.GetPreferredRuntime()

switch preferredRuntime {
case runtimePkg.RuntimeTypeDocker:
    // Usar Docker client
case runtimePkg.RuntimeTypeContainerd:
    // Usar containerd client
}
```

### **Manejo de Variables de Entorno:**

```go
// Descifrado automático de valores secretos
if env.IsSecret.Bool {
    if decryptedValue, err := handlers.DecryptValue(env.Value); err != nil {
        logrus.Errorf("Error descifrando valor secreto %s: %v", env.Key, err)
        continue
    } else {
        value = decryptedValue
    }
}
```

## 📊 **Estados de Recuperación**

### **Contenedores Recuperados:**
- ✅ **Ya ejecutándose:** Contenedor existe y está healthy
- ✅ **Recreado exitosamente:** Contenedor recreado y ejecutándose
- ⚠️ **Error:** Problema durante recreación (imagen no disponible, etc.)

### **Contenedores Saltados:**
- ⏭️ **No running:** Aplicación no estaba en estado "running"
- ⏭️ **Sin container_id:** Aplicación sin ID de contenedor

## 🚀 **Uso**

### **Recuperación Automática:**
```bash
# Al reiniciar el servidor, la recuperación es automática
./bin/diplo
```

### **Recuperación Manual:**
```bash
# Endpoint para recuperación manual
curl -X POST http://localhost:8080/api/v1/maintenance/recover-containers
```

## 🔍 **Logs de Recuperación**

El sistema proporciona logs detallados:

```
🔍 Iniciando recuperación de contenedores...
📋 Encontradas 3 aplicaciones en BD
🎯 Usando runtime preferido: containerd
🔧 Encontrados 2 contenedores containerd ejecutándose
✅ Contenedor diplo-app123 para app app_123 está ejecutándose
⚠️  Contenedor diplo-app456 para app app_456 no está ejecutándose, intentando recrear...
🔄 Recreando contenedor para app app_456
✅ Contenedor recreado exitosamente para app app_456
🎯 Recuperación completada: 2 recuperadas, 0 errores
```

## 🛡️ **Seguridad y Robustez**

### **Manejo de Errores:**
- ✅ Verificación de imágenes existentes
- ✅ Descifrado seguro de variables secretas
- ✅ Rollback automático en caso de fallo
- ✅ Logs detallados para debugging
- ✅ Fallback automático entre runtimes

### **Consistencia de Datos:**
- ✅ Verificación de estado real vs. BD
- ✅ Actualización atómica de estados
- ✅ Manejo de contenedores huérfanos
- ✅ Soporte multi-runtime

## 📈 **Beneficios**

1. **🔄 Persistencia:** Contenedores sobreviven reinicios del servidor
2. **⚡ Recuperación Rápida:** Recuperación automática al iniciar
3. **🛠️ Mantenimiento:** Endpoint manual para recuperación
4. **📊 Visibilidad:** Logs detallados del proceso
5. **🔒 Seguridad:** Manejo seguro de variables secretas
6. **🎯 Precisión:** Verificación de estado real de contenedores
7. **🔧 Multi-Runtime:** Soporte para Docker y containerd
8. **🤖 Automático:** Detección automática del runtime preferido

## 🔮 **Próximas Mejoras**

- [ ] **Recuperación Incremental:** Solo recrear contenedores perdidos
- [ ] **Health Checks:** Verificación de salud de contenedores recuperados
- [ ] **Métricas:** Estadísticas de recuperación
- [ ] **Notificaciones:** Alertas de contenedores no recuperables
- [ ] **Configuración:** Opciones de recuperación configurables
- [ ] **Recreación Containerd:** Implementar recreación nativa para containerd
- [ ] **Métricas por Runtime:** Estadísticas separadas por runtime 