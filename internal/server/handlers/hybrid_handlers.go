package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/models"
	runtimePkg "github.com/rodrwan/diplo/internal/runtime"
	"github.com/sirupsen/logrus"
)

// UnifiedStatusHandler maneja el endpoint GET /api/status
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
	var req models.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return Response{Code: http.StatusBadRequest, Message: "JSON inválido"}, nil
	}

	// Validar campos requeridos
	if req.RepoURL == "" {
		return Response{Code: http.StatusBadRequest, Message: "repo_url es requerido"}, nil
	}

	factory, ok := ctx.runtimeFactory.(runtimePkg.RuntimeFactory)
	if !ok {
		logrus.Error("Runtime factory no es del tipo correcto")
		return Response{Code: http.StatusInternalServerError, Message: "Error interno del servidor"}, nil
	}

	// Verificar si ya existe una aplicación con este repo_url
	existingApp, err := ctx.queries.GetAppByRepoUrl(r.Context(), req.RepoURL)
	if err != nil && err != sql.ErrNoRows {
		logrus.Errorf("Error verificando aplicación existente: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error verificando aplicación existente"}, err
	}

	// Si existe una app con el mismo repo_url, hacer redeploy
	if err != sql.ErrNoRows {
		logrus.Infof("App existente encontrada para %s, haciendo redeploy: %s", req.RepoURL, existingApp.ID)

		// Actualizar nombre si se proporcionó uno nuevo
		if req.Name != "" && req.Name != existingApp.Name {
			existingApp.Name = req.Name
		}

		// Actualizar variables de entorno si se proporcionaron
		if len(req.EnvVars) > 0 {
			// Eliminar variables existentes una por una (no existe DeleteAppEnvVars)
			existingEnvVars, err := ctx.queries.GetAppEnvVars(r.Context(), existingApp.ID)
			if err == nil {
				for _, existing := range existingEnvVars {
					if err := ctx.queries.DeleteAppEnvVar(r.Context(), database.DeleteAppEnvVarParams{
						AppID: existingApp.ID,
						Key:   existing.Key,
					}); err != nil {
						logrus.Errorf("Error eliminando variable de entorno existente %s: %v", existing.Key, err)
					}
				}
			}

			// Guardar nuevas variables de entorno
			for _, envVar := range req.EnvVars {
				// Validar nombre de variable
				if !isValidEnvVarName(envVar.Name) {
					logrus.Errorf("Nombre de variable de entorno inválido: %s", envVar.Name)
					continue
				}

				// Validar valor de variable
				if !isValidEnvVarValue(envVar.Value) {
					logrus.Errorf("Valor de variable de entorno inválido para %s", envVar.Name)
					continue
				}

				value := envVar.Value
				isSecret := false

				// Determinar si es una variable secreta basándose en palabras clave
				secretKeywords := []string{"password", "secret", "key", "token", "api_key", "private"}
				lowerName := strings.ToLower(envVar.Name)
				lowerValue := strings.ToLower(envVar.Value)

				for _, keyword := range secretKeywords {
					if strings.Contains(lowerName, keyword) || strings.Contains(lowerValue, keyword) {
						isSecret = true
						break
					}
				}

				// Cifrar si es secreto
				if shouldEncryptValue(isSecret) {
					if encryptedValue, err := encryptValue(envVar.Value); err != nil {
						logrus.Errorf("Error cifrando variable secreta %s: %v", envVar.Name, err)
						continue
					} else {
						value = encryptedValue
					}
				}

				if err := ctx.queries.CreateAppEnvVar(r.Context(), database.CreateAppEnvVarParams{
					AppID:    existingApp.ID,
					Key:      envVar.Name,
					Value:    value,
					IsSecret: sql.NullBool{Bool: isSecret, Valid: true},
				}); err != nil {
					logrus.Errorf("Error guardando variable de entorno %s: %v", envVar.Name, err)
				}
			}
		}

		// Iniciar redeploy en background usando runtime factory
		go unifiedRedeployApp(ctx, &existingApp, factory)

		// Responder inmediatamente
		response := map[string]interface{}{
			"id":           existingApp.ID,
			"name":         existingApp.Name,
			"repo_url":     existingApp.RepoUrl,
			"port":         existingApp.Port,
			"url":          fmt.Sprintf("http://localhost:%d", existingApp.Port),
			"status":       "redeploying",
			"runtime_type": factory.GetPreferredRuntime(),
			"message":      "Redeploy iniciado para aplicación existente",
		}

		return Response{Code: http.StatusOK, Data: response}, nil
	}

	// Si no existe, crear nueva aplicación
	app := &database.App{
		ID:      database.GenerateAppID(),
		Name:    req.Name,
		RepoUrl: req.RepoURL,
	}

	// Asignar puerto libre
	port, err := findFreePort()
	if err != nil {
		logrus.Errorf("Error asignando puerto: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "No se pudo asignar puerto libre"}, err
	}
	app.Port = int64(port)

	// Determinar runtime a usar
	selectedRuntime := factory.GetPreferredRuntime()
	if req.RuntimeType != "" {
		// Validar que el runtime solicitado esté disponible
		availableRuntimes := factory.GetAvailableRuntimes()
		found := false
		for _, available := range availableRuntimes {
			if string(available) == req.RuntimeType {
				selectedRuntime = available
				found = true
				break
			}
		}
		if !found {
			return Response{Code: http.StatusBadRequest, Message: "Runtime solicitado no está disponible"}, nil
		}
	}

	// Guardar en base de datos
	if err := ctx.queries.CreateApp(r.Context(), database.CreateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  req.RepoURL,
		Language: sql.NullString{String: "unknown", Valid: true}, // Se detectará durante el deployment
		Port:     int64(port),
		Status:   database.StatusDeploying,
	}); err != nil {
		logrus.Errorf("Error guardando aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error guardando aplicación"}, err
	}

	// Guardar variables de entorno si se proporcionaron
	if len(req.EnvVars) > 0 {
		for _, envVar := range req.EnvVars {
			// Validar nombre de variable
			if !isValidEnvVarName(envVar.Name) {
				logrus.Errorf("Nombre de variable de entorno inválido: %s", envVar.Name)
				continue
			}

			// Validar valor de variable
			if !isValidEnvVarValue(envVar.Value) {
				logrus.Errorf("Valor de variable de entorno inválido para %s", envVar.Name)
				continue
			}

			value := envVar.Value
			isSecret := false

			// Determinar si es una variable secreta basándose en palabras clave
			secretKeywords := []string{"password", "secret", "key", "token", "api_key", "private"}
			lowerName := strings.ToLower(envVar.Name)
			lowerValue := strings.ToLower(envVar.Value)

			for _, keyword := range secretKeywords {
				if strings.Contains(lowerName, keyword) || strings.Contains(lowerValue, keyword) {
					isSecret = true
					break
				}
			}

			// Cifrar si es secreto
			if shouldEncryptValue(isSecret) {
				if encryptedValue, err := encryptValue(envVar.Value); err != nil {
					logrus.Errorf("Error cifrando variable secreta %s: %v", envVar.Name, err)
					continue
				} else {
					value = encryptedValue
				}
			}

			if err := ctx.queries.CreateAppEnvVar(r.Context(), database.CreateAppEnvVarParams{
				AppID:    app.ID,
				Key:      envVar.Name,
				Value:    value,
				IsSecret: sql.NullBool{Bool: isSecret, Valid: true},
			}); err != nil {
				logrus.Errorf("Error guardando variable de entorno %s: %v", envVar.Name, err)
			}
		}
	}

	// Iniciar deployment en background usando runtime factory
	go unifiedDeployApp(ctx, app, factory)

	// Responder inmediatamente
	response := map[string]interface{}{
		"id":           app.ID,
		"name":         app.Name,
		"repo_url":     app.RepoUrl,
		"port":         app.Port,
		"url":          fmt.Sprintf("http://localhost:%d", app.Port),
		"status":       "deploying",
		"runtime_type": selectedRuntime,
		"message":      "Aplicación creada y deployment iniciado con runtime " + string(selectedRuntime),
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

// unifiedDeployApp ejecuta el deployment usando el runtime factory
func unifiedDeployApp(ctx *HybridContext, app *database.App, factory runtimePkg.RuntimeFactory) {
	// Obtener runtime preferido del factory
	selectedRuntime := factory.GetPreferredRuntime()
	logrus.Infof("Iniciando deployment unificado de: %s (%s) con runtime %s", app.Name, app.ID, selectedRuntime)

	// Cargar variables de entorno de la base de datos
	existingEnvVars, err := ctx.queries.GetAppEnvVars(context.Background(), app.ID)
	if err != nil {
		logrus.Warnf("Error cargando variables de entorno para deployment: %v", err)
	}

	// Convertir a formato models.EnvVar
	envVars := make([]models.EnvVar, 0, len(existingEnvVars))
	for _, env := range existingEnvVars {
		value := env.Value

		// Descifrar valores secretos para el contenedor
		if env.IsSecret.Bool {
			if decryptedValue, err := decryptValue(env.Value); err != nil {
				logrus.Errorf("Error descifrando valor secreto para contenedor %s: %v", env.Key, err)
				// Usar valor por defecto o saltar esta variable
				continue
			} else {
				value = decryptedValue
			}
		}

		envVars = append(envVars, models.EnvVar{
			Name:  env.Key,
			Value: value,
		})
	}

	// Convertir HybridContext a Context regular para usar deployApp existente
	regularCtx := ctx.Context

	// Por ahora todos los runtimes usan el sistema Docker como backend
	// En el futuro aquí se podría agregar lógica específica por runtime
	logrus.Infof("Usando sistema Docker para runtime %s", selectedRuntime)
	deployApp(regularCtx, app, envVars)
}

// unifiedRedeployApp ejecuta el redeploy usando el runtime factory
func unifiedRedeployApp(ctx *HybridContext, app *database.App, factory runtimePkg.RuntimeFactory) {
	logrus.Infof("Iniciando redeploy unificado de: %s (%s)", app.Name, app.ID)

	// Obtener runtime preferido para el redeploy
	preferredRuntime := factory.GetPreferredRuntime()

	// Si el runtime preferido es Docker, usar el sistema Docker existente
	if preferredRuntime == runtimePkg.RuntimeTypeDocker {
		// Convertir HybridContext a Context regular para usar redeployExistingApp existente
		regularCtx := ctx.Context
		redeployExistingApp(regularCtx, app)
		return
	}

	// Para otros runtimes, implementar lógica específica
	logrus.Infof("Runtime %s no tiene implementación específica, usando Docker como fallback", preferredRuntime)
	regularCtx := ctx.Context
	redeployExistingApp(regularCtx, app)
}
