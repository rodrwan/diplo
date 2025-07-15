# Diplo - PaaS Local en Go
# Makefile para compilación

.PHONY: build run clean test deps install docker-build docker-run help debug test-docker

# Variables
BINARY_NAME=diplo
BUILD_DIR=bin
MAIN_PATH=./cmd/diplo

# Comandos principales
build: deps generate sqlc
	@echo "Compilando Diplo..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

# Generar templates y compilar
build-templates: generate
	@echo "Generando templates Templ..."
	go generate ./...

run: build
	@echo "Ejecutando Diplo..."
	@clear && ./$(BUILD_DIR)/$(BINARY_NAME)


clean:
	@echo "Limpiando..."
	@rm -rf $(BUILD_DIR)
	@go clean

test:
	@echo "Ejecutando tests..."
	go test ./...

deps:
	@echo "Descargando dependencias..."
	go mod download
	go mod tidy

generate:
	@echo "Generando templates..."
	go generate ./...

sqlc:
	@echo "Generando código SQL..."
	sqlc generate

# Desarrollo
dev: deps
	@echo "Ejecutando en modo desarrollo..."
	go run $(MAIN_PATH)

# Debug mode
debug: deps
	@echo "Ejecutando en modo debug..."
	go run -ldflags="-X main.debug=true" $(MAIN_PATH)

# Testing Docker builds
test-docker:
	@echo "Testing Docker builds..."
	@echo "⚠️  Script test_docker_build.sh eliminado - usar 'make test' para tests generales"

# Instalación
install: build
	@echo "Instalando Diplo..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

# Docker
docker-build:
	@echo "Construyendo imagen Docker..."
	docker build -t diplo .

docker-run:
	@echo "Ejecutando en Docker..."
	docker run -p 8080:8080 -v /var/run/docker.sock:/var/run/docker.sock diplo

deploy:
	@echo "Deploying..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-unknown-linux-gnu-gcc go build -o ./bin/diplo-rpi ./cmd/diplo/main.go
	@echo "Copiando binario a Raspberry Pi..."
	@scp bin/diplo-rpi mango@raspberrypi.local:~/Mangoticket
	@echo "Copiando scripts de gestión..."
	@ssh mango@raspberrypi.local "mkdir -p ~/Mangoticket/diplo-scripts"
	@scp scripts/install_diplo_raspberry.sh mango@raspberrypi.local:~/Mangoticket/diplo-scripts/
	@scp scripts/install_containerd.sh mango@raspberrypi.local:~/Mangoticket/diplo-scripts/
	@scp scripts/setup_raspberry_containerd.sh mango@raspberrypi.local:~/Mangoticket/diplo-scripts/
	@scp scripts/diagnose_containerd.sh mango@raspberrypi.local:~/Mangoticket/diplo-scripts/
	@scp scripts/manage_diplo.sh mango@raspberrypi.local:~/Mangoticket/diplo-scripts/
	@scp scripts/post_deploy_setup.sh mango@raspberrypi.local:~/Mangoticket/diplo-scripts/
	@ssh mango@raspberrypi.local "chmod +x ~/Mangoticket/diplo-scripts/*.sh"
	@echo "Copiando documentación..."
	@ssh mango@raspberrypi.local "mkdir -p ~/Mangoticket/docs"
	@scp docs/RASPBERRY_PI_SETUP.md mango@raspberrypi.local:~/Mangoticket/docs/
	@echo "✅ Deploy completado - binario, scripts y documentación copiados"

# Configuración post-deploy en Raspberry Pi
post-deploy:
	@echo "Ejecutando configuración post-deploy en Raspberry Pi..."
	@ssh mango@raspberrypi.local "cd ~/Mangoticket && ./diplo-scripts/post_deploy_setup.sh"
	@echo "✅ Configuración post-deploy completada"

# Deploy completo con configuración automática
deploy-auto: deploy post-deploy
	@echo "✅ Deploy automático completado - Diplo + Containerd configurado automáticamente"

# Configuración completa para Raspberry Pi
setup-rpi: deploy
	@echo "Configurando Raspberry Pi..."
	@bash scripts/manage_diplo.sh setup-docker

# Deploy completo con configuración Docker
deploy-full: deploy
	@echo "Configurando Docker en Raspberry Pi..."
	@ssh mango@raspberrypi.local "cd ~/Mangoticket/diplo-scripts && ./install_diplo_raspberry.sh"
	@echo "✅ Deploy completo completado - Diplo + Docker configurado"

# Deploy completo con configuración Containerd (Recomendado para Raspberry Pi)
deploy-containerd: deploy
	@echo "Configurando Containerd en Raspberry Pi..."
	@ssh mango@raspberrypi.local "cd ~/Mangoticket/diplo-scripts && ./setup_raspberry_containerd.sh"
	@echo "✅ Deploy completo completado - Diplo + Containerd configurado"

# Solo configurar Docker (sin desplegar Diplo)
setup-docker:
	@echo "Configurando solo Docker..."
	@bash scripts/manage_diplo.sh setup-docker

# Instalar containerd en Raspberry Pi
setup-containerd:
	@echo "Instalando containerd en Raspberry Pi..."
	@ssh mango@raspberrypi.local "cd ~/Mangoticket/diplo-scripts && ./setup_raspberry_containerd.sh"

# Diagnóstico de containerd en Raspberry Pi
diagnose-containerd:
	@echo "Diagnosticando containerd en Raspberry Pi..."
	@ssh mango@raspberrypi.local "cd ~/Mangoticket/diplo-scripts && ./diagnose_containerd.sh"

# Gestión de Diplo
manage:
	@echo "Gestión de Diplo..."
	@bash scripts/manage_diplo.sh

# Help
help:
	@echo "Comandos disponibles:"
	@echo "  build       - Compilar el proyecto"
	@echo "  run         - Compilar y ejecutar"
	@echo "  dev         - Ejecutar en modo desarrollo"
	@echo "  debug       - Ejecutar en modo debug"
	@echo "  clean       - Limpiar archivos generados"
	@echo "  test        - Ejecutar tests"
	@echo "  test-docker - Testing de builds Docker (deshabilitado)"
	@echo "  deps        - Descargar dependencias"
	@echo "  install     - Instalar en /usr/local/bin"
	@echo "  docker-build- Construir imagen Docker"
	@echo "  docker-run  - Ejecutar en Docker"
	@echo ""
	@echo "Deploy en Raspberry Pi:"
	@echo "  deploy      - Compilar y desplegar en Raspberry Pi"
	@echo "  deploy-auto - Deploy automático (Diplo + Containerd) [RECOMENDADO]"
	@echo "  deploy-full - Deploy completo (Diplo + Docker)"
	@echo "  deploy-containerd - Deploy completo (Diplo + Containerd)"
	@echo "  post-deploy - Configuración post-deploy en Raspberry Pi"
	@echo "  setup-docker   - Solo configurar Docker en Raspberry Pi"
	@echo "  setup-containerd - Instalar containerd en Raspberry Pi"
	@echo "  setup-rpi   - Configuración completa (Docker + deploy)"
	@echo "  diagnose-containerd - Diagnosticar containerd en Raspberry Pi"
	@echo "  manage      - Gestión de Diplo (start/stop/status/etc)"