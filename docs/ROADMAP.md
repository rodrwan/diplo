# 🗺️ ROADMAP - Diplo PaaS Local

## 📍 **Estado Actual: v1.0 - Arquitectura Completa**

### ✅ **COMPLETADO (Enero 2024)**

#### **🏗️ Arquitectura Híbrida Completa**
- [x] Sistema híbrido LXC/Docker/containerd ✅
- [x] Factory pattern con detección automática de runtime ✅
- [x] Interfaz unificada `ContainerRuntime` ✅
- [x] Templates completos para todos los runtimes ✅

#### **🐳 Docker Integration**
- [x] Docker client nativo (API, no CLI) ✅
- [x] BuildImage() y RunContainer() funcionales ✅
- [x] Gestión de eventos y logs en tiempo real ✅
- [x] Tags únicos basados en commits Git ✅

#### **📱 Frontend & API**
- [x] Interfaz web completa con Server-Sent Events ✅
- [x] API REST completa con CORS ✅
- [x] Sistema de logs en tiempo real ✅
- [x] Dashboard con gestión de aplicaciones ✅

#### **💾 Persistencia**
- [x] Base de datos SQLite con SQLC ✅
- [x] Modelos y queries completos ✅
- [x] Migraciones automáticas ✅

#### **🔧 Templates Multi-lenguaje**
- [x] Go (multi-stage build optimizado) ✅
- [x] Node.js/JavaScript (NPM/Yarn support) ✅
- [x] Python (pip/requirements.txt) ✅
- [x] Rust (Cargo build optimizado) ✅
- [x] Templates genéricos para otros lenguajes ✅

---

## 🚧 **PENDIENTES CRÍTICOS**

> **📋 Ver detalles completos en:** [`docs/PENDIENTES.md`](./PENDIENTES.md)

### **🔥 ALTA PRIORIDAD - Deployment Automático Completo**

#### **Phase 1: Funciones Críticas (1-2 horas)**
- [ ] **Detección real de lenguajes** - Actualmente hardcodeado a "go"
- [ ] **Conectar templates existentes** - No usa los templates implementados
- [ ] **Asignación real de puertos** - Números aleatorios sin verificación
- [ ] **Health checks** - Verificar que la app funcione después del deploy
- [ ] **Testing de integración** - Probar flujo completo

#### **Resultado:** 
Una vez completado, el sistema podrá:
```bash
# INPUT: URL de GitHub
curl -X POST http://localhost:8080/api/v1/deploy \
  -d '{"repo_url": "https://github.com/user/app.git", "name": "mi-app"}'

# OUTPUT: App funcionando en puerto automático (10-30 segundos)
{
  "port": 3847,
  "url": "http://localhost:3847",
  "status": "running",
  "health": "healthy"
}
```

---

## 🎯 **ROADMAP FUTURO**

### **v1.1 - Deployment Automático Completo** ⏱️ *1-2 horas*
- [ ] Implementar funciones críticas de deployment
- [ ] Testing completo del flujo
- [ ] Documentación de uso

### **v1.2 - Mejoras de Experiencia** ⏱️ *1-2 días*
- [ ] Logs mejorados durante deployment
- [ ] Rollback automático en caso de fallo
- [ ] Métricas básicas de aplicaciones
- [ ] Limpieza automática de recursos

### **v1.3 - Funcionalidades Avanzadas** ⏱️ *1-2 semanas*
- [ ] Variables de entorno por aplicación
- [ ] Configuración de recursos (CPU/RAM)
- [ ] Hooks de deployment (pre/post)
- [ ] Integración con webhooks de GitHub

### **v1.4 - Escalabilidad** ⏱️ *2-4 semanas*
- [ ] Múltiples instancias por aplicación
- [ ] Load balancing básico
- [ ] Persistent volumes
- [ ] Backup automático de aplicaciones

### **v2.0 - Distribución** ⏱️ *1-3 meses*
- [ ] Soporte para múltiples nodos
- [ ] Clustering básico
- [ ] Dashboard multi-nodo
- [ ] Sincronización de estado

---

## 📊 **Métricas del Proyecto**

### **Líneas de Código (Estimado)**
- **Go Backend:** ~8,000 líneas
- **Templates:** ~2,000 líneas
- **Frontend:** ~1,500 líneas
- **Configuración:** ~500 líneas
- **Total:** ~12,000 líneas

### **Arquitectura**
- **Runtimes soportados:** 3 (LXC, Docker, containerd)
- **Lenguajes soportados:** 4+ (Go, Node.js, Python, Rust, Generic)
- **Endpoints API:** 10+
- **Funciones core:** 50+

### **Testing**
- **Scripts de test:** 5 scripts
- **Cobertura estimada:** 70-80%
- **Platforms testadas:** macOS, Linux

---

## 🎪 **Casos de Uso Objetivo**

### **Desarrollador Individual**
```bash
# Desarrollo local rápido
diplo deploy https://github.com/mi-usuario/mi-proyecto.git
# → App disponible en http://localhost:3847 en 30 segundos
```

### **Equipo Pequeño**
```bash
# Demo/staging rápido
diplo deploy https://github.com/empresa/producto.git --name staging
# → Staging disponible para demos inmediatas
```

### **Raspberry Pi / Home Lab**
```bash
# Self-hosted personal PaaS
diplo deploy https://github.com/personal/blog.git --runtime lxc
# → Blog personal con recursos mínimos
```

---

## 🔄 **Versionado y Releases**

### **Versionado Semántico**
- **Major:** Cambios incompatibles en API
- **Minor:** Nuevas funcionalidades compatibles
- **Patch:** Bug fixes y mejoras menores

### **Release Schedule**
- **v1.1:** Funcionalidades críticas (próximo release)
- **v1.x:** Releases mensuales con mejoras
- **v2.0:** Release mayor con distribución

---

## 📞 **Contacto y Contribución**

### **Prioridades de Desarrollo**
1. **Deployment automático completo** (v1.1) 🔥
2. **Mejoras de experiencia** (v1.2) 🔥
3. **Funcionalidades avanzadas** (v1.3) 🟡
4. **Escalabilidad** (v1.4) 🟡
5. **Distribución** (v2.0) 🟢

### **Herramientas de Desarrollo**
- **Lenguaje:** Go 1.24+
- **Base de datos:** SQLite + SQLC
- **Templates:** templ
- **Build:** Make
- **Testing:** Scripts bash + Go tests

---

**📅 Última actualización:** 2024-01-15  
**👤 Estado:** Listo para implementar funciones críticas  
**🎯 Próximo hito:** Deployment automático completo (v1.1) 