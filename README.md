# Diplo - PaaS Local en Go

Diplo es una plataforma como servicio (PaaS) local escrita en Go que permite desplegar aplicaciones desde repositorios Git usando contenedores Docker.

## Características

- 🚀 **Deployment automático** desde repositorios Git
- 🐳 **Integración nativa con Docker** usando la API oficial
- 📊 **Base de datos SQLite** para persistencia
- 🔄 **Shutdown graceful** con manejo de señales
- 🌐 **API REST** con soporte CORS
- 📝 **Logging estructurado** con logrus
- 🎯 **Detección automática de lenguajes** (Go, Node.js, Python)

## Requisitos

- Go 1.21 o superior
- Docker Engine
- Git

## Instalación

### Desde el código fuente

```bash
# Clonar el repositorio
git clone https://github.com/rodrwan/diplo.git
cd diplo

# Instalar dependencias
make deps

# Compilar
make build

# Ejecutar
make run
```

### Desarrollo

```bash
# Ejecutar en modo desarrollo
make dev
```

## Uso

### API Endpoints

#### Health Check
```bash
curl http://localhost:8080/
```

#### Deploy Application
```bash
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/user/my-app.git",
    "name": "my-app"
  }'
```

#### List Applications
```bash
curl http://localhost:8080/api/v1/apps
```

#### Get Application
```bash
curl http://localhost:8080/api/v1/apps/{app-id}
```

#### Delete Application
```bash
curl -X DELETE http://localhost:8080/api/v1/apps/{app-id}
```

### Ejemplo de uso

1. **Desplegar una aplicación Go:**
```bash
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/rodrwan/simple-go-app.git"
  }'
```

2. **Verificar el estado:**
```bash
curl http://localhost:8080/api/v1/apps
```

3. **Acceder a la aplicación:**
```bash
# La aplicación estará disponible en http://localhost:{puerto-asignado}
```

## Estructura del Proyecto

```
diplo/
├── cmd/diplo/          # Punto de entrada de la aplicación
├── internal/
│   ├── database/       # Capa de base de datos SQLite
│   ├── docker/         # Cliente Docker
│   ├── models/         # Modelos de datos
│   └── server/         # Servidor HTTP y handlers
├── scripts/            # Scripts de utilidad
├── docs/              # Documentación
├── go.mod             # Dependencias Go
├── Makefile           # Comandos de build
└── README.md          # Este archivo
```

## Configuración

### Variables de Entorno

- `DIPLO_HOST` - Host del servidor (default: 0.0.0.0)
- `DIPLO_PORT` - Puerto del servidor (default: 8080)
- `DIPLO_DB_PATH` - Ruta de la base de datos (default: diplo.db)

### Docker

El servidor necesita acceso al socket de Docker para gestionar contenedores:

```bash
# Asegúrate de que el usuario tenga permisos para acceder al socket de Docker
sudo usermod -aG docker $USER
```

## Desarrollo

### Comandos útiles

```bash
# Instalar dependencias
make deps

# Ejecutar tests
make test

# Limpiar archivos generados
make clean

# Ver todos los comandos disponibles
make help
```

### Agregar nuevos lenguajes

Para agregar soporte para un nuevo lenguaje:

1. Modificar `internal/server/server.go` en la función `generateDockerfile()`
2. Agregar el template de Dockerfile correspondiente
3. Actualizar la función `detectLanguage()` para detectar el nuevo lenguaje

## Arquitectura

### Componentes principales

- **Server**: Maneja las requests HTTP y coordina los deployments
- **Database**: Persistencia de aplicaciones y logs
- **Docker Client**: Comunicación con la API de Docker
- **Models**: Estructuras de datos para aplicaciones

### Flujo de deployment

1. **Recepción de request** → Validación de datos
2. **Creación de aplicación** → Asignación de puerto
3. **Detección de lenguaje** → Análisis del repositorio
4. **Generación de Dockerfile** → Template según lenguaje
5. **Build de imagen** → Construcción via Docker API
6. **Ejecución de contenedor** → Deployment en puerto asignado
7. **Actualización de estado** → Persistencia en BD

## Contribuir

1. Fork el proyecto
2. Crear una rama para tu feature (`git checkout -b feature/AmazingFeature`)
3. Commit tus cambios (`git commit -m 'Add some AmazingFeature'`)
4. Push a la rama (`git push origin feature/AmazingFeature`)
5. Abrir un Pull Request

## Licencia

Este proyecto está bajo la Licencia MIT. Ver el archivo `LICENSE` para más detalles.

## Roadmap

- [ ] Soporte para más lenguajes (Java, Rust, PHP)
- [ ] Webhooks para notificaciones
- [ ] Métricas y monitoreo
- [ ] Autoscaling basado en carga
- [ ] Volúmenes persistentes
- [ ] Variables de entorno
- [ ] Logs en tiempo real
- [ ] Dashboard web 