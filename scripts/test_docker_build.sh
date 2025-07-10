#!/bin/bash

# Script para testing manual de builds de Docker
# Uso: ./test_docker_build.sh <repo_url> <language>

set -e

REPO_URL=${1:-"https://github.com/rodrwan/simple-go-app.git"}
LANGUAGE=${2:-"go"}
APP_ID="test_$(date +%s)"

echo "=== TESTING DOCKER BUILD ==="
echo "Repo URL: $REPO_URL"
echo "Language: $LANGUAGE"
echo "App ID: $APP_ID"
echo ""

# Obtener hash del último commit
echo "=== OBTENIENDO HASH DEL COMMIT ==="
TEMP_DIR=$(mktemp -d)
git clone --depth 1 "$REPO_URL" "$TEMP_DIR" 2>/dev/null || {
    echo "❌ Error clonando repositorio"
    exit 1
}

COMMIT_HASH=$(cd "$TEMP_DIR" && git rev-parse HEAD | cut -c1-8)
rm -rf "$TEMP_DIR"

echo "Hash del último commit: $COMMIT_HASH"

# Generar tag único
IMAGE_TAG="diplo_${APP_ID}_${COMMIT_HASH}"
echo "Tag de imagen: $IMAGE_TAG"
echo ""

# Generar Dockerfile según lenguaje
case $LANGUAGE in
    "go")
        DOCKERFILE="# Diplo - Dockerfile generado automáticamente
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone $REPO_URL .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD [\"./main\"]"
        ;;
    "node")
        DOCKERFILE="# Diplo - Dockerfile generado automáticamente
FROM node:18-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone $REPO_URL .
RUN npm ci --only=production

FROM node:18-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE 3000
CMD [\"npm\", \"start\"]"
        ;;
    "python")
        DOCKERFILE="# Diplo - Dockerfile generado automáticamente
FROM python:3.11-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone $REPO_URL .
RUN pip install -r requirements.txt

FROM python:3.11-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE 8000
CMD [\"python\", \"app.py\"]"
        ;;
    *)
        echo "Lenguaje no soportado: $LANGUAGE"
        exit 1
        ;;
esac

echo "=== DOCKERFILE GENERADO ==="
echo "$DOCKERFILE"
echo ""

# Crear contexto de build
echo "=== CREANDO CONTEXTO DE BUILD ==="
BUILD_DIR=$(mktemp -d)
echo "Build dir: $BUILD_DIR"

# Crear Dockerfile en directorio temporal
echo "$DOCKERFILE" > "$BUILD_DIR/Dockerfile"

# Construir imagen
echo "=== CONSTRUYENDO IMAGEN ==="
echo "Image tag: $IMAGE_TAG"

if docker build --no-cache -t "$IMAGE_TAG" "$BUILD_DIR"; then
    echo "✅ Build exitoso"
    
    # Verificar que la imagen existe
    if docker images | grep -q "$IMAGE_TAG"; then
        echo "✅ Imagen encontrada en Docker"
        echo "Image ID: $(docker images -q $IMAGE_TAG)"
        
        # Mostrar información de la imagen
        echo "=== INFORMACIÓN DE LA IMAGEN ==="
        docker images "$IMAGE_TAG" --format "table {{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}\t{{.CreatedAt}}"
    else
        echo "❌ Imagen no encontrada en Docker"
    fi
else
    echo "❌ Build falló"
fi

# Limpiar
echo "=== LIMPIANDO ==="
rm -rf "$BUILD_DIR"
docker rmi "$IMAGE_TAG" 2>/dev/null || true

echo "=== TEST COMPLETADO ===" 