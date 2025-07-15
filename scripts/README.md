# Scripts de Diplo

Este directorio contiene los scripts necesarios para la gestión y configuración de Diplo.

## 📁 Scripts Disponibles

### **Scripts de Gestión**

#### `manage_diplo.sh`
Script principal de gestión para Diplo en Raspberry Pi.

**Uso:**
```bash
# Gestión de Diplo
./manage_diplo.sh start       # Iniciar Diplo
./manage_diplo.sh stop        # Detener Diplo
./manage_diplo.sh restart     # Reiniciar Diplo
./manage_diplo.sh status      # Ver estado
./manage_diplo.sh logs        # Ver logs

# Deploy
./manage_diplo.sh deploy      # Deploy básico
./manage_diplo.sh deploy-full # Deploy completo

# LXC
./manage_diplo.sh setup-lxc   # Configurar LXC
./manage_diplo.sh verify      # Verificar LXC
./manage_diplo.sh test        # Probar sistema
./manage_diplo.sh list        # Listar contenedores
./manage_diplo.sh cleanup     # Limpiar contenedores

# Diagnóstico
./manage_diplo.sh health      # Verificar salud del sistema
```

### **Scripts de Instalación**

#### `install_diplo_raspberry.sh`
Script de instalación LXC para Raspberry Pi.

**Propósito:**
- Instala y configura LXC para auto-provisionamiento
- Configura red LXC y cgroups
- Crea scripts de verificación y prueba

**Uso:**
```bash
# Ejecutar en Raspberry Pi
./install_diplo_raspberry.sh
```

### **Scripts de Verificación**

#### `verify_lxc_setup.sh`
Script de verificación de configuración LXC.

**Propósito:**
- Verifica que LXC esté instalado correctamente
- Verifica configuración de red y cgroups
- Verifica permisos de usuario

**Uso:**
```bash
# Ejecutar en Raspberry Pi
./verify_lxc_setup.sh
```

### **Scripts de Prueba**

#### `test_hybrid_system.sh`
Script de prueba del sistema híbrido.

**Propósito:**
- Prueba la integración entre Diplo y LXC
- Verifica auto-provisionamiento
- Prueba creación y gestión de contenedores

**Uso:**
```bash
# Ejecutar en Raspberry Pi
./test_hybrid_system.sh
```

#### `test_docker_build.sh`
Script de prueba de builds Docker.

**Propósito:**
- Prueba la funcionalidad de build de imágenes Docker
- Verifica integración con Docker API
- Prueba templates de Dockerfile

**Uso:**
```bash
# Ejecutar localmente
./test_docker_build.sh
```

## 🔄 Flujo de Trabajo

### **Deploy Completo**
```bash
make deploy-full
```

### **Deploy Modular**
```bash
make deploy           # Solo binario + scripts
make manage setup-lxc # Configurar LXC
make manage start     # Iniciar Diplo
```

### **Gestión Diaria**
```bash
make manage health    # Verificar estado
make manage start     # Iniciar
make manage logs      # Ver logs
make manage stop      # Detener
```

## 📋 Estructura en Raspberry Pi

Después del deploy, los scripts se copian a:

```
~/Mangoticket/
├── diplo-rpi          # Binario de Diplo

~/diplo-scripts/
├── install_diplo_raspberry.sh
├── verify_lxc_setup.sh
└── test_hybrid_system.sh
```

## 🧹 Mantenimiento

Los scripts están diseñados para ser:
- **Modulares** - Cada uno tiene una función específica
- **Reutilizables** - Se pueden ejecutar múltiples veces
- **Seguros** - Incluyen verificaciones y manejo de errores
- **Documentados** - Incluyen mensajes informativos 