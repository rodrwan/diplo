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
	@./scripts/test_docker_build.sh

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

# Help
help:
	@echo "Comandos disponibles:"
	@echo "  build       - Compilar el proyecto"
	@echo "  run         - Compilar y ejecutar"
	@echo "  dev         - Ejecutar en modo desarrollo"
	@echo "  debug       - Ejecutar en modo debug"
	@echo "  clean       - Limpiar archivos generados"
	@echo "  test        - Ejecutar tests"
	@echo "  test-docker - Testing de builds Docker"
	@echo "  deps        - Descargar dependencias"
	@echo "  install     - Instalar en /usr/local/bin"
	@echo "  docker-build- Construir imagen Docker"
	@echo "  docker-run  - Ejecutar en Docker"