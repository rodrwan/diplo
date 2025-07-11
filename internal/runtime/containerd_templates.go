package runtime

import (
	"bytes"
	"fmt"
	texttemplate "text/template"
)

// ContainerdTemplate define un template para crear imágenes containerd
type ContainerdTemplate struct {
	Name        string
	BaseImage   string
	Dockerfile  string
	Command     []string
	Environment map[string]string
	Ports       []int
	Workdir     string
	User        string
	Labels      map[string]string
}

// ContainerdTemplateData contiene los datos para renderizar templates containerd
type ContainerdTemplateData struct {
	AppName     string
	AppID       string
	Port        int
	RepoURL     string
	Language    string
	Environment map[string]string
	Files       map[string]string // archivos adicionales que se pueden incluir
}

// ContainerdTemplateManager maneja los templates para containerd
type ContainerdTemplateManager struct {
	templates map[string]*ContainerdTemplate
}

// NewContainerdTemplateManager crea un nuevo gestor de templates containerd
func NewContainerdTemplateManager() *ContainerdTemplateManager {
	tm := &ContainerdTemplateManager{
		templates: make(map[string]*ContainerdTemplate),
	}

	// Inicializar templates por defecto
	tm.initDefaultTemplates()

	return tm
}

// initDefaultTemplates inicializa los templates por defecto para cada lenguaje
func (tm *ContainerdTemplateManager) initDefaultTemplates() {
	// Template para Go
	tm.templates["go"] = &ContainerdTemplate{
		Name:      "diplo-go",
		BaseImage: "golang:1.24-alpine",
		Dockerfile: `FROM {{.BaseImage}}

# Instalar dependencias del sistema
RUN apk update && apk add --no-cache \
    git \
    ca-certificates \
    build-base \
    && rm -rf /var/cache/apk/*

# Establecer directorio de trabajo
WORKDIR /app

# Copiar go.mod y go.sum si existen
COPY go.* ./

# Descargar dependencias
RUN if [ -f go.mod ]; then go mod download; fi

# Copiar código fuente
COPY . .

# Compilar aplicación
RUN go build -o main .

# Configurar usuario no root
RUN adduser -D -s /bin/sh appuser
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["./main"]
`,
		Command: []string{"./main"},
		Environment: map[string]string{
			"CGO_ENABLED": "0",
			"GOOS":        "linux",
		},
		Ports:   []int{8080},
		Workdir: "/app",
		User:    "appuser",
		Labels: map[string]string{
			"diplo.language": "go",
			"diplo.version":  "1.0",
		},
	}

	// Template para Node.js
	tm.templates["node"] = &ContainerdTemplate{
		Name:      "diplo-node",
		BaseImage: "node:22-alpine",
		Dockerfile: `FROM {{.BaseImage}}

# Instalar dependencias del sistema
RUN apk update && apk add --no-cache \
    git \
    python3 \
    make \
    g++ \
    && rm -rf /var/cache/apk/*

# Establecer directorio de trabajo
WORKDIR /app

# Copiar package.json y package-lock.json si existen
COPY package*.json ./

# Instalar dependencias de Node.js
RUN if [ -f package.json ]; then npm ci --only=production; fi

# Copiar código fuente
COPY . .

# Compilar si es necesario
RUN if [ -f package.json ] && npm run build >/dev/null 2>&1; then npm run build; fi

# Configurar usuario no root
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nextjs -u 1001 -G nodejs
USER nextjs

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["npm", "start"]
`,
		Command: []string{"npm", "start"},
		Environment: map[string]string{
			"NODE_ENV": "production",
		},
		Ports:   []int{3000},
		Workdir: "/app",
		User:    "nextjs",
		Labels: map[string]string{
			"diplo.language": "node",
			"diplo.version":  "1.0",
		},
	}

	// Template para Python
	tm.templates["python"] = &ContainerdTemplate{
		Name:      "diplo-python",
		BaseImage: "python:3.13-alpine",
		Dockerfile: `FROM {{.BaseImage}}

# Instalar dependencias del sistema
RUN apk update && apk add --no-cache \
    git \
    gcc \
    musl-dev \
    postgresql-dev \
    && rm -rf /var/cache/apk/*

# Establecer directorio de trabajo
WORKDIR /app

# Copiar requirements.txt si existe
COPY requirements.txt* ./

# Instalar dependencias de Python
RUN if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi

# Copiar código fuente
COPY . .

# Configurar usuario no root
RUN adduser -D -s /bin/sh appuser
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["python", "app.py"]
`,
		Command: []string{"python", "app.py"},
		Environment: map[string]string{
			"PYTHONUNBUFFERED":        "1",
			"PYTHONDONTWRITEBYTECODE": "1",
		},
		Ports:   []int{5000},
		Workdir: "/app",
		User:    "appuser",
		Labels: map[string]string{
			"diplo.language": "python",
			"diplo.version":  "1.0",
		},
	}

	// Template para Rust
	tm.templates["rust"] = &ContainerdTemplate{
		Name:      "diplo-rust",
		BaseImage: "rust:1.83-alpine",
		Dockerfile: `FROM {{.BaseImage}} AS builder

# Instalar dependencias del sistema
RUN apk update && apk add --no-cache \
    git \
    musl-dev \
    && rm -rf /var/cache/apk/*

# Establecer directorio de trabajo
WORKDIR /app

# Copiar Cargo.toml y Cargo.lock si existen
COPY Cargo.* ./

# Crear directorio src temporal para cache de dependencias
RUN mkdir src && echo "fn main() {}" > src/main.rs

# Compilar dependencias (cache layer)
RUN if [ -f Cargo.toml ]; then cargo build --release && rm -rf src; fi

# Copiar código fuente
COPY . .

# Compilar aplicación
RUN cargo build --release

# Runtime stage
FROM alpine:latest

# Instalar dependencias de runtime
RUN apk --no-cache add ca-certificates

# Configurar usuario no root
RUN adduser -D -s /bin/sh appuser
USER appuser

# Establecer directorio de trabajo
WORKDIR /home/appuser

# Copiar binario compilado
COPY --from=builder /app/target/release/{{.AppName}} ./app

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["./app"]
`,
		Command:     []string{"./app"},
		Environment: map[string]string{},
		Ports:       []int{8000},
		Workdir:     "/home/appuser",
		User:        "appuser",
		Labels: map[string]string{
			"diplo.language": "rust",
			"diplo.version":  "1.0",
		},
	}

	// Template genérico para otros lenguajes
	tm.templates["generic"] = &ContainerdTemplate{
		Name:      "diplo-generic",
		BaseImage: "ubuntu:22.04",
		Dockerfile: `FROM {{.BaseImage}}

# Instalar dependencias básicas
RUN apt-get update && apt-get install -y \
    git \
    curl \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Establecer directorio de trabajo
WORKDIR /app

# Copiar código fuente
COPY . .

# Configurar usuario no root
RUN useradd -m -s /bin/bash appuser && \
    chown -R appuser:appuser /app
USER appuser

# Exponer puerto
EXPOSE {{.Port}}

# Comando por defecto
CMD ["bash"]
`,
		Command:     []string{"bash"},
		Environment: map[string]string{},
		Ports:       []int{8080},
		Workdir:     "/app",
		User:        "appuser",
		Labels: map[string]string{
			"diplo.language": "generic",
			"diplo.version":  "1.0",
		},
	}
}

// GetTemplate obtiene un template por lenguaje
func (tm *ContainerdTemplateManager) GetTemplate(language string) (*ContainerdTemplate, bool) {
	template, exists := tm.templates[language]
	return template, exists
}

// RenderDockerfile renderiza un Dockerfile usando el template y los datos
func (tm *ContainerdTemplateManager) RenderDockerfile(language string, data *ContainerdTemplateData) (string, error) {
	template, exists := tm.GetTemplate(language)
	if !exists {
		// Usar template genérico si no existe uno específico
		template, _ = tm.GetTemplate("generic")
	}

	// Preparar datos para renderizar
	renderData := struct {
		*ContainerdTemplateData
		BaseImage string
	}{
		ContainerdTemplateData: data,
		BaseImage:              template.BaseImage,
	}

	return tm.renderTemplate(template.Dockerfile, renderData)
}

// GetImageName genera el nombre de la imagen para el contenedor
func (tm *ContainerdTemplateManager) GetImageName(language, appName string) string {
	template, exists := tm.GetTemplate(language)
	if !exists {
		template, _ = tm.GetTemplate("generic")
	}

	return fmt.Sprintf("%s:%s", template.Name, appName)
}

// GetContainerConfig obtiene la configuración del contenedor para un lenguaje
func (tm *ContainerdTemplateManager) GetContainerConfig(language string, data *ContainerdTemplateData) *CreateContainerRequest {
	template, exists := tm.GetTemplate(language)
	if !exists {
		template, _ = tm.GetTemplate("generic")
	}

	// Combinar environment variables del template con las del usuario
	environment := make(map[string]string)
	for k, v := range template.Environment {
		environment[k] = v
	}
	for k, v := range data.Environment {
		environment[k] = v
	}

	// Configurar puertos
	var ports []PortMapping
	if data.Port > 0 {
		ports = append(ports, PortMapping{
			HostPort:      data.Port,
			ContainerPort: template.Ports[0],
			Protocol:      "tcp",
		})
	}

	// Combinar labels del template con metadata del usuario
	labels := make(map[string]string)
	for k, v := range template.Labels {
		labels[k] = v
	}
	labels["diplo.app"] = data.AppName
	labels["diplo.app-id"] = data.AppID

	return &CreateContainerRequest{
		Name:          data.AppName,
		Image:         tm.GetImageName(language, data.AppName),
		Command:       template.Command,
		WorkingDir:    template.Workdir,
		Environment:   environment,
		Labels:        labels,
		Ports:         ports,
		NetworkMode:   "bridge",
		RestartPolicy: "unless-stopped",
		AutoRemove:    false,
		Privileged:    false,
		Metadata: map[string]interface{}{
			"language": language,
			"template": template.Name,
			"repo_url": data.RepoURL,
		},
	}
}

// renderTemplate renderiza un template de texto con los datos proporcionados
func (tm *ContainerdTemplateManager) renderTemplate(templateStr string, data interface{}) (string, error) {
	tmpl, err := texttemplate.New("containerd").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template: %w", err)
	}

	return buf.String(), nil
}

// ListTemplates devuelve una lista de todos los templates disponibles
func (tm *ContainerdTemplateManager) ListTemplates() []string {
	var templates []string
	for name := range tm.templates {
		templates = append(templates, name)
	}
	return templates
}

// AddTemplate añade un nuevo template personalizado
func (tm *ContainerdTemplateManager) AddTemplate(name string, template *ContainerdTemplate) {
	tm.templates[name] = template
}

// UpdateTemplate actualiza un template existente
func (tm *ContainerdTemplateManager) UpdateTemplate(name string, template *ContainerdTemplate) error {
	if _, exists := tm.templates[name]; !exists {
		return fmt.Errorf("template %s not found", name)
	}
	tm.templates[name] = template
	return nil
}

// DeleteTemplate elimina un template
func (tm *ContainerdTemplateManager) DeleteTemplate(name string) error {
	if _, exists := tm.templates[name]; !exists {
		return fmt.Errorf("template %s not found", name)
	}
	delete(tm.templates, name)
	return nil
}

// ValidateTemplate valida que un template esté correctamente configurado
func (tm *ContainerdTemplateManager) ValidateTemplate(template *ContainerdTemplate) error {
	if template.Name == "" {
		return fmt.Errorf("template name cannot be empty")
	}

	if template.BaseImage == "" {
		return fmt.Errorf("base image cannot be empty")
	}

	if template.Dockerfile == "" {
		return fmt.Errorf("dockerfile cannot be empty")
	}

	if len(template.Command) == 0 {
		return fmt.Errorf("command cannot be empty")
	}

	// Validar que el Dockerfile sea parseable como template
	_, err := texttemplate.New("test").Parse(template.Dockerfile)
	if err != nil {
		return fmt.Errorf("invalid dockerfile template: %w", err)
	}

	return nil
}

// GetTemplateInfo devuelve información detallada sobre un template
func (tm *ContainerdTemplateManager) GetTemplateInfo(language string) (map[string]interface{}, error) {
	template, exists := tm.GetTemplate(language)
	if !exists {
		return nil, fmt.Errorf("template for language %s not found", language)
	}

	return map[string]interface{}{
		"name":        template.Name,
		"base_image":  template.BaseImage,
		"command":     template.Command,
		"environment": template.Environment,
		"ports":       template.Ports,
		"workdir":     template.Workdir,
		"user":        template.User,
		"labels":      template.Labels,
	}, nil
}
