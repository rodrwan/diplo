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

	"io"

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
	// LXC removido - endpoint deshabilitado
	status := map[string]interface{}{
		"runtime_type": "lxc",
		"available":    false,
		"message":      "LXC ha sido removido del sistema",
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
		return []string{"golang:1.24-alpine", "node:22-alpine", "python:3.13-alpine", "rust:1.83-alpine", "ubuntu:24.04", "nginx:alpine"}
	case runtimePkg.RuntimeTypeContainerd:
		return []string{"golang:1.24-alpine", "node:22-alpine", "python:3.13-alpine", "rust:1.83-alpine", "ubuntu:24.04"}
	// LXC removido - usar solo Docker y containerd
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

	// Enviar log inicial
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Iniciando deployment con runtime %s", selectedRuntime))

	// Actualizar estado
	app.Status = database.StatusDeploying
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: sql.NullString{String: "unknown", Valid: true},
		Port:     app.Port,
		Status:   app.Status,
	})

	// Detectar lenguaje
	sendHybridLogMessage(ctx, app.ID, "info", "Detectando lenguaje...")
	language, err := detectLanguage(app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error detectando lenguaje: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error detectando lenguaje: %v", err))
		return
	}
	app.Language = sql.NullString{String: language, Valid: true}
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Lenguaje detectado: %s", language))

	// Crear runtime específico según el tipo seleccionado
	runtime, err := factory.CreateRuntime(selectedRuntime)
	if err != nil {
		logrus.Errorf("Error creando runtime %s: %v", selectedRuntime, err)

		// Intentar fallback a Docker si containerd falla
		if selectedRuntime == runtimePkg.RuntimeTypeContainerd {
			logrus.Warnf("Containerd no disponible, intentando fallback a Docker")
			sendHybridLogMessage(ctx, app.ID, "warning", "Containerd no disponible, usando Docker como fallback")

			// Verificar si Docker está disponible
			availableRuntimes := factory.GetAvailableRuntimes()
			dockerAvailable := false
			for _, rt := range availableRuntimes {
				if rt == runtimePkg.RuntimeTypeDocker {
					dockerAvailable = true
					break
				}
			}

			if dockerAvailable {
				selectedRuntime = runtimePkg.RuntimeTypeDocker
				logrus.Infof("Cambiando a runtime Docker para deployment")
				sendHybridLogMessage(ctx, app.ID, "info", "Cambiando a runtime Docker")
			} else {
				handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error creando runtime %s y no hay fallback disponible: %v", selectedRuntime, err))
				return
			}
		} else {
			handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error creando runtime %s: %v", selectedRuntime, err))
			return
		}
	}

	// Si el runtime se creó exitosamente, cerrarlo al final
	if runtime != nil {
		defer runtime.Close()
	}

	// Configurar callback de eventos para el runtime
	if runtime != nil {
		runtime.SetEventCallback(func(event runtimePkg.Event) {
			sendUnifiedRuntimeEvent(ctx, app.ID, event)
		})
	}

	// Ejecutar deployment según el runtime
	switch selectedRuntime {
	case runtimePkg.RuntimeTypeDocker:
		// Usar el sistema Docker existente
		regularCtx := ctx.Context
		deployApp(regularCtx, app, envVars)
	case runtimePkg.RuntimeTypeContainerd:
		if runtime != nil {
			deployWithContainerd(ctx, app, runtime, envVars, language)
		} else {
			// Fallback a Docker si no se pudo crear el runtime containerd
			logrus.Warnf("No se pudo crear runtime containerd, usando Docker como fallback")
			regularCtx := ctx.Context
			deployApp(regularCtx, app, envVars)
		}
	default:
		// Fallback a Docker
		logrus.Warnf("Runtime %s no implementado, usando Docker como fallback", selectedRuntime)
		regularCtx := ctx.Context
		deployApp(regularCtx, app, envVars)
	}
}

// deployWithContainerd ejecuta el deployment usando containerd
func deployWithContainerd(ctx *HybridContext, app *database.App, runtime runtimePkg.ContainerRuntime, envVars []models.EnvVar, language string) {
	sendHybridLogMessage(ctx, app.ID, "info", "🚀 Iniciando deployment con containerd...")
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📦 Aplicación: %s (%s)", app.Name, app.ID))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("🔗 Repositorio: %s", app.RepoUrl))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("💻 Lenguaje detectado: %s", language))

	// Verificar que el runtime containerd esté realmente disponible
	if runtime == nil {
		logrus.Warnf("Runtime containerd es nil, usando Docker como fallback")
		sendHybridLogMessage(ctx, app.ID, "warning", "Runtime containerd no disponible, usando Docker como fallback")
		regularCtx := ctx.Context
		deployApp(regularCtx, app, envVars)
		return
	}

	// Implementación real de deployment con containerd
	sendHybridLogMessage(ctx, app.ID, "info", "Implementando deployment con containerd...")

	// Determinar imagen base según el lenguaje
	baseImage := getContainerdBaseImage(language)
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Usando imagen base: %s", baseImage))

	// Preparar variables de entorno con el puerto correcto
	envVarsMap := convertEnvVarsToMap(envVars)

	// Agregar variables de entorno del sistema
	envVarsMap["PORT"] = fmt.Sprintf("%d", app.Port)
	envVarsMap["DIPLO_APP_ID"] = app.ID
	envVarsMap["DIPLO_APP_NAME"] = app.Name
	envVarsMap["DIPLO_APP_PORT"] = fmt.Sprintf("%d", app.Port)

	// Crear request para el contenedor
	containerReq := &runtimePkg.CreateContainerRequest{
		Name:        app.ID, // Usar app ID como nombre del contenedor
		Image:       baseImage,
		Command:     []string{"sleep", "infinity"}, // Comando temporal para mantener contenedor vivo
		WorkingDir:  "/app",
		Environment: envVarsMap,
		Ports: []runtimePkg.PortMapping{
			{
				HostPort:      int(app.Port),
				ContainerPort: int(app.Port), // Usar el mismo puerto que el host
				Protocol:      "tcp",
			},
		},
		NetworkMode: "host", // Usar host networking para containerd
		Labels: map[string]string{
			"app.id":   app.ID,
			"app.name": app.Name,
			"runtime":  "containerd",
		},
		Resources: &runtimePkg.ResourceConfig{
			Memory:    512 * 1024 * 1024, // 512MB
			CPUShares: 512,
		},
	}

	// Crear contenedor
	sendHybridLogMessage(ctx, app.ID, "info", "Creando contenedor containerd...")
	container, err := runtime.CreateContainer(containerReq)
	if err != nil {
		logrus.Errorf("Error creando contenedor containerd: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error creando contenedor containerd: %v", err))
		return
	}

	// Iniciar contenedor
	sendHybridLogMessage(ctx, app.ID, "info", "Iniciando contenedor containerd...")
	if err := runtime.StartContainer(context.Background(), container.ID); err != nil {
		logrus.Errorf("Error iniciando contenedor containerd: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error iniciando contenedor containerd: %v", err))
		return
	}

	// Esperar a que el contenedor esté corriendo
	sendHybridLogMessage(ctx, app.ID, "info", "Esperando a que el contenedor esté listo...")
	containerIP := ""
	maxRetries := 10
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Verificar estado del contenedor
		containerInfo, err := runtime.GetContainer(container.ID)
		if err != nil {
			logrus.Warnf("No se pudo obtener información del contenedor containerd (intento %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryDelay)
			continue
		}

		if containerInfo.Status != runtimePkg.ContainerStatusRunning {
			logrus.Infof("Contenedor no está corriendo aún (estado: %s), esperando... (intento %d/%d)", containerInfo.Status, i+1, maxRetries)
			time.Sleep(retryDelay)
			continue
		}

		// Intentar obtener la IP
		containerIP, err = runtime.GetContainerIP(container.ID)
		if err != nil {
			logrus.Warnf("No se pudo obtener IP del contenedor containerd (intento %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryDelay)
			continue
		}

		if containerIP == "" {
			logrus.Warnf("IP del contenedor está vacía (intento %d/%d), esperando...", i+1, maxRetries)
			time.Sleep(retryDelay)
			continue
		}

		// Si llegamos aquí, tenemos una IP válida
		logrus.Infof("Contenedor containerd listo con IP: %s", containerIP)
		break
	}

	// Si después de todos los intentos no tenemos IP, fallar el deployment
	if containerIP == "" {
		// Leer logs del contenedor para mostrar el error real
		logs, logErr := runtime.GetContainerLogs(context.Background(), container.ID)
		logOutput := ""
		if logErr == nil && logs != nil {
			defer logs.Close()
			buf := new(strings.Builder)
			io.Copy(buf, logs)
			logOutput = buf.String()
		}
		errorMsg := fmt.Sprintf("No se pudo obtener IP del contenedor containerd después de múltiples intentos. Logs:\n%s", logOutput)
		logrus.Error(errorMsg)
		handleUnifiedDeployError(ctx, app, errorMsg)
		return
	}

	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Contenedor containerd iniciado exitosamente con IP: %s", containerIP))

	// Verificar que el contenedor esté realmente listo antes de ejecutar comandos
	sendHybridLogMessage(ctx, app.ID, "info", "Verificando que el contenedor esté listo para comandos...")
	readyCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"echo", "container-ready"})
	if err != nil {
		logrus.Errorf("Error verificando que el contenedor esté listo: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error verificando que el contenedor esté listo: %v", err))
		return
	}
	if readyCheck.ExitCode != 0 {
		logrus.Errorf("Contenedor no está listo para comandos: %s", readyCheck.Error)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Contenedor no está listo para comandos: %s", readyCheck.Error))
		return
	}
	sendHybridLogMessage(ctx, app.ID, "info", "Contenedor listo para comandos")

	// Instalar git si es necesario (para imágenes Alpine)
	sendHybridLogMessage(ctx, app.ID, "info", "Verificando dependencias...")
	gitCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "git"})
	if err != nil {
		logrus.Errorf("Error verificando git: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error verificando git: %v", err))
		return
	}

	if gitCheck.ExitCode != 0 {
		sendHybridLogMessage(ctx, app.ID, "info", "Git no encontrado, instalando...")

		// Detectar el gestor de paquetes disponible
		packageManager := "apk" // Por defecto Alpine

		// Verificar si es Alpine Linux
		sendHybridLogMessage(ctx, app.ID, "info", "Detectando gestor de paquetes...")
		alpineCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "apk"})
		if err != nil {
			logrus.Errorf("Error verificando apk: %v", err)
		} else {
			sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Verificación apk: exit_code=%d, output=%s", alpineCheck.ExitCode, alpineCheck.Output))
		}

		if err == nil && alpineCheck.ExitCode == 0 {
			packageManager = "apk"
			installResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"apk", "add", "--no-cache", "git"})
			if err != nil {
				logrus.Errorf("Error instalando git con apk: %v", err)
				handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error instalando git con apk: %v", err))
				return
			}
			if installResult.ExitCode != 0 {
				logrus.Errorf("Error instalando git con apk: %s", installResult.Error)
				handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error instalando git con apk: %s", installResult.Error))
				return
			}
		} else {
			// Intentar con apt (Ubuntu/Debian)
			aptCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "apt"})
			if err != nil {
				logrus.Errorf("Error verificando apt: %v", err)
			} else {
				sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Verificación apt: exit_code=%d, output=%s", aptCheck.ExitCode, aptCheck.Output))
			}

			if err == nil && aptCheck.ExitCode == 0 {
				packageManager = "apt"
				// Actualizar repositorios primero
				updateResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"apt-get", "update"})
				if err != nil {
					logrus.Errorf("Error actualizando repositorios: %v", err)
				}
				if updateResult.ExitCode != 0 {
					logrus.Warnf("Error actualizando repositorios: %s", updateResult.Error)
				}

				installResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"apt-get", "install", "-y", "git"})
				if err != nil {
					logrus.Errorf("Error instalando git con apt: %v", err)
					handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error instalando git con apt: %v", err))
					return
				}
				if installResult.ExitCode != 0 {
					logrus.Errorf("Error instalando git con apt: %s", installResult.Error)
					handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error instalando git con apt: %s", installResult.Error))
					return
				}
			} else {
				// Intentar con yum (RHEL/CentOS)
				yumCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "yum"})
				if err != nil {
					logrus.Errorf("Error verificando yum: %v", err)
				} else {
					sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Verificación yum: exit_code=%d, output=%s", yumCheck.ExitCode, yumCheck.Output))
				}

				if err == nil && yumCheck.ExitCode == 0 {
					packageManager = "yum"
					installResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"yum", "install", "-y", "git"})
					if err != nil {
						logrus.Errorf("Error instalando git con yum: %v", err)
						handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error instalando git con yum: %v", err))
						return
					}
					if installResult.ExitCode != 0 {
						logrus.Errorf("Error instalando git con yum: %s", installResult.Error)
						handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error instalando git con yum: %s", installResult.Error))
						return
					}
				} else {
					// No se pudo detectar el gestor de paquetes
					errorMsg := "No se pudo detectar el gestor de paquetes (apk/apt/yum) para instalar git"
					logrus.Error(errorMsg)
					handleUnifiedDeployError(ctx, app, errorMsg)
					return
				}
			}
		}

		sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Git instalado correctamente usando %s", packageManager))
	} else {
		sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Git ya está disponible: %s", gitCheck.Output))
	}

	// Clonar repositorio PRIMERO
	sendHybridLogMessage(ctx, app.ID, "info", "Clonando repositorio...")
	cloneResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"git", "clone", app.RepoUrl, "/app/src"})
	if err != nil {
		logrus.Errorf("Error clonando repositorio: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error clonando repositorio: %v", err))
		return
	}
	if cloneResult.ExitCode != 0 {
		logrus.Errorf("Error clonando repositorio: %s", cloneResult.Error)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error clonando repositorio: %s", cloneResult.Error))
		return
	}

	// Compilación Go con debug
	sendHybridLogMessage(ctx, app.ID, "info", "Compilando aplicación Go...")
	// Verificar que Go esté instalado
	goVersionResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"go", "version"})
	if err != nil {
		logrus.Errorf("Error verificando versión de Go: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error verificando versión de Go: %v", err))
		return
	}
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Go instalado: %s", goVersionResult.Output))
	// Verificar el contenido del directorio
	lsResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"ls", "-la", "/app/src"})
	if err != nil {
		logrus.Errorf("Error listando directorio: %v", err)
	} else {
		sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Contenido del directorio: %s", lsResult.Output))
	}

	// Verificar go.mod y mostrar información para debug
	sendHybridLogMessage(ctx, app.ID, "info", "Verificando go.mod...")
	_, err = runtime.ExecuteCommand(context.Background(), container.ID, []string{"ls", "-la", "/app/src/go.mod"})
	if err == nil {
		// go.mod existe, mostrar su contenido para debug
		goModContent, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"cat", "/app/src/go.mod"})
		if err == nil {
			sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Contenido de go.mod: %s", goModContent.Output))
		}
	} else {
		sendHybridLogMessage(ctx, app.ID, "info", "No se encontró go.mod, se creará uno automáticamente")
	}

	// Intentar compilar con más información de debug
	buildResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"sh", "-c", "cd /app/src && go mod tidy && go build -v -o /app/app ."})
	if err != nil {
		logrus.Errorf("Error compilando aplicación: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error compilando aplicación: %v", err))
		return
	}
	if buildResult.ExitCode != 0 {
		logrus.Errorf("Error compilando aplicación: %s", buildResult.Error)
		logrus.Errorf("Output de compilación: %s", buildResult.Output)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error compilando aplicación: %s\nOutput: %s", buildResult.Error, buildResult.Output))
		return
	}

	sendHybridLogMessage(ctx, app.ID, "info", "Ejecutando aplicación...")
	// Ejecutar la app en background con el puerto correcto
	execCmd := fmt.Sprintf("cd /app && PORT=%d nohup ./app > /app/app.log 2>&1 &", app.Port)
	execResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"sh", "-c", execCmd})
	if err != nil {
		logrus.Errorf("Error ejecutando aplicación: %v", err)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error ejecutando aplicación: %v", err))
		return
	}
	if execResult.ExitCode != 0 {
		logrus.Errorf("Error ejecutando aplicación: %s", execResult.Error)
		handleUnifiedDeployError(ctx, app, fmt.Sprintf("Error ejecutando aplicación: %s", execResult.Error))
		return
	}

	sendHybridLogMessage(ctx, app.ID, "success", "Aplicación Go compilada y ejecutada exitosamente")

	// Actualizar aplicación con información del contenedor
	app.Status = database.StatusRunning
	app.ContainerID = sql.NullString{String: container.ID, Valid: true}
	app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	app.ErrorMsg = sql.NullString{String: "", Valid: true}

	if err := ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación: %v", err)
	}

	// Mensajes informativos finales detallados
	sendHybridLogMessage(ctx, app.ID, "success", "🎉 ¡Deployment completado exitosamente!")
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📋 Información del contenedor:"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • ID: %s", container.ID))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Runtime: containerd"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • IP: %s", containerIP))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Puerto: %d", app.Port))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Lenguaje: %s", language))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Estado: %s", app.Status))

	sendHybridLogMessage(ctx, app.ID, "success", fmt.Sprintf("🌐 Aplicación disponible en: http://%s:%d", containerIP, app.Port))
	sendHybridLogMessage(ctx, app.ID, "success", fmt.Sprintf("🔗 URL local: http://localhost:%d", app.Port))

	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📊 Recursos asignados:"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Memoria: 512MB"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • CPU: 512 shares"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Red: host networking"))

	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📝 Logs disponibles en: /app/app.log dentro del contenedor"))
	sendHybridLogMessage(ctx, app.ID, "success", fmt.Sprintf("✅ Contenedor %s listo y funcionando", container.ID))
}

// unifiedRedeployApp ejecuta el redeploy usando el runtime factory
func unifiedRedeployApp(ctx *HybridContext, app *database.App, factory runtimePkg.RuntimeFactory) {
	logrus.Infof("Iniciando redeploy unificado de: %s (%s)", app.Name, app.ID)

	// Obtener runtime preferido para el redeploy
	preferredRuntime := factory.GetPreferredRuntime()
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Iniciando redeploy con runtime %s", preferredRuntime))

	// Actualizar estado a redeploying
	app.Status = database.StatusRedeploying
	app.ErrorMsg = sql.NullString{String: "", Valid: true}
	if err := ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
	}); err != nil {
		logrus.Errorf("Error actualizando estado de redeploy: %v", err)
	}

	// Crear runtime específico
	runtime, err := factory.CreateRuntime(preferredRuntime)
	if err != nil {
		logrus.Errorf("Error creando runtime %s para redeploy: %v", preferredRuntime, err)

		// Intentar fallback a Docker si containerd falla
		if preferredRuntime == runtimePkg.RuntimeTypeContainerd {
			logrus.Warnf("Containerd no disponible para redeploy, intentando fallback a Docker")
			sendHybridLogMessage(ctx, app.ID, "warning", "Containerd no disponible, usando Docker como fallback")

			// Verificar si Docker está disponible
			availableRuntimes := factory.GetAvailableRuntimes()
			dockerAvailable := false
			for _, rt := range availableRuntimes {
				if rt == runtimePkg.RuntimeTypeDocker {
					dockerAvailable = true
					break
				}
			}

			if dockerAvailable {
				preferredRuntime = runtimePkg.RuntimeTypeDocker
				logrus.Infof("Cambiando a runtime Docker para redeploy")
				sendHybridLogMessage(ctx, app.ID, "info", "Cambiando a runtime Docker")
			} else {
				handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error creando runtime %s y no hay fallback disponible: %v", preferredRuntime, err))
				return
			}
		} else {
			handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error creando runtime %s: %v", preferredRuntime, err))
			return
		}
	}

	// Si el runtime se creó exitosamente, cerrarlo al final
	if runtime != nil {
		defer runtime.Close()
	}

	// Configurar callback de eventos
	if runtime != nil {
		runtime.SetEventCallback(func(event runtimePkg.Event) {
			sendUnifiedRuntimeEvent(ctx, app.ID, event)
		})
	}

	// Ejecutar redeploy según el runtime
	switch preferredRuntime {
	case runtimePkg.RuntimeTypeDocker:
		// Usar el sistema Docker existente
		regularCtx := ctx.Context
		redeployExistingApp(regularCtx, app)
	case runtimePkg.RuntimeTypeContainerd:
		if runtime != nil {
			redeployWithContainerd(ctx, app, runtime)
		} else {
			// Fallback a Docker si no se pudo crear el runtime containerd
			logrus.Warnf("No se pudo crear runtime containerd para redeploy, usando Docker como fallback")
			regularCtx := ctx.Context
			redeployExistingApp(regularCtx, app)
		}
	default:
		// Fallback a Docker
		logrus.Warnf("Runtime %s no implementado para redeploy, usando Docker como fallback", preferredRuntime)
		regularCtx := ctx.Context
		redeployExistingApp(regularCtx, app)
	}
}

// redeployWithContainerd ejecuta el redeploy usando containerd
func redeployWithContainerd(ctx *HybridContext, app *database.App, runtime runtimePkg.ContainerRuntime) {
	sendHybridLogMessage(ctx, app.ID, "info", "🔄 Iniciando redeploy con containerd...")
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📦 Aplicación: %s (%s)", app.Name, app.ID))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("🔗 Repositorio: %s", app.RepoUrl))

	// Verificar que el runtime containerd esté realmente disponible
	if runtime == nil {
		logrus.Warnf("Runtime containerd es nil, usando Docker como fallback")
		sendHybridLogMessage(ctx, app.ID, "warning", "Runtime containerd no disponible, usando Docker como fallback")
		regularCtx := ctx.Context
		redeployExistingApp(regularCtx, app)
		return
	}

	// Cargar variables de entorno existentes de la base de datos
	existingEnvVars, err := ctx.queries.GetAppEnvVars(context.Background(), app.ID)
	if err != nil {
		logrus.Warnf("Error cargando variables de entorno para redeploy: %v", err)
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

	// Detectar lenguaje
	sendHybridLogMessage(ctx, app.ID, "info", "Detectando lenguaje...")
	language, err := detectLanguage(app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error detectando lenguaje en redeploy: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error detectando lenguaje: %v", err))
		return
	}
	app.Language = sql.NullString{String: language, Valid: true}
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Lenguaje detectado: %s", language))

	// Parar contenedor anterior si existe
	if app.ContainerID.String != "" {
		sendHybridLogMessage(ctx, app.ID, "info", "Deteniendo contenedor anterior...")
		if err := runtime.StopContainer(context.Background(), app.ContainerID.String); err != nil {
			logrus.Warnf("Error deteniendo contenedor anterior %s: %v", app.ContainerID.String, err)
			sendHybridLogMessage(ctx, app.ID, "warning", fmt.Sprintf("Error deteniendo contenedor anterior: %v", err))
		} else {
			sendHybridLogMessage(ctx, app.ID, "info", "Contenedor anterior detenido exitosamente")
		}

		// Eliminar contenedor anterior
		if err := runtime.RemoveContainer(context.Background(), app.ContainerID.String); err != nil {
			logrus.Warnf("Error eliminando contenedor anterior %s: %v", app.ContainerID.String, err)
			sendHybridLogMessage(ctx, app.ID, "warning", fmt.Sprintf("Error eliminando contenedor anterior: %v", err))
		} else {
			sendHybridLogMessage(ctx, app.ID, "info", "Contenedor anterior eliminado exitosamente")
		}

		// Limpiar container ID
		app.ContainerID = sql.NullString{String: "", Valid: true}
	}

	// Limpieza adicional para containerd - eliminar snapshots y contenedores huérfanos
	sendHybridLogMessage(ctx, app.ID, "info", "Limpiando recursos containerd...")
	cleanupContainerdResources(ctx, app.ID, runtime)

	// Determinar imagen base según el lenguaje
	baseImage := getContainerdBaseImage(language)
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Usando imagen base: %s", baseImage))

	// Preparar variables de entorno con el puerto correcto
	envVarsMap := convertEnvVarsToMap(envVars)

	// Agregar variables de entorno del sistema
	envVarsMap["PORT"] = fmt.Sprintf("%d", app.Port)
	envVarsMap["DIPLO_APP_ID"] = app.ID
	envVarsMap["DIPLO_APP_NAME"] = app.Name
	envVarsMap["DIPLO_APP_PORT"] = fmt.Sprintf("%d", app.Port)

	// Generar nombre único para el contenedor para evitar conflictos
	containerName := fmt.Sprintf("%s_%d", app.ID, time.Now().Unix())

	// Crear request para el nuevo contenedor
	containerReq := &runtimePkg.CreateContainerRequest{
		Name:        containerName, // Usar nombre único para evitar conflictos
		Image:       baseImage,
		Command:     []string{"sleep", "infinity"}, // Comando temporal para mantener contenedor vivo
		WorkingDir:  "/app",
		Environment: envVarsMap,
		Ports: []runtimePkg.PortMapping{
			{
				HostPort:      int(app.Port),
				ContainerPort: int(app.Port), // Usar el mismo puerto que el host
				Protocol:      "tcp",
			},
		},
		NetworkMode: "host", // Usar host networking para containerd
		Labels: map[string]string{
			"app.id":   app.ID,
			"app.name": app.Name,
			"runtime":  "containerd",
		},
		Resources: &runtimePkg.ResourceConfig{
			Memory:    512 * 1024 * 1024, // 512MB
			CPUShares: 512,
		},
	}

	// Crear nuevo contenedor con reintentos
	sendHybridLogMessage(ctx, app.ID, "info", "Creando nuevo contenedor containerd...")
	var container *runtimePkg.Container
	createMaxRetries := 3
	createRetryDelay := 2 * time.Second

	for i := 0; i < createMaxRetries; i++ {
		container, err = runtime.CreateContainer(containerReq)
		if err == nil {
			break
		}

		logrus.Warnf("Error creando contenedor containerd (intento %d/%d): %v", i+1, createMaxRetries, err)

		if i < createMaxRetries-1 {
			sendHybridLogMessage(ctx, app.ID, "warning", fmt.Sprintf("Error creando contenedor, reintentando en %v...", createRetryDelay))
			time.Sleep(createRetryDelay)

			// Limpiar recursos nuevamente antes del reintento
			cleanupContainerdResources(ctx, app.ID, runtime)
		}
	}

	if err != nil {
		logrus.Errorf("Error creando contenedor containerd después de %d intentos: %v", createMaxRetries, err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error creando contenedor containerd después de %d intentos: %v", createMaxRetries, err))
		return
	}

	// Iniciar nuevo contenedor
	sendHybridLogMessage(ctx, app.ID, "info", "Iniciando nuevo contenedor containerd...")
	if err := runtime.StartContainer(context.Background(), container.ID); err != nil {
		logrus.Errorf("Error iniciando contenedor containerd: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error iniciando contenedor containerd: %v", err))
		return
	}

	// Esperar a que el contenedor esté corriendo
	sendHybridLogMessage(ctx, app.ID, "info", "Esperando a que el contenedor esté listo...")
	containerIP := ""
	maxRetries := 10
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		// Verificar estado del contenedor
		containerInfo, err := runtime.GetContainer(container.ID)
		if err != nil {
			logrus.Warnf("No se pudo obtener información del contenedor containerd (intento %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryDelay)
			continue
		}

		if containerInfo.Status != runtimePkg.ContainerStatusRunning {
			logrus.Infof("Contenedor no está corriendo aún (estado: %s), esperando... (intento %d/%d)", containerInfo.Status, i+1, maxRetries)
			time.Sleep(retryDelay)
			continue
		}

		// Intentar obtener la IP
		containerIP, err = runtime.GetContainerIP(container.ID)
		if err != nil {
			logrus.Warnf("No se pudo obtener IP del contenedor containerd (intento %d/%d): %v", i+1, maxRetries, err)
			time.Sleep(retryDelay)
			continue
		}

		if containerIP == "" {
			logrus.Warnf("IP del contenedor está vacía (intento %d/%d), esperando...", i+1, maxRetries)
			time.Sleep(retryDelay)
			continue
		}

		// Si llegamos aquí, tenemos una IP válida
		logrus.Infof("Contenedor containerd listo con IP: %s", containerIP)
		break
	}

	// Si después de todos los intentos no tenemos IP, fallar el redeploy
	if containerIP == "" {
		// Leer logs del contenedor para mostrar el error real
		logs, logErr := runtime.GetContainerLogs(context.Background(), container.ID)
		logOutput := ""
		if logErr == nil && logs != nil {
			defer logs.Close()
			buf := new(strings.Builder)
			io.Copy(buf, logs)
			logOutput = buf.String()
		}
		errorMsg := fmt.Sprintf("No se pudo obtener IP del contenedor containerd después de múltiples intentos. Logs:\n%s", logOutput)
		logrus.Error(errorMsg)
		handleUnifiedRedeployError(ctx, app, errorMsg)
		return
	}

	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Contenedor containerd iniciado exitosamente con IP: %s", containerIP))

	// Verificar que el contenedor esté realmente listo antes de ejecutar comandos
	sendHybridLogMessage(ctx, app.ID, "info", "Verificando que el contenedor esté listo para comandos...")
	readyCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"echo", "container-ready"})
	if err != nil {
		logrus.Errorf("Error verificando que el contenedor esté listo: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error verificando que el contenedor esté listo: %v", err))
		return
	}
	if readyCheck.ExitCode != 0 {
		logrus.Errorf("Contenedor no está listo para comandos: %s", readyCheck.Error)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Contenedor no está listo para comandos: %s", readyCheck.Error))
		return
	}
	sendHybridLogMessage(ctx, app.ID, "info", "Contenedor listo para comandos")

	// Instalar git si es necesario (para imágenes Alpine)
	sendHybridLogMessage(ctx, app.ID, "info", "Verificando dependencias...")
	gitCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "git"})
	if err != nil {
		logrus.Errorf("Error verificando git: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error verificando git: %v", err))
		return
	}

	if gitCheck.ExitCode != 0 {
		sendHybridLogMessage(ctx, app.ID, "info", "Git no encontrado, instalando...")

		// Detectar el gestor de paquetes disponible
		packageManager := "apk" // Por defecto Alpine

		// Verificar si es Alpine Linux
		sendHybridLogMessage(ctx, app.ID, "info", "Detectando gestor de paquetes...")
		alpineCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "apk"})
		if err != nil {
			logrus.Errorf("Error verificando apk: %v", err)
		} else {
			sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Verificación apk: exit_code=%d, output=%s", alpineCheck.ExitCode, alpineCheck.Output))
		}

		if err == nil && alpineCheck.ExitCode == 0 {
			packageManager = "apk"
			installResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"apk", "add", "--no-cache", "git"})
			if err != nil {
				logrus.Errorf("Error instalando git con apk: %v", err)
				handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error instalando git con apk: %v", err))
				return
			}
			if installResult.ExitCode != 0 {
				logrus.Errorf("Error instalando git con apk: %s", installResult.Error)
				handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error instalando git con apk: %s", installResult.Error))
				return
			}
		} else {
			// Intentar con apt (Ubuntu/Debian)
			aptCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "apt"})
			if err != nil {
				logrus.Errorf("Error verificando apt: %v", err)
			} else {
				sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Verificación apt: exit_code=%d, output=%s", aptCheck.ExitCode, aptCheck.Output))
			}

			if err == nil && aptCheck.ExitCode == 0 {
				packageManager = "apt"
				// Actualizar repositorios primero
				updateResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"apt-get", "update"})
				if err != nil {
					logrus.Errorf("Error actualizando repositorios: %v", err)
				}
				if updateResult.ExitCode != 0 {
					logrus.Warnf("Error actualizando repositorios: %s", updateResult.Error)
				}

				installResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"apt-get", "install", "-y", "git"})
				if err != nil {
					logrus.Errorf("Error instalando git con apt: %v", err)
					handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error instalando git con apt: %v", err))
					return
				}
				if installResult.ExitCode != 0 {
					logrus.Errorf("Error instalando git con apt: %s", installResult.Error)
					handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error instalando git con apt: %s", installResult.Error))
					return
				}
			} else {
				// Intentar con yum (RHEL/CentOS)
				yumCheck, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"which", "yum"})
				if err != nil {
					logrus.Errorf("Error verificando yum: %v", err)
				} else {
					sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Verificación yum: exit_code=%d, output=%s", yumCheck.ExitCode, yumCheck.Output))
				}

				if err == nil && yumCheck.ExitCode == 0 {
					packageManager = "yum"
					installResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"yum", "install", "-y", "git"})
					if err != nil {
						logrus.Errorf("Error instalando git con yum: %v", err)
						handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error instalando git con yum: %v", err))
						return
					}
					if installResult.ExitCode != 0 {
						logrus.Errorf("Error instalando git con yum: %s", installResult.Error)
						handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error instalando git con yum: %s", installResult.Error))
						return
					}
				} else {
					// No se pudo detectar el gestor de paquetes
					errorMsg := "No se pudo detectar el gestor de paquetes (apk/apt/yum) para instalar git"
					logrus.Error(errorMsg)
					handleUnifiedRedeployError(ctx, app, errorMsg)
					return
				}
			}
		}

		sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Git instalado correctamente usando %s", packageManager))
	} else {
		sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Git ya está disponible: %s", gitCheck.Output))
	}

	// Clonar repositorio PRIMERO
	sendHybridLogMessage(ctx, app.ID, "info", "Clonando repositorio...")
	cloneResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"git", "clone", app.RepoUrl, "/app/src"})
	if err != nil {
		logrus.Errorf("Error clonando repositorio: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error clonando repositorio: %v", err))
		return
	}
	if cloneResult.ExitCode != 0 {
		logrus.Errorf("Error clonando repositorio: %s", cloneResult.Error)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error clonando repositorio: %s", cloneResult.Error))
		return
	}

	// Compilación Go con debug
	sendHybridLogMessage(ctx, app.ID, "info", "Compilando aplicación Go...")
	// Verificar que Go esté instalado
	goVersionResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"go", "version"})
	if err != nil {
		logrus.Errorf("Error verificando versión de Go: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error verificando versión de Go: %v", err))
		return
	}
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Go instalado: %s", goVersionResult.Output))
	// Verificar el contenido del directorio
	lsResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"ls", "-la", "/app/src"})
	if err != nil {
		logrus.Errorf("Error listando directorio: %v", err)
	} else {
		sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Contenido del directorio: %s", lsResult.Output))
	}

	// Verificar go.mod y mostrar información para debug
	sendHybridLogMessage(ctx, app.ID, "info", "Verificando go.mod...")
	_, err = runtime.ExecuteCommand(context.Background(), container.ID, []string{"ls", "-la", "/app/src/go.mod"})
	if err == nil {
		// go.mod existe, mostrar su contenido para debug
		goModContent, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"cat", "/app/src/go.mod"})
		if err == nil {
			sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("Contenido de go.mod: %s", goModContent.Output))
		}
	} else {
		sendHybridLogMessage(ctx, app.ID, "info", "No se encontró go.mod, se creará uno automáticamente")
	}

	// Intentar compilar con más información de debug
	buildResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"sh", "-c", "cd /app/src && go mod tidy && go build -v -o /app/app ."})
	if err != nil {
		logrus.Errorf("Error compilando aplicación: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error compilando aplicación: %v", err))
		return
	}
	if buildResult.ExitCode != 0 {
		logrus.Errorf("Error compilando aplicación: %s", buildResult.Error)
		logrus.Errorf("Output de compilación: %s", buildResult.Output)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error compilando aplicación: %s\nOutput: %s", buildResult.Error, buildResult.Output))
		return
	}

	sendHybridLogMessage(ctx, app.ID, "info", "Ejecutando aplicación...")
	// Ejecutar la app en background con el puerto correcto
	execCmd := fmt.Sprintf("cd /app && PORT=%d nohup ./app > /app/app.log 2>&1 &", app.Port)
	execResult, err := runtime.ExecuteCommand(context.Background(), container.ID, []string{"sh", "-c", execCmd})
	if err != nil {
		logrus.Errorf("Error ejecutando aplicación: %v", err)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error ejecutando aplicación: %v", err))
		return
	}
	if execResult.ExitCode != 0 {
		logrus.Errorf("Error ejecutando aplicación: %s", execResult.Error)
		handleUnifiedRedeployError(ctx, app, fmt.Sprintf("Error ejecutando aplicación: %s", execResult.Error))
		return
	}

	sendHybridLogMessage(ctx, app.ID, "success", "Aplicación Go compilada y ejecutada exitosamente")

	// Actualizar aplicación con información del nuevo contenedor
	app.Status = database.StatusRunning
	app.ContainerID = sql.NullString{String: container.ID, Valid: true}
	app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	app.ErrorMsg = sql.NullString{String: "", Valid: true}

	if err := ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación después del redeploy: %v", err)
	}

	// Mensajes informativos finales detallados
	sendHybridLogMessage(ctx, app.ID, "success", "🎉 ¡Redeploy completado exitosamente!")
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📋 Información del contenedor:"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • ID: %s", container.ID))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Runtime: containerd"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • IP: %s", containerIP))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Puerto: %d", app.Port))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Lenguaje: %s", language))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Estado: %s", app.Status))

	sendHybridLogMessage(ctx, app.ID, "success", fmt.Sprintf("🌐 Aplicación disponible en: http://%s:%d", containerIP, app.Port))
	sendHybridLogMessage(ctx, app.ID, "success", fmt.Sprintf("🔗 URL local: http://localhost:%d", app.Port))

	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📊 Recursos asignados:"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Memoria: 512MB"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • CPU: 512 shares"))
	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("   • Red: host networking"))

	sendHybridLogMessage(ctx, app.ID, "info", fmt.Sprintf("📝 Logs disponibles en: /app/app.log dentro del contenedor"))
	sendHybridLogMessage(ctx, app.ID, "success", fmt.Sprintf("✅ Contenedor %s listo y funcionando", container.ID))
}

func convertEnvVarsToMap(envVars []models.EnvVar) map[string]string {
	result := make(map[string]string)
	for _, env := range envVars {
		result[env.Name] = env.Value
	}
	return result
}

// Error handling functions

func handleUnifiedDeployError(ctx *HybridContext, app *database.App, errorMsg string) {
	app.Status = database.StatusError
	app.ErrorMsg = sql.NullString{String: errorMsg, Valid: true}
	if err := ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación con error de deployment: %v", err)
	}
	sendHybridLogMessage(ctx, app.ID, "error", errorMsg)
}

func handleUnifiedRedeployError(ctx *HybridContext, app *database.App, errorMsg string) {
	app.Status = database.StatusError
	app.ErrorMsg = sql.NullString{String: errorMsg, Valid: true}
	if err := ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación con error de redeploy: %v", err)
	}
	sendHybridLogMessage(ctx, app.ID, "error", errorMsg)
}

// sendHybridLogMessage envía un mensaje de log desde el contexto híbrido
func sendHybridLogMessage(ctx *HybridContext, appID, logType, message string) {
	// Usar el contexto regular para enviar logs
	regularCtx := ctx.Context
	regularCtx.logMu.RLock()
	if logChan, exists := regularCtx.logChannels[appID]; exists {
		logMsg := createLogMessage(logType, message)
		select {
		case logChan <- logMsg:
		default:
			// Canal lleno, ignorar mensaje
		}
	}
	regularCtx.logMu.RUnlock()
}

func sendUnifiedRuntimeEvent(ctx *HybridContext, appID string, event runtimePkg.Event) {
	// Enviar evento a través del sistema de logs
	eventType := "info"
	switch event.Type {
	case "container_create_success", "container_start_success":
		eventType = "success"
	case "container_create_error", "container_start_error":
		eventType = "error"
	}

	sendHybridLogMessage(ctx, appID, eventType, event.Message)
}

// getContainerdBaseImage retorna la imagen base para containerd según el lenguaje
func getContainerdBaseImage(language string) string {
	switch language {
	case "go":
		return "docker.io/library/golang:1.24-alpine"
	case "javascript", "node":
		return "docker.io/library/node:22-alpine"
	case "python":
		return "docker.io/library/python:3.13-alpine"
	case "rust":
		return "docker.io/library/rust:1.83-alpine"
	default:
		return "docker.io/library/ubuntu:24.04"
	}
}

// cleanupContainerdResources limpia recursos containerd para evitar conflictos
func cleanupContainerdResources(ctx *HybridContext, appID string, runtime runtimePkg.ContainerRuntime) {
	// Verificar que el runtime esté disponible antes de intentar limpiar
	if runtime == nil {
		logrus.Warnf("Runtime es nil, saltando limpieza de recursos")
		sendHybridLogMessage(ctx, appID, "warning", "Runtime no disponible, saltando limpieza de recursos")
		return
	}

	// Intentar eliminar contenedores con nombres similares que puedan estar huérfanos
	containerNames := []string{
		appID,
		fmt.Sprintf("diplo-%s", appID),
		fmt.Sprintf("app_%s", appID),
	}

	cleanupErrors := 0
	for _, containerName := range containerNames {
		// Intentar detener el contenedor si está corriendo
		if err := runtime.StopContainer(context.Background(), containerName); err != nil {
			logrus.Debugf("No se pudo detener contenedor %s (puede no existir): %v", containerName, err)
			cleanupErrors++
		}

		// Intentar eliminar el contenedor
		if err := runtime.RemoveContainer(context.Background(), containerName); err != nil {
			logrus.Debugf("No se pudo eliminar contenedor %s (puede no existir): %v", containerName, err)
			cleanupErrors++
		}
	}

	// Esperar un momento para que el runtime procese las eliminaciones
	time.Sleep(1 * time.Second)

	if cleanupErrors > 0 {
		sendHybridLogMessage(ctx, appID, "warning", fmt.Sprintf("Limpieza completada con %d errores (normal si los contenedores no existían)", cleanupErrors))
	} else {
		sendHybridLogMessage(ctx, appID, "info", "Limpieza de recursos completada exitosamente")
	}
}
