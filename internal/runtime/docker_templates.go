package runtime

import (
	"fmt"
	"strings"
	texttemplate "text/template"
)

// DockerTemplate representa un template de Docker para un lenguaje específico
type DockerTemplate struct {
	Language    string
	BaseImage   string
	BuildSteps  []string
	RunCommand  string
	ExposedPort int
	WorkDir     string
	Template    string
}

// DockerTemplateManager maneja los templates de Docker
type DockerTemplateManager struct {
	templates map[string]*DockerTemplate
}

// NewDockerTemplateManager crea un nuevo manager de templates Docker
func NewDockerTemplateManager() *DockerTemplateManager {
	manager := &DockerTemplateManager{
		templates: make(map[string]*DockerTemplate),
	}
	manager.initializeTemplates()
	return manager
}

// initializeTemplates inicializa los templates predefinidos
func (tm *DockerTemplateManager) initializeTemplates() {
	// Template para Go
	tm.templates["go"] = &DockerTemplate{
		Language:    "go",
		BaseImage:   "golang:1.24-alpine",
		BuildSteps:  []string{"go mod download", "go build -o app ."},
		RunCommand:  "./app",
		ExposedPort: 8080,
		WorkDir:     "/app",
		Template: `# Multi-stage build para Go
FROM golang:1.24-alpine AS builder

# Instalar dependencias del sistema
RUN apk add --no-cache git ca-certificates

# Configurar directorio de trabajo
WORKDIR /app

# Clonar repositorio
RUN git clone {{.RepoURL}} .

# Descargar dependencias
RUN go mod download

# Compilar aplicación
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

# Imagen final
FROM alpine:3.18

# Instalar certificados SSL
RUN apk --no-cache add ca-certificates

# Crear usuario no privilegiado
RUN adduser -D -s /bin/sh appuser

# Configurar directorio de trabajo
WORKDIR /app

# Copiar binario desde builder
COPY --from=builder /app/app .

# Cambiar propietario
RUN chown -R appuser:appuser /app

# Cambiar a usuario no privilegiado
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["./app"]
`,
	}

	// Template para Node.js
	tm.templates["javascript"] = &DockerTemplate{
		Language:    "javascript",
		BaseImage:   "node:22-alpine",
		BuildSteps:  []string{"npm ci --only=production"},
		RunCommand:  "npm start",
		ExposedPort: 3000,
		WorkDir:     "/app",
		Template: `# Multi-stage build para Node.js
FROM node:22-alpine AS builder

# Instalar dependencias del sistema
RUN apk add --no-cache git

# Configurar directorio de trabajo
WORKDIR /app

# Clonar repositorio
RUN git clone {{.RepoURL}} .

# Instalar dependencias (incluyendo dev)
RUN npm ci

# Compilar aplicación (si aplica)
RUN npm run build || true

# Imagen final
FROM node:22-alpine

# Instalar dumb-init para manejo de señales
RUN apk add --no-cache dumb-init

# Crear usuario no privilegiado
RUN adduser -D -s /bin/sh appuser

# Configurar directorio de trabajo
WORKDIR /app

# Copiar aplicación desde builder
COPY --from=builder /app .

# Instalar solo dependencias de producción
RUN npm ci --only=production && npm cache clean --force

# Cambiar propietario
RUN chown -R appuser:appuser /app

# Cambiar a usuario no privilegiado
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
ENTRYPOINT ["dumb-init", "--"]
CMD ["npm", "start"]
`,
	}

	// Template para Python
	tm.templates["python"] = &DockerTemplate{
		Language:    "python",
		BaseImage:   "python:3.13-alpine",
		BuildSteps:  []string{"pip install -r requirements.txt"},
		RunCommand:  "python app.py",
		ExposedPort: 5000,
		WorkDir:     "/app",
		Template: `# Multi-stage build para Python
FROM python:3.13-alpine AS builder

# Instalar dependencias del sistema para compilación
RUN apk add --no-cache git gcc musl-dev libffi-dev

# Configurar directorio de trabajo
WORKDIR /app

# Clonar repositorio
RUN git clone {{.RepoURL}} .

# Instalar dependencias en directorio local
RUN pip install --user -r requirements.txt

# Imagen final
FROM python:3.13-alpine

# Instalar dependencias runtime
RUN apk add --no-cache libffi

# Crear usuario no privilegiado
RUN adduser -D -s /bin/sh appuser

# Configurar directorio de trabajo
WORKDIR /app

# Copiar dependencias instaladas
COPY --from=builder /root/.local /home/appuser/.local

# Copiar código fuente
COPY --from=builder /app .

# Cambiar propietario
RUN chown -R appuser:appuser /app

# Cambiar a usuario no privilegiado
USER appuser

# Configurar PATH
ENV PATH=/home/appuser/.local/bin:$PATH

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["python", "app.py"]
`,
	}

	// Template para Rust
	tm.templates["rust"] = &DockerTemplate{
		Language:    "rust",
		BaseImage:   "rust:1.83-alpine",
		BuildSteps:  []string{"cargo build --release"},
		RunCommand:  "./target/release/app",
		ExposedPort: 8080,
		WorkDir:     "/app",
		Template: `# Multi-stage build para Rust
FROM rust:1.83-alpine AS builder

# Instalar dependencias del sistema
RUN apk add --no-cache git musl-dev

# Configurar directorio de trabajo
WORKDIR /app

# Clonar repositorio
RUN git clone {{.RepoURL}} .

# Compilar aplicación
RUN cargo build --release

# Imagen final
FROM alpine:3.18

# Instalar certificados SSL
RUN apk --no-cache add ca-certificates

# Crear usuario no privilegiado
RUN adduser -D -s /bin/sh appuser

# Configurar directorio de trabajo
WORKDIR /app

# Copiar binario
COPY --from=builder /app/target/release/* .

# Cambiar propietario
RUN chown -R appuser:appuser /app

# Cambiar a usuario no privilegiado
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["./app"]
`,
	}

	// Template genérico
	tm.templates["generic"] = &DockerTemplate{
		Language:    "generic",
		BaseImage:   "ubuntu:22.04",
		BuildSteps:  []string{"echo 'Generic build'"},
		RunCommand:  "echo 'Generic run'",
		ExposedPort: 8080,
		WorkDir:     "/app",
		Template: `# Imagen genérica para aplicaciones
FROM ubuntu:22.04

# Instalar dependencias básicas
RUN apt-get update && apt-get install -y \
    git \
    curl \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Crear usuario no privilegiado
RUN useradd -m -s /bin/bash appuser

# Configurar directorio de trabajo
WORKDIR /app

# Clonar repositorio
RUN git clone {{.RepoURL}} .

# Cambiar propietario
RUN chown -R appuser:appuser /app

# Cambiar a usuario no privilegiado
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["echo", "Aplicación Docker genérica iniciada"]
`,
	}
}

// GetTemplate obtiene un template para un lenguaje específico
func (tm *DockerTemplateManager) GetTemplate(language string) (*DockerTemplate, error) {
	// Normalizar el nombre del lenguaje
	language = strings.ToLower(language)

	// Mapear variaciones de nombres
	switch language {
	case "node", "nodejs", "js":
		language = "javascript"
	case "py":
		language = "python"
	case "rs":
		language = "rust"
	case "golang":
		language = "go"
	}

	template, exists := tm.templates[language]
	if !exists {
		// Devolver template genérico si no existe uno específico
		return tm.templates["generic"], nil
	}

	return template, nil
}

// RenderTemplate renderiza un template con los parámetros dados
func (tm *DockerTemplateManager) RenderTemplate(language string, port int, repoURL string) (string, error) {
	template, err := tm.GetTemplate(language)
	if err != nil {
		return "", fmt.Errorf("error obteniendo template: %w", err)
	}

	// Crear template de Go
	tmpl, err := texttemplate.New("dockerfile").Parse(template.Template)
	if err != nil {
		return "", fmt.Errorf("error parseando template: %w", err)
	}

	// Preparar datos para el template
	data := struct {
		Port    int
		RepoURL string
	}{
		Port:    port,
		RepoURL: repoURL,
	}

	// Renderizar template
	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("error renderizando template: %w", err)
	}

	return rendered.String(), nil
}
