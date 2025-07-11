package runtime

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
)

// LXCTemplate define un template para configurar un container LXC
type LXCTemplate struct {
	Name              string
	Language          string
	BaseDistro        string
	Release           string
	Packages          []string
	PreInstallScript  string
	PostInstallScript string
	Environment       map[string]string
	Config            map[string]string
	ExposedPort       int
}

// LXCTemplateData contiene los datos para renderizar templates LXC
type LXCTemplateData struct {
	AppName     string
	AppID       string
	Port        int
	RepoURL     string
	Language    string
	Packages    []string
	Environment map[string]string
}

// LXCTemplateManager maneja los templates LXC
type LXCTemplateManager struct {
	templates map[string]*LXCTemplate
}

// NewLXCTemplateManager crea un nuevo gestor de templates LXC
func NewLXCTemplateManager() *LXCTemplateManager {
	tm := &LXCTemplateManager{
		templates: make(map[string]*LXCTemplate),
	}

	// Inicializar templates por defecto
	tm.initializeTemplates()

	return tm
}

// initializeTemplates inicializa los templates por defecto para cada lenguaje
func (tm *LXCTemplateManager) initializeTemplates() {
	// Template para Go
	tm.templates["go"] = &LXCTemplate{
		Name:       "go",
		Language:   "go",
		BaseDistro: "ubuntu",
		Release:    "focal",
		Packages: []string{
			"golang-go",
			"git",
			"ca-certificates",
			"build-essential",
			"curl",
			"wget",
		},
		PreInstallScript: `#!/bin/bash
# Pre-install script para Go
echo "Configurando entorno para Go..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
`,
		PostInstallScript: `#!/bin/bash
# Post-install script para Go
echo "Configurando Go..."

# Configurar GOPATH y PATH
export GOPATH=/go
export PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

# Crear directorios de Go
mkdir -p "$GOPATH/src" "$GOPATH/bin"
chmod -R 777 "$GOPATH"

# Crear directorio de aplicación
mkdir -p /app
chmod 777 /app

# Crear script de inicio
cat > /app/start.sh << 'EOF'
#!/bin/bash
cd /app
export GOPATH=/go
export PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

# Descargar dependencias
echo "Descargando dependencias..."
go mod tidy

# Compilar aplicación
echo "Compilando aplicación..."
go build -o app .

# Ejecutar aplicación
echo "Iniciando aplicación..."
./app
EOF

chmod +x /app/start.sh
echo "Configuración de Go completada"
`,
		Environment: map[string]string{
			"GOPATH":          "/go",
			"PATH":            "/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"DEBIAN_FRONTEND": "noninteractive",
		},
		Config: map[string]string{
			"lxc.arch":                   "amd64",
			"lxc.network.type":           "veth",
			"lxc.network.link":           "lxcbr0",
			"lxc.network.flags":          "up",
			"lxc.network.hwaddr":         "00:16:3e:xx:xx:xx",
			"lxc.mount.auto":             "proc:mixed sys:ro",
			"lxc.autodev":                "1",
			"lxc.kmsg":                   "0",
			"lxc.cap.drop":               "mac_admin mac_override sys_time sys_module sys_rawio",
			"lxc.devttydir":              "lxc",
			"lxc.tty":                    "4",
			"lxc.pts":                    "1024",
			"lxc.rootfs":                 "/var/lib/lxc/{{.AppName}}/rootfs",
			"lxc.mount":                  "/var/lib/lxc/{{.AppName}}/fstab",
			"lxc.utsname":                "{{.AppName}}",
			"lxc.cgroup.devices.deny":    "a",
			"lxc.cgroup.devices.allow.1": "c 1:3 rwm",
			"lxc.cgroup.devices.allow.2": "c 1:5 rwm",
		},
		ExposedPort: 8080,
	}

	// Template para Node.js
	tm.templates["javascript"] = &LXCTemplate{
		Name:       "javascript",
		Language:   "javascript",
		BaseDistro: "ubuntu",
		Release:    "focal",
		Packages: []string{
			"nodejs",
			"npm",
			"git",
			"ca-certificates",
			"build-essential",
			"curl",
			"wget",
		},
		PreInstallScript: `#!/bin/bash
# Pre-install script para Node.js
echo "Configurando entorno para Node.js..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y

# Instalar Node.js más reciente
curl -fsSL https://deb.nodesource.com/setup_18.x | bash -
`,
		PostInstallScript: `#!/bin/bash
# Post-install script para Node.js
echo "Configurando Node.js..."

# Crear directorio de aplicación
mkdir -p /app
chmod 777 /app

# Crear script de inicio
cat > /app/start.sh << 'EOF'
#!/bin/bash
cd /app

# Instalar dependencias
if [ -f package.json ]; then
    echo "Instalando dependencias..."
    npm install
fi

# Compilar si es necesario
if [ -f package.json ] && npm run build >/dev/null 2>&1; then
    echo "Compilando aplicación..."
    npm run build
fi

# Ejecutar aplicación
echo "Iniciando aplicación..."
if [ -f package.json ]; then
    npm start
else
    node app.js
fi
EOF

chmod +x /app/start.sh
echo "Configuración de Node.js completada"
`,
		Environment: map[string]string{
			"NODE_ENV":        "production",
			"PATH":            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"DEBIAN_FRONTEND": "noninteractive",
		},
		Config: map[string]string{
			"lxc.arch":                   "amd64",
			"lxc.network.type":           "veth",
			"lxc.network.link":           "lxcbr0",
			"lxc.network.flags":          "up",
			"lxc.network.hwaddr":         "00:16:3e:xx:xx:xx",
			"lxc.mount.auto":             "proc:mixed sys:ro",
			"lxc.autodev":                "1",
			"lxc.kmsg":                   "0",
			"lxc.cap.drop":               "mac_admin mac_override sys_time sys_module sys_rawio",
			"lxc.devttydir":              "lxc",
			"lxc.tty":                    "4",
			"lxc.pts":                    "1024",
			"lxc.rootfs":                 "/var/lib/lxc/{{.AppName}}/rootfs",
			"lxc.mount":                  "/var/lib/lxc/{{.AppName}}/fstab",
			"lxc.utsname":                "{{.AppName}}",
			"lxc.cgroup.devices.deny":    "a",
			"lxc.cgroup.devices.allow.1": "c 1:3 rwm",
			"lxc.cgroup.devices.allow.2": "c 1:5 rwm",
		},
		ExposedPort: 3000,
	}

	// Template para Python
	tm.templates["python"] = &LXCTemplate{
		Name:       "python",
		Language:   "python",
		BaseDistro: "ubuntu",
		Release:    "focal",
		Packages: []string{
			"python3",
			"python3-pip",
			"python3-venv",
			"git",
			"ca-certificates",
			"build-essential",
			"curl",
			"wget",
		},
		PreInstallScript: `#!/bin/bash
# Pre-install script para Python
echo "Configurando entorno para Python..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
`,
		PostInstallScript: `#!/bin/bash
# Post-install script para Python
echo "Configurando Python..."

# Crear directorio de aplicación
mkdir -p /app
chmod 777 /app

# Crear script de inicio
cat > /app/start.sh << 'EOF'
#!/bin/bash
cd /app

# Crear entorno virtual
if [ ! -d "venv" ]; then
    echo "Creando entorno virtual..."
    python3 -m venv venv
fi

# Activar entorno virtual
source venv/bin/activate

# Instalar dependencias
if [ -f requirements.txt ]; then
    echo "Instalando dependencias..."
    pip install -r requirements.txt
fi

# Ejecutar aplicación
echo "Iniciando aplicación..."
if [ -f app.py ]; then
    python app.py
elif [ -f main.py ]; then
    python main.py
elif [ -f manage.py ]; then
    python manage.py runserver 0.0.0.0:{{.Port}}
else
    echo "No se encontró archivo principal de Python"
    exit 1
fi
EOF

chmod +x /app/start.sh
echo "Configuración de Python completada"
`,
		Environment: map[string]string{
			"PYTHONPATH":       "/app",
			"PYTHONUNBUFFERED": "1",
			"PATH":             "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"DEBIAN_FRONTEND":  "noninteractive",
		},
		Config: map[string]string{
			"lxc.arch":                   "amd64",
			"lxc.network.type":           "veth",
			"lxc.network.link":           "lxcbr0",
			"lxc.network.flags":          "up",
			"lxc.network.hwaddr":         "00:16:3e:xx:xx:xx",
			"lxc.mount.auto":             "proc:mixed sys:ro",
			"lxc.autodev":                "1",
			"lxc.kmsg":                   "0",
			"lxc.cap.drop":               "mac_admin mac_override sys_time sys_module sys_rawio",
			"lxc.devttydir":              "lxc",
			"lxc.tty":                    "4",
			"lxc.pts":                    "1024",
			"lxc.rootfs":                 "/var/lib/lxc/{{.AppName}}/rootfs",
			"lxc.mount":                  "/var/lib/lxc/{{.AppName}}/fstab",
			"lxc.utsname":                "{{.AppName}}",
			"lxc.cgroup.devices.deny":    "a",
			"lxc.cgroup.devices.allow.1": "c 1:3 rwm",
			"lxc.cgroup.devices.allow.2": "c 1:5 rwm",
		},
		ExposedPort: 8000,
	}

	// Template para Rust
	tm.templates["rust"] = &LXCTemplate{
		Name:       "rust",
		Language:   "rust",
		BaseDistro: "ubuntu",
		Release:    "focal",
		Packages: []string{
			"curl",
			"git",
			"ca-certificates",
			"build-essential",
			"pkg-config",
			"libssl-dev",
			"wget",
		},
		PreInstallScript: `#!/bin/bash
# Pre-install script para Rust
echo "Configurando entorno para Rust..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y

# Instalar Rust
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
source ~/.cargo/env
`,
		PostInstallScript: `#!/bin/bash
# Post-install script para Rust
echo "Configurando Rust..."

# Configurar PATH para Rust
export PATH="$HOME/.cargo/bin:$PATH"

# Crear directorio de aplicación
mkdir -p /app
chmod 777 /app

# Crear script de inicio
cat > /app/start.sh << 'EOF'
#!/bin/bash
cd /app
export PATH="$HOME/.cargo/bin:$PATH"

# Compilar aplicación
echo "Compilando aplicación..."
cargo build --release

# Ejecutar aplicación
echo "Iniciando aplicación..."
./target/release/$(basename $(pwd))
EOF

chmod +x /app/start.sh
echo "Configuración de Rust completada"
`,
		Environment: map[string]string{
			"PATH":            "$HOME/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"DEBIAN_FRONTEND": "noninteractive",
		},
		Config: map[string]string{
			"lxc.arch":                   "amd64",
			"lxc.network.type":           "veth",
			"lxc.network.link":           "lxcbr0",
			"lxc.network.flags":          "up",
			"lxc.network.hwaddr":         "00:16:3e:xx:xx:xx",
			"lxc.mount.auto":             "proc:mixed sys:ro",
			"lxc.autodev":                "1",
			"lxc.kmsg":                   "0",
			"lxc.cap.drop":               "mac_admin mac_override sys_time sys_module sys_rawio",
			"lxc.devttydir":              "lxc",
			"lxc.tty":                    "4",
			"lxc.pts":                    "1024",
			"lxc.rootfs":                 "/var/lib/lxc/{{.AppName}}/rootfs",
			"lxc.mount":                  "/var/lib/lxc/{{.AppName}}/fstab",
			"lxc.utsname":                "{{.AppName}}",
			"lxc.cgroup.devices.deny":    "a",
			"lxc.cgroup.devices.allow.1": "c 1:3 rwm",
			"lxc.cgroup.devices.allow.2": "c 1:5 rwm",
		},
		ExposedPort: 8080,
	}

	// Template genérico para Ubuntu
	tm.templates["generic"] = &LXCTemplate{
		Name:       "generic",
		Language:   "generic",
		BaseDistro: "ubuntu",
		Release:    "focal",
		Packages: []string{
			"git",
			"ca-certificates",
			"build-essential",
			"curl",
			"wget",
		},
		PreInstallScript: `#!/bin/bash
# Pre-install script genérico
echo "Configurando entorno genérico..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
`,
		PostInstallScript: `#!/bin/bash
# Post-install script genérico
echo "Configurando entorno genérico..."

# Crear directorio de aplicación
mkdir -p /app
chmod 777 /app

# Crear script de inicio básico
cat > /app/start.sh << 'EOF'
#!/bin/bash
cd /app

# Detectar tipo de aplicación y ejecutar
if [ -f app.py ]; then
    python3 app.py
elif [ -f app.js ]; then
    node app.js
elif [ -f main.go ]; then
    go run main.go
elif [ -f Cargo.toml ]; then
    cargo run
else
    echo "No se pudo detectar el tipo de aplicación"
    ls -la
fi
EOF

chmod +x /app/start.sh
echo "Configuración genérica completada"
`,
		Environment: map[string]string{
			"PATH":            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"DEBIAN_FRONTEND": "noninteractive",
		},
		Config: map[string]string{
			"lxc.arch":                   "amd64",
			"lxc.network.type":           "veth",
			"lxc.network.link":           "lxcbr0",
			"lxc.network.flags":          "up",
			"lxc.network.hwaddr":         "00:16:3e:xx:xx:xx",
			"lxc.mount.auto":             "proc:mixed sys:ro",
			"lxc.autodev":                "1",
			"lxc.kmsg":                   "0",
			"lxc.cap.drop":               "mac_admin mac_override sys_time sys_module sys_rawio",
			"lxc.devttydir":              "lxc",
			"lxc.tty":                    "4",
			"lxc.pts":                    "1024",
			"lxc.rootfs":                 "/var/lib/lxc/{{.AppName}}/rootfs",
			"lxc.mount":                  "/var/lib/lxc/{{.AppName}}/fstab",
			"lxc.utsname":                "{{.AppName}}",
			"lxc.cgroup.devices.deny":    "a",
			"lxc.cgroup.devices.allow.1": "c 1:3 rwm",
			"lxc.cgroup.devices.allow.2": "c 1:5 rwm",
		},
		ExposedPort: 8080,
	}
}

// GetTemplate obtiene un template por lenguaje
func (tm *LXCTemplateManager) GetTemplate(language string) (*LXCTemplate, error) {
	// Normalizar lenguajes
	normalizedLanguage := tm.normalizeLanguage(language)

	if template, exists := tm.templates[normalizedLanguage]; exists {
		return template, nil
	}

	// Fallback a template genérico
	if template, exists := tm.templates["generic"]; exists {
		logrus.Warnf("Usando template genérico para lenguaje: %s", language)
		return template, nil
	}

	return nil, fmt.Errorf("template no encontrado para lenguaje: %s", language)
}

// GetSupportedLanguages devuelve la lista de lenguajes soportados
func (tm *LXCTemplateManager) GetSupportedLanguages() []string {
	var languages []string
	for _, template := range tm.templates {
		if template.Language != "generic" {
			languages = append(languages, template.Language)
		}
	}
	return languages
}

// RenderTemplate renderiza un template con los datos proporcionados
func (tm *LXCTemplateManager) RenderTemplate(language string, data *LXCTemplateData) (string, error) {
	template, err := tm.GetTemplate(language)
	if err != nil {
		return "", err
	}

	// Renderizar configuración
	config := ""
	for key, value := range template.Config {
		renderedValue, err := tm.renderString(value, data)
		if err != nil {
			return "", fmt.Errorf("error renderizando configuración %s: %w", key, err)
		}
		config += fmt.Sprintf("%s = %s\n", key, renderedValue)
	}

	return config, nil
}

// renderString renderiza una cadena con los datos proporcionados
func (tm *LXCTemplateManager) renderString(templateStr string, data *LXCTemplateData) (string, error) {
	tmpl, err := template.New("lxc").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("error parseando template: %w", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, data); err != nil {
		return "", fmt.Errorf("error ejecutando template: %w", err)
	}

	return result.String(), nil
}

// normalizeLanguage normaliza el nombre del lenguaje
func (tm *LXCTemplateManager) normalizeLanguage(language string) string {
	switch strings.ToLower(language) {
	case "js", "nodejs", "node":
		return "javascript"
	case "py":
		return "python"
	case "golang":
		return "go"
	case "rs":
		return "rust"
	default:
		return strings.ToLower(language)
	}
}

// GetTemplateByLanguage obtiene un template específico por lenguaje
func (tm *LXCTemplateManager) GetTemplateByLanguage(language string) (*LXCTemplate, bool) {
	normalizedLanguage := tm.normalizeLanguage(language)
	template, exists := tm.templates[normalizedLanguage]
	return template, exists
}

// ListTemplates lista todos los templates disponibles
func (tm *LXCTemplateManager) ListTemplates() []string {
	var templates []string
	for name := range tm.templates {
		templates = append(templates, name)
	}
	return templates
}

// GetDefaultPort obtiene el puerto por defecto para un lenguaje
func (tm *LXCTemplateManager) GetDefaultPort(language string) int {
	if template, err := tm.GetTemplate(language); err == nil {
		return template.ExposedPort
	}
	return 8080 // Puerto por defecto
}
