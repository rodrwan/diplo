# Diplo - PaaS Local en Go

Diplo es una plataforma como servicio (PaaS) local construida en Go, diseñada para desplegar y gestionar aplicaciones de forma sencilla.

## Características

- 🚀 **Deploy rápido** de aplicaciones
- 🐳 **Soporte para contenedores** (Docker y Containerd)
- 📊 **Monitoreo en tiempo real**
- 🔧 **Gestión de aplicaciones** desde interfaz web
- 🍓 **Optimizado para Raspberry Pi** con containerd
- 🔒 **Variables de entorno seguras** con cifrado

## Instalación

### Requisitos

- Go 1.21+
- SQLite3
- Docker o Containerd (recomendado para Raspberry Pi)

### Desarrollo Local

```bash
# Clonar repositorio
git clone <repository-url>
cd diplo

# Instalar dependencias
make deps

# Ejecutar en modo desarrollo
make dev
```

### Deploy en Raspberry Pi

#### Opción 1: Deploy Automático (Recomendado)

```bash
# Deploy completo con containerd optimizado para Raspberry Pi
make deploy-auto
```

Este comando:
- Compila el binario para ARM64
- Copia todos los scripts y documentación
- Configura containerd automáticamente
- Optimiza para Raspberry Pi

#### Opción 2: Deploy Manual

```bash
# Solo copiar archivos
make deploy

# Configurar containerd manualmente
make setup-containerd

# O configurar Docker
make setup-docker
```

#### Opción 3: Deploy con Docker

```bash
# Deploy completo con Docker
make deploy-full
```

## Comandos Disponibles

### Desarrollo
```bash
make build      # Compilar
make run        # Compilar y ejecutar
make dev        # Modo desarrollo
make debug      # Modo debug
make test       # Ejecutar tests
make clean      # Limpiar archivos
```

### Deploy en Raspberry Pi
```bash
make deploy-auto        # Deploy automático (RECOMENDADO)
make deploy             # Solo copiar archivos
make post-deploy        # Configuración post-deploy
make setup-containerd   # Instalar containerd
make setup-docker       # Instalar Docker
make diagnose-containerd # Diagnosticar containerd
```

### Gestión
```bash
make manage     # Gestión de Diplo
make help       # Ver todos los comandos
```

## Uso

### Acceso Web

Una vez ejecutado, accede a:
- **Local**: http://localhost:8080
- **Raspberry Pi**: http://raspberrypi.local:8080

### API Endpoints

```bash
# Deploy de aplicación
POST /api/unified/deploy
{
  "name": "mi-app",
  "repo_url": "https://github.com/usuario/repo",
  "runtime_type": "containerd"
}

# Estado del sistema
GET /api/status

# Diagnóstico de containerd
GET /api/containerd/diagnostic

# Instalar containerd (solo Raspberry Pi)
POST /api/containerd/install
```

## Configuración para Raspberry Pi

### ¿Por qué Containerd?

- **Mejor rendimiento**: Más ligero que Docker
- **Menor uso de memoria**: Ideal para recursos limitados
- **Optimizado para ARM**: Mejor soporte para arquitectura ARM
- **Inicio más rápido**: ~2-3 segundos vs 5-8 segundos de Docker

### Configuración Automática

El sistema detecta automáticamente que estás en Raspberry Pi y:
1. Prioriza containerd sobre Docker
2. Aplica optimizaciones específicas para ARM
3. Configura límites de memoria apropiados
4. Crea el namespace 'diplo' automáticamente

### Verificación

```bash
# En tu Raspberry Pi
./diplo-scripts/diagnose_containerd.sh
./diplo-scripts/verify_raspberry_containerd.sh
```

## Documentación

- [Configuración de Raspberry Pi](docs/RASPBERRY_PI_SETUP.md)
- [Arquitectura del Sistema](docs/ARCHITECTURE.md)
- [API Testing](docs/API_TESTING.md)

## Estructura del Proyecto

```
diplo/
├── cmd/diplo/           # Punto de entrada principal
├── internal/            # Código interno
│   ├── database/        # Capa de base de datos
│   ├── docker/          # Integración con Docker
│   ├── runtime/         # Sistema de runtimes (Docker/Containerd)
│   ├── server/          # Servidor HTTP y handlers
│   └── templates/       # Templates HTML
├── scripts/             # Scripts de instalación y gestión
├── docs/               # Documentación
└── Makefile            # Comandos de build y deploy
```

## Contribuir

1. Fork el proyecto
2. Crea una rama para tu feature (`git checkout -b feature/AmazingFeature`)
3. Commit tus cambios (`git commit -m 'Add some AmazingFeature'`)
4. Push a la rama (`git push origin feature/AmazingFeature`)
5. Abre un Pull Request

## Licencia

Este proyecto está bajo la Licencia MIT. Ver el archivo `LICENSE` para más detalles.

## Soporte

Si encuentras problemas:

1. Ejecuta el diagnóstico: `make diagnose-containerd`
2. Revisa los logs: `sudo journalctl -u containerd -f`
3. Verifica la configuración: `cat /etc/containerd/config.toml`
4. Reinicia el servicio: `sudo systemctl restart containerd`

---

**¡Diplo está optimizado para tu Raspberry Pi con containerd!** 🍓 