# Diplo - PaaS Local en C

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![C](https://img.shields.io/badge/C-99-blue.svg)](https://en.wikipedia.org/wiki/C99)
[![Docker](https://img.shields.io/badge/Docker-Required-blue.svg)](https://www.docker.com/)

**Diplo** es un **Platform as a Service (PaaS) local** desarrollado en **C puro** que permite desplegar aplicaciones web desde repositorios Git automáticamente. Funciona como un daemon que expone una API REST y gestiona contenedores Docker.

## 🚀 **Características**

- ✅ **API REST completa** para gestión de aplicaciones
- ✅ **Deployment automático** desde repositorios Git
- ✅ **Soporte multi-lenguaje** (Go 1.24, Node.js 18, Python 3.11)
- ✅ **Asignación automática de puertos** (3000-9999)
- ✅ **Base de datos SQLite** para persistencia
- ✅ **Logging estructurado** de todos los eventos
- ✅ **Gestión de contenedores Docker** automática
- ✅ **URLs automáticas** para acceso a aplicaciones

## 📋 **Requisitos**

### **Sistema Operativo:**
- macOS (desarrollado en)
- Linux (Raspberry Pi OS compatible)

### **Dependencias:**
- **GCC** (compilador C)
- **Docker** (para contenedores)
- **Git** (para clonado de repositorios)

### **Bibliotecas Externas:**
- `libmicrohttpd` - Servidor HTTP
- `jansson` - Parsing JSON
- `sqlite3` - Base de datos
- `pthread` - Threading (incluida en sistema)

## 🛠️ **Instalación**

### **1. Clonar el repositorio:**
```bash
git clone https://github.com/rodrwan/diplo.git
cd diplo
```

### **2. Instalar dependencias:**

#### **En macOS:**
```bash
# Instalar Homebrew si no está instalado
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Instalar dependencias
brew install libmicrohttpd jansson sqlite3
```

#### **En Linux (Raspberry Pi):**
```bash
sudo apt-get update
sudo apt-get install -y libmicrohttpd-dev libjansson-dev libsqlite3-dev
```

### **3. Compilar el proyecto:**
```bash
make
```

### **4. Verificar la instalación:**
```bash
./bin/diplo --help
```

## 🚀 **Uso Rápido**

### **1. Iniciar el servidor:**
```bash
./bin/diplo
```

El servidor se iniciará en `http://localhost:8080`

### **2. Desplegar una aplicación:**
```bash
curl -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/usuario/proyecto-go.git",
    "name": "mi-app"
  }'
```

### **3. Ver aplicaciones desplegadas:**
```bash
curl http://localhost:8080/apps
```

### **4. Acceder a tu aplicación:**
La respuesta del deployment incluirá una URL como:
```
http://localhost:3456
```

## 📚 **API REST**

### **Endpoints Disponibles:**

#### **POST /deploy**
Crear y desplegar una nueva aplicación.

**Body:**
```json
{
  "repo_url": "https://github.com/usuario/repo.git",
  "name": "opcional"
}
```

**Respuesta:**
```json
{
  "id": "app_1234567890_123456",
  "name": "mi-aplicacion",
  "repo_url": "https://github.com/usuario/repo.git",
  "port": 3456,
  "url": "http://localhost:3456",
  "status": "deploying",
  "message": "Aplicación creada y deployment iniciado"
}
```

#### **GET /apps**
Listar todas las aplicaciones.

#### **GET /apps/{id}**
Obtener detalles de una aplicación específica.

#### **DELETE /apps/{id}**
Eliminar una aplicación y su contenedor.

#### **GET /**
Health check del servidor.

## 🐳 **Lenguajes Soportados**

### **Go 1.24**
- Multi-stage build optimizado
- CGO deshabilitado para imágenes más pequeñas
- Soporte para `go.mod` y `go.sum`

### **Node.js 18**
- Soporte para `package.json`
- Instalación de dependencias con `npm ci`
- Comando `npm start` por defecto

### **Python 3.11**
- Soporte para `requirements.txt`
- Entorno virtual automático
- Comando `python app.py` por defecto

## 📊 **Base de Datos**

Diplo utiliza SQLite para persistencia. Los datos se almacenan en `diplo.db`:

### **Tablas:**
- **`apps`** - Información de aplicaciones desplegadas
- **`deployment_logs`** - Historial de eventos de deployment

### **Consultas útiles:**
```bash
# Ver todas las aplicaciones
sqlite3 diplo.db "SELECT * FROM apps;"

# Ver logs de deployment
sqlite3 diplo.db "SELECT * FROM deployment_logs ORDER BY timestamp DESC;"

# Ver aplicaciones en estado 'running'
sqlite3 diplo.db "SELECT * FROM apps WHERE status = 'running';"
```

## 🔧 **Configuración**

### **Variables de Entorno:**
```bash
export DIPLO_PORT=8080          # Puerto del servidor
export DIPLO_DB_PATH="diplo.db" # Archivo de base de datos
```

### **Constantes en el código:**
```c
#define DIPLO_PORT 8080           // Puerto del servidor
#define DIPLO_DB_PATH "diplo.db"  // Archivo de BD
#define MIN_PORT 3000             // Puerto mínimo para apps
#define MAX_PORT 9999             // Puerto máximo para apps
```

## 🛠️ **Comandos Make**

```bash
make          # Compilar el proyecto
make clean    # Limpiar archivos generados
make run      # Compilar y ejecutar
make debug    # Compilar con símbolos de debug
make install  # Instalar en /usr/local/bin
make uninstall # Desinstalar
```

## 🐛 **Debugging**

### **Logs del servidor:**
```bash
# Ver logs en tiempo real
./bin/diplo 2>&1 | tee diplo.log

# Ver logs de contenedor específico
docker logs <container_id>
```

### **Verificar estado del sistema:**
```bash
# Ver contenedores Docker
docker ps

# Ver imágenes Docker
docker images | grep diplo

# Ver puertos en uso
netstat -tulpn | grep :3000
```

## 📁 **Estructura del Proyecto**

```
diplo/
├── src/                    # Código fuente
│   ├── main.c             # Punto de entrada
│   ├── handlers.c         # Endpoints REST
│   ├── database.c         # Operaciones SQLite
│   ├── docker.c           # Gestión Docker
│   └── utils.c            # Utilidades
├── include/               # Headers
│   └── diplo.h           # Header principal
├── docs/                  # Documentación
│   ├── ARCHITECTURE.md    # Arquitectura del sistema
│   ├── CHANGELOG.md       # Historial de cambios
│   └── ROADMAP.md         # Roadmap técnico
├── bin/                   # Ejecutables (generado)
├── obj/                   # Objetos compilados (generado)
├── Makefile               # Sistema de build
├── .gitignore            # Archivos a ignorar
└── README.md             # Este archivo
```

## 🤝 **Contribuir**

### **Desarrollo:**
1. Fork el repositorio
2. Crear una rama para tu feature (`git checkout -b feature/nueva-funcionalidad`)
3. Commit tus cambios (`git commit -am 'Agregar nueva funcionalidad'`)
4. Push a la rama (`git push origin feature/nueva-funcionalidad`)
5. Crear un Pull Request

### **Reportar Bugs:**
- Usar el sistema de Issues de GitHub
- Incluir información del sistema y pasos para reproducir

## 📄 **Licencia**

Este proyecto está bajo la Licencia MIT. Ver el archivo `LICENSE` para más detalles.

## 🙏 **Agradecimientos**

- **libmicrohttpd** - Servidor HTTP ligero
- **jansson** - Biblioteca JSON para C
- **SQLite** - Base de datos embebida
- **Docker** - Contenedores y orquestación

## 📞 **Contacto**

- **Autor:** Rodrigo Wan
- **GitHub:** [@rodrwan](https://github.com/rodrwan)
- **Proyecto:** [Diplo](https://github.com/rodrwan/diplo)

---

**Diplo** - Tu PaaS local en C puro 🚀 