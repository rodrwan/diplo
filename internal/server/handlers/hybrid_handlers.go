package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	runtimePkg "github.com/rodrwan/diplo/internal/runtime"
	"github.com/sirupsen/logrus"
)

// UnifiedStatusHandler maneja el endpoint GET /api/unified/status
func UnifiedStatusHandler(ctx *HybridContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	factory, ok := ctx.runtimeFactory.(runtimePkg.RuntimeFactory)
	if !ok {
		logrus.Error("Runtime factory no es del tipo correcto")
		return Response{Code: http.StatusInternalServerError, Message: "Error interno del servidor"}, nil
	}

	// Obtener información básica del sistema
	status := map[string]interface{}{
		"timestamp": time.Now(),
		"system": map[string]interface{}{
			"os":           runtime.GOOS,
			"architecture": runtime.GOARCH,
			"runtime":      "hybrid",
		},
		"runtime": map[string]interface{}{
			"available":           factory.GetAvailableRuntimes(),
			"preferred":           factory.GetPreferredRuntime(),
			"supported_languages": []string{"go", "javascript", "python", "rust", "java"},
			"supported_images":    getSupportedImages(factory.GetPreferredRuntime()),
		},
		"applications": []interface{}{},
	}

	return Response{Code: http.StatusOK, Data: status}, nil
}

// UnifiedDeployHandler maneja el endpoint POST /api/unified/deploy
func UnifiedDeployHandler(ctx *HybridContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return Response{Code: http.StatusBadRequest, Message: "JSON inválido"}, nil
	}

	// Validar campos requeridos
	repoURL, ok := req["repo_url"].(string)
	if !ok || repoURL == "" {
		return Response{Code: http.StatusBadRequest, Message: "repo_url es requerido"}, nil
	}

	factory, ok := ctx.runtimeFactory.(runtimePkg.RuntimeFactory)
	if !ok {
		logrus.Error("Runtime factory no es del tipo correcto")
		return Response{Code: http.StatusInternalServerError, Message: "Error interno del servidor"}, nil
	}

	// Determinar runtime a usar
	selectedRuntime := factory.GetPreferredRuntime()
	if runtimeType, exists := req["runtime_type"].(string); exists && runtimeType != "" {
		// Validar que el runtime solicitado esté disponible
		availableRuntimes := factory.GetAvailableRuntimes()
		found := false
		for _, available := range availableRuntimes {
			if string(available) == runtimeType {
				selectedRuntime = available
				found = true
				break
			}
		}
		if !found {
			return Response{Code: http.StatusBadRequest, Message: "Runtime solicitado no está disponible"}, nil
		}
	}

	// Por ahora, devolver respuesta de éxito simulada
	response := map[string]interface{}{
		"id":           generateSimpleID(),
		"name":         req["name"],
		"repo_url":     repoURL,
		"language":     req["language"],
		"runtime_type": selectedRuntime,
		"status":       "deploying",
		"message":      "Deployment iniciado con runtime " + string(selectedRuntime),
		"created_at":   time.Now(),
	}

	return Response{Code: http.StatusCreated, Data: response}, nil
}

// HybridLXCStatusHandler maneja el endpoint GET /api/lxc/status (versión híbrida)
func HybridLXCStatusHandler(ctx *HybridContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	factory, ok := ctx.runtimeFactory.(runtimePkg.RuntimeFactory)
	if !ok {
		logrus.Error("Runtime factory no es del tipo correcto")
		return Response{Code: http.StatusInternalServerError, Message: "Error interno del servidor"}, nil
	}

	// Verificar si LXC está disponible
	availableRuntimes := factory.GetAvailableRuntimes()
	lxcAvailable := false
	for _, rt := range availableRuntimes {
		if rt == runtimePkg.RuntimeTypeLXC {
			lxcAvailable = true
			break
		}
	}

	if !lxcAvailable {
		return Response{Code: http.StatusServiceUnavailable, Message: "LXC no está disponible en este sistema"}, nil
	}

	// Obtener información básica de LXC
	status := map[string]interface{}{
		"runtime_type": "lxc",
		"available":    true,
		"version":      "simulated",
		"timestamp":    time.Now(),
	}

	return Response{Code: http.StatusOK, Data: status}, nil
}

// HybridDockerStatusHandler maneja el endpoint GET /api/docker/status (versión híbrida)
func HybridDockerStatusHandler(ctx *HybridContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	factory, ok := ctx.runtimeFactory.(runtimePkg.RuntimeFactory)
	if !ok {
		logrus.Error("Runtime factory no es del tipo correcto")
		return Response{Code: http.StatusInternalServerError, Message: "Error interno del servidor"}, nil
	}

	// Verificar si Docker está disponible
	availableRuntimes := factory.GetAvailableRuntimes()
	dockerAvailable := false
	for _, rt := range availableRuntimes {
		if rt == runtimePkg.RuntimeTypeDocker {
			dockerAvailable = true
			break
		}
	}

	if !dockerAvailable {
		return Response{Code: http.StatusServiceUnavailable, Message: "Docker no está disponible en este sistema"}, nil
	}

	// Obtener información básica de Docker
	status := map[string]interface{}{
		"runtime_type": "docker",
		"available":    true,
		"version":      "integrated",
		"capabilities": []string{
			"build", "run", "logs", "networking", "volumes", "exec", "events",
		},
		"supported_images": getSupportedImages(runtimePkg.RuntimeTypeDocker),
		"timestamp":        time.Now(),
	}

	return Response{Code: http.StatusOK, Data: status}, nil
}

// Helper functions
func generateSimpleID() string {
	return "app-" + time.Now().Format("20060102150405")
}

func getSupportedImages(runtimeType runtimePkg.RuntimeType) []string {
	switch runtimeType {
	case runtimePkg.RuntimeTypeDocker:
		return []string{"golang:1.24-alpine", "node:22-alpine", "python:3.13-alpine", "rust:1.83-alpine", "ubuntu:22.04", "nginx:alpine"}
	case runtimePkg.RuntimeTypeContainerd:
		return []string{"golang:1.24-alpine", "node:22-alpine", "python:3.13-alpine", "rust:1.83-alpine", "ubuntu:22.04"}
	case runtimePkg.RuntimeTypeLXC:
		return []string{"ubuntu:22.04", "alpine:3.18", "debian:bullseye"}
	default:
		return []string{}
	}
}
