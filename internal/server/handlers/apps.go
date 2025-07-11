package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/dto"
	runtimePkg "github.com/rodrwan/diplo/internal/runtime"
	"github.com/sirupsen/logrus"
)

func ListAppsHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	apps, err := ctx.queries.GetAllApps(r.Context())
	if err != nil {
		logrus.Errorf("Error obteniendo aplicaciones: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicaciones"}, err
	}

	appsDTO := make([]*dto.App, 0, len(apps))
	for _, app := range apps {
		appsDTO = append(appsDTO, &dto.App{
			ID:          app.ID,
			Name:        app.Name,
			RepoUrl:     app.RepoUrl,
			Language:    app.Language.String,
			Port:        int(app.Port),
			ContainerID: app.ContainerID.String,
			ImageID:     app.ImageID.String,
			Status:      app.Status.String,
			ErrorMsg:    app.ErrorMsg.String,
		})
	}

	return Response{Code: http.StatusOK, Data: appsDTO}, nil
}

func GetAppHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	return Response{Code: http.StatusOK, Data: app}, nil
}

func DeleteAppHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación para eliminar: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	logrus.Infof("Iniciando eliminación de aplicación: %s (%s)", app.Name, app.ID)

	// Eliminar contenedor si existe usando el método híbrido
	if app.ContainerID.String != "" {
		if err := deleteContainerHybrid(ctx, &app); err != nil {
			logrus.Warnf("Error eliminando contenedor %s: %v", app.ContainerID.String, err)
		}
	}

	// Eliminar imagen si existe usando el método híbrido
	if app.ImageID.String != "" {
		if err := deleteImageHybrid(ctx, &app); err != nil {
			logrus.Warnf("Error eliminando imagen %s: %v", app.ImageID.String, err)
		}
	}

	// Eliminar aplicación de la base de datos
	if err := ctx.queries.DeleteApp(r.Context(), appID); err != nil {
		logrus.Errorf("Error eliminando aplicación de la base de datos: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error eliminando aplicación"}, err
	}

	logrus.Infof("Aplicación eliminada exitosamente: %s (%s)", app.Name, app.ID)
	return Response{Code: http.StatusOK, Message: "Aplicación eliminada exitosamente"}, nil
}

// deleteContainerHybrid elimina un contenedor usando el runtime apropiado
func deleteContainerHybrid(ctx *Context, app *database.App) error {
	containerID := app.ContainerID.String
	logrus.Infof("Eliminando contenedor: %s", containerID)

	// Intentar determinar el runtime basándose en el prefijo del container ID
	runtimeType := inferRuntimeFromContainerID(containerID)

	switch runtimeType {
	case runtimePkg.RuntimeTypeDocker:
		// Usar Docker client existente
		if err := ctx.docker.StopContainer(containerID); err != nil {
			return fmt.Errorf("error eliminando contenedor Docker: %w", err)
		}
		logrus.Infof("Contenedor Docker eliminado: %s", containerID)

	case runtimePkg.RuntimeTypeLXC:
		// Para LXC, usar el cliente específico
		lxcClient, err := runtimePkg.NewLXCClient()
		if err != nil {
			logrus.Warnf("Error creando cliente LXC, usando Docker como fallback: %v", err)
			return ctx.docker.StopContainer(containerID)
		}
		defer lxcClient.Close()

		if err := lxcClient.StopContainer(context.Background(), containerID); err != nil {
			return fmt.Errorf("error eliminando contenedor LXC: %w", err)
		}

		if err := lxcClient.RemoveContainer(context.Background(), containerID); err != nil {
			return fmt.Errorf("error removiendo contenedor LXC: %w", err)
		}
		logrus.Infof("Contenedor LXC eliminado: %s", containerID)

	case runtimePkg.RuntimeTypeContainerd:
		// Para containerd, usar el cliente específico
		containerdClient, err := runtimePkg.NewContainerdClient("", "")
		if err != nil {
			logrus.Warnf("Error creando cliente containerd, usando Docker como fallback: %v", err)
			return ctx.docker.StopContainer(containerID)
		}
		defer containerdClient.Close()

		if err := containerdClient.StopContainer(context.Background(), containerID); err != nil {
			return fmt.Errorf("error eliminando contenedor containerd: %w", err)
		}

		if err := containerdClient.RemoveContainer(context.Background(), containerID); err != nil {
			return fmt.Errorf("error removiendo contenedor containerd: %w", err)
		}
		logrus.Infof("Contenedor containerd eliminado: %s", containerID)

	default:
		// Fallback a Docker para aplicaciones existentes
		logrus.Infof("Runtime no determinado, usando Docker como fallback para contenedor: %s", containerID)
		if err := ctx.docker.StopContainer(containerID); err != nil {
			return fmt.Errorf("error eliminando contenedor (Docker fallback): %w", err)
		}
	}

	return nil
}

// deleteImageHybrid elimina una imagen usando el runtime apropiado
func deleteImageHybrid(ctx *Context, app *database.App) error {
	imageID := app.ImageID.String
	logrus.Infof("Eliminando imagen: %s", imageID)

	// Para imágenes, principalmente usamos Docker ya que es el que maneja builds
	// En el futuro se puede expandir para otros runtimes que manejen imágenes

	// Intentar eliminar imagen específica usando la nueva función RemoveImage
	if err := ctx.docker.RemoveImage(imageID); err != nil {
		logrus.Warnf("Error eliminando imagen específica %s: %v", imageID, err)

		// Fallback: intentar limpiar todas las imágenes de la app
		if err := ctx.docker.CleanupOldImages(app.ID, 0); err != nil {
			logrus.Warnf("Error usando CleanupOldImages como fallback: %v", err)
		}
	}

	// Ejecutar limpieza de imágenes dangling para limpiar capas huérfanas
	if err := ctx.docker.PruneDanglingImages(); err != nil {
		logrus.Warnf("Error limpiando imágenes dangling: %v", err)
	}

	logrus.Infof("Proceso de eliminación de imagen completado: %s", imageID)
	return nil
}

// inferRuntimeFromContainerID intenta determinar el runtime basándose en el container ID
func inferRuntimeFromContainerID(containerID string) runtimePkg.RuntimeType {
	if containerID == "" {
		return runtimePkg.RuntimeTypeDocker // Default fallback
	}

	// Patrones comunes de container IDs por runtime
	switch {
	case len(containerID) == 64: // Docker container IDs son típicamente 64 caracteres hex
		return runtimePkg.RuntimeTypeDocker
	case containerID[:4] == "lxc-": // LXC containers suelen tener prefijo
		return runtimePkg.RuntimeTypeLXC
	case containerID[:11] == "containerd-": // containerd puede tener prefijo específico
		return runtimePkg.RuntimeTypeContainerd
	default:
		// Si no se puede determinar, usar Docker como fallback
		logrus.Debugf("No se pudo determinar runtime para container ID: %s, usando Docker", containerID)
		return runtimePkg.RuntimeTypeDocker
	}
}

func HealthCheckHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	// Verificar que la aplicación tenga un contenedor
	if app.ContainerID.String == "" {
		return Response{Code: http.StatusNotFound, Message: "No hay contenedor asociado a esta aplicación"}, nil
	}

	// Realizar healthcheck interno
	healthStatus, err := performHealthCheck(ctx, &app)
	if err != nil {
		logrus.Errorf("Error en healthcheck para %s: %v", appID, err)
		return Response{Code: http.StatusInternalServerError, Message: "Error realizando healthcheck"}, err
	}

	return Response{Code: http.StatusOK, Data: healthStatus}, nil
}

// performHealthCheck realiza un healthcheck interno al contenedor
func performHealthCheck(ctx *Context, app *database.App) (map[string]interface{}, error) {
	containerID := app.ContainerID.String

	// Verificar estado del contenedor primero
	containerStatus, err := ctx.docker.GetContainerStatus(containerID)
	if err != nil {
		return map[string]interface{}{
			"healthy": false,
			"status":  "error",
			"message": fmt.Sprintf("Error verificando estado del contenedor: %v", err),
			"details": map[string]interface{}{
				"container_id": containerID,
				"error":        err.Error(),
			},
		}, nil
	}

	// Si el contenedor no está running, no está healthy
	if containerStatus != "running" {
		return map[string]interface{}{
			"healthy": false,
			"status":  "container_not_running",
			"message": fmt.Sprintf("Contenedor no está ejecutándose: %s", containerStatus),
			"details": map[string]interface{}{
				"container_id":     containerID,
				"container_status": containerStatus,
				"port":             app.Port,
			},
		}, nil
	}

	// Hacer ping HTTP interno al contenedor
	url := fmt.Sprintf("http://localhost:%d", app.Port)

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Crear request con timeout
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return map[string]interface{}{
			"healthy": false,
			"status":  "request_error",
			"message": fmt.Sprintf("Error creando request: %v", err),
			"details": map[string]interface{}{
				"url":   url,
				"error": err.Error(),
			},
		}, nil
	}

	// Ejecutar request
	resp, err := httpClient.Do(req)
	if err != nil {
		return map[string]interface{}{
			"healthy": false,
			"status":  "connection_error",
			"message": fmt.Sprintf("Error conectando al servicio: %v", err),
			"details": map[string]interface{}{
				"url":          url,
				"error":        err.Error(),
				"container_id": containerID,
			},
		}, nil
	}
	defer resp.Body.Close()

	// Verificar código de respuesta
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	statusText := "unhealthy"
	if healthy {
		statusText = "healthy"
	}

	return map[string]interface{}{
		"healthy": healthy,
		"status":  statusText,
		"message": fmt.Sprintf("Servicio respondió con código %d", resp.StatusCode),
		"details": map[string]interface{}{
			"url":              url,
			"http_status_code": resp.StatusCode,
			"container_id":     containerID,
			"container_status": containerStatus,
			"response_time_ms": time.Since(time.Now()).Milliseconds(),
			"timestamp":        time.Now().Format(time.RFC3339),
		},
	}, nil
}

// ListAppEnvVarsHandler obtiene todas las variables de entorno de una aplicación
func ListAppEnvVarsHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]

	// Verificar que la aplicación existe
	_, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, err
	}

	// Obtener variables de entorno
	envVars, err := ctx.queries.GetAppEnvVars(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo variables de entorno: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo variables de entorno"}, err
	}

	// Convertir a formato de respuesta
	envVarsResponse := make([]map[string]interface{}, 0, len(envVars))
	for _, env := range envVars {
		displayValue := env.Value

		// Descifrar valores secretos para mostrar
		if env.IsSecret.Bool {
			if decryptedValue, err := decryptValue(env.Value); err != nil {
				logrus.Errorf("Error descifrando valor secreto para %s: %v", env.Key, err)
				displayValue = "[ERROR_DESCIFRANDO]"
			} else {
				displayValue = decryptedValue
			}
		}

		envVarsResponse = append(envVarsResponse, map[string]interface{}{
			"id":         env.ID,
			"key":        env.Key,
			"value":      displayValue,
			"is_secret":  env.IsSecret.Bool,
			"created_at": env.CreatedAt.Time.Format(time.RFC3339),
			"updated_at": env.UpdatedAt.Time.Format(time.RFC3339),
		})
	}

	return Response{Code: http.StatusOK, Data: envVarsResponse}, nil
}

// GetAppEnvVarHandler obtiene una variable de entorno específica
func GetAppEnvVarHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]
	key := vars["key"]

	// Verificar que la aplicación existe
	_, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, err
	}

	// Obtener variable de entorno específica
	envVar, err := ctx.queries.GetAppEnvVar(r.Context(), database.GetAppEnvVarParams{
		AppID: appID,
		Key:   key,
	})
	if err != nil {
		logrus.Errorf("Error obteniendo variable de entorno: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Variable de entorno no encontrada"}, err
	}

	displayValue := envVar.Value

	// Descifrar valor secreto para mostrar
	if envVar.IsSecret.Bool {
		if decryptedValue, err := decryptValue(envVar.Value); err != nil {
			logrus.Errorf("Error descifrando valor secreto para %s: %v", envVar.Key, err)
			displayValue = "[ERROR_DESCIFRANDO]"
		} else {
			displayValue = decryptedValue
		}
	}

	envVarResponse := map[string]interface{}{
		"id":         envVar.ID,
		"key":        envVar.Key,
		"value":      displayValue,
		"is_secret":  envVar.IsSecret.Bool,
		"created_at": envVar.CreatedAt.Time.Format(time.RFC3339),
		"updated_at": envVar.UpdatedAt.Time.Format(time.RFC3339),
	}

	return Response{Code: http.StatusOK, Data: envVarResponse}, nil
}

// CreateAppEnvVarHandler crea una nueva variable de entorno para una aplicación
func CreateAppEnvVarHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]

	// Verificar que la aplicación existe
	_, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, err
	}

	// Decodificar request body
	var req struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		IsSecret bool   `json:"is_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return Response{Code: http.StatusBadRequest, Message: "JSON inválido"}, err
	}

	// Validar campos requeridos
	if req.Key == "" || req.Value == "" {
		return Response{Code: http.StatusBadRequest, Message: "Los campos 'key' y 'value' son requeridos"}, nil
	}

	// Validar longitud de campos
	if len(req.Key) > 100 {
		return Response{Code: http.StatusBadRequest, Message: "El nombre de la variable no puede exceder 100 caracteres"}, nil
	}
	if len(req.Value) > 1000 {
		return Response{Code: http.StatusBadRequest, Message: "El valor de la variable no puede exceder 1000 caracteres"}, nil
	}

	// Validar nombre de variable de entorno
	if !isValidEnvVarName(req.Key) {
		return Response{Code: http.StatusBadRequest, Message: "Nombre de variable de entorno inválido. Solo se permiten letras, números y guiones bajos"}, nil
	}

	// Validar valor de variable de entorno
	if !isValidEnvVarValue(req.Value) {
		return Response{Code: http.StatusBadRequest, Message: "Valor de variable de entorno contiene caracteres no permitidos"}, nil
	}

	// Verificar límite de variables de entorno por aplicación
	existingEnvVars, err := ctx.queries.GetAppEnvVars(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error verificando variables de entorno existentes: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error verificando variables de entorno"}, err
	}

	const maxEnvVars = 50 // Límite de 50 variables de entorno por aplicación
	if len(existingEnvVars) >= maxEnvVars {
		return Response{Code: http.StatusBadRequest, Message: "Límite máximo de variables de entorno alcanzado (50)"}, nil
	}

	// Verificar si ya existe una variable con ese nombre
	_, err = ctx.queries.GetAppEnvVar(r.Context(), database.GetAppEnvVarParams{
		AppID: appID,
		Key:   req.Key,
	})
	if err == nil {
		return Response{Code: http.StatusConflict, Message: "Variable de entorno ya existe"}, nil
	}

	// Cifrar el valor si es marcado como secreto
	valueToStore := req.Value
	if shouldEncryptValue(req.IsSecret) {
		encryptedValue, err := encryptValue(req.Value)
		if err != nil {
			logrus.Errorf("Error cifrando valor secreto: %v", err)
			return Response{Code: http.StatusInternalServerError, Message: "Error procesando valor secreto"}, err
		}
		valueToStore = encryptedValue
	}

	// Crear variable de entorno
	err = ctx.queries.CreateAppEnvVar(r.Context(), database.CreateAppEnvVarParams{
		AppID:     appID,
		Key:       req.Key,
		Value:     valueToStore,
		IsSecret:  sql.NullBool{Bool: req.IsSecret, Valid: true},
		UpdatedAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	if err != nil {
		logrus.Errorf("Error creando variable de entorno: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error creando variable de entorno"}, err
	}

	response := map[string]interface{}{
		"message": "Variable de entorno creada exitosamente",
		"key":     req.Key,
		"app_id":  appID,
	}

	return Response{Code: http.StatusCreated, Data: response}, nil
}

// UpdateAppEnvVarHandler actualiza una variable de entorno existente
func UpdateAppEnvVarHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]
	key := vars["key"]

	// Verificar que la aplicación existe
	_, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, err
	}

	// Verificar que la variable de entorno existe
	_, err = ctx.queries.GetAppEnvVar(r.Context(), database.GetAppEnvVarParams{
		AppID: appID,
		Key:   key,
	})
	if err != nil {
		logrus.Errorf("Error obteniendo variable de entorno: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Variable de entorno no encontrada"}, err
	}

	// Decodificar request body
	var req struct {
		Value    string `json:"value"`
		IsSecret bool   `json:"is_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return Response{Code: http.StatusBadRequest, Message: "JSON inválido"}, err
	}

	// Validar campos requeridos
	if req.Value == "" {
		return Response{Code: http.StatusBadRequest, Message: "El campo 'value' es requerido"}, nil
	}

	// Validar longitud del valor
	if len(req.Value) > 1000 {
		return Response{Code: http.StatusBadRequest, Message: "El valor de la variable no puede exceder 1000 caracteres"}, nil
	}

	// Validar valor de variable de entorno
	if !isValidEnvVarValue(req.Value) {
		return Response{Code: http.StatusBadRequest, Message: "Valor de variable de entorno contiene caracteres no permitidos"}, nil
	}

	// Cifrar el valor si es marcado como secreto
	valueToStore := req.Value
	if shouldEncryptValue(req.IsSecret) {
		encryptedValue, err := encryptValue(req.Value)
		if err != nil {
			logrus.Errorf("Error cifrando valor secreto: %v", err)
			return Response{Code: http.StatusInternalServerError, Message: "Error procesando valor secreto"}, err
		}
		valueToStore = encryptedValue
	}

	// Actualizar variable de entorno
	err = ctx.queries.UpdateAppEnvVar(r.Context(), database.UpdateAppEnvVarParams{
		AppID:     appID,
		Key:       key,
		Value:     valueToStore,
		IsSecret:  sql.NullBool{Bool: req.IsSecret, Valid: true},
		UpdatedAt: sql.NullTime{Time: time.Now(), Valid: true},
	})
	if err != nil {
		logrus.Errorf("Error actualizando variable de entorno: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error actualizando variable de entorno"}, err
	}

	response := map[string]interface{}{
		"message": "Variable de entorno actualizada exitosamente",
		"key":     key,
		"app_id":  appID,
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

// DeleteAppEnvVarHandler elimina una variable de entorno específica
func DeleteAppEnvVarHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]
	key := vars["key"]

	// Verificar que la aplicación existe
	_, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, err
	}

	// Verificar que la variable de entorno existe
	_, err = ctx.queries.GetAppEnvVar(r.Context(), database.GetAppEnvVarParams{
		AppID: appID,
		Key:   key,
	})
	if err != nil {
		logrus.Errorf("Error obteniendo variable de entorno: %v", err)
		return Response{Code: http.StatusNotFound, Message: "Variable de entorno no encontrada"}, err
	}

	// Eliminar variable de entorno
	err = ctx.queries.DeleteAppEnvVar(r.Context(), database.DeleteAppEnvVarParams{
		AppID: appID,
		Key:   key,
	})
	if err != nil {
		logrus.Errorf("Error eliminando variable de entorno: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error eliminando variable de entorno"}, err
	}

	response := map[string]interface{}{
		"message": "Variable de entorno eliminada exitosamente",
		"key":     key,
		"app_id":  appID,
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

// isValidEnvVarName valida nombres de variables de entorno con validaciones de seguridad mejoradas
func isValidEnvVarName(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}

	// El nombre debe empezar con una letra o guión bajo
	if !((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z') || name[0] == '_') {
		return false
	}

	// Prevenir variables del sistema sean sobrescritas
	systemVars := []string{
		"PATH", "HOME", "USER", "SHELL", "TERM", "PWD", "LANG", "LC_ALL",
		"LD_LIBRARY_PATH", "LD_PRELOAD", "TMPDIR", "TMP", "TEMP",
	}
	for _, sysVar := range systemVars {
		if name == sysVar {
			return false
		}
	}

	// Prevenir variables específicas de Docker y contenedores
	dockerVars := []string{
		"HOSTNAME", "DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH",
		"DOCKER_MACHINE_NAME", "DOCKER_BUILDKIT", "COMPOSE_PROJECT_NAME",
	}
	for _, dockerVar := range dockerVars {
		if name == dockerVar {
			return false
		}
	}

	// Prevenir variables internas de Diplo sean sobrescritas
	diploVars := []string{"DIPLO_APP_ID", "DIPLO_APP_NAME", "PORT"}
	for _, diploVar := range diploVars {
		if name == diploVar {
			return false
		}
	}

	// Prevenir variables que podrían ser usadas para ataques
	dangerousVars := []string{
		"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES",
		"NODE_OPTIONS", "PYTHONPATH", "RUBYLIB", "PERL5LIB", "CLASSPATH",
	}
	for _, dangerousVar := range dangerousVars {
		if name == dangerousVar {
			return false
		}
	}

	// Validación básica de caracteres (alfanuméricos y guión bajo solamente)
	for _, char := range name {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_') {
			return false
		}
	}

	return true
}

// isValidEnvVarValue valida valores de variables de entorno para prevenir ataques
func isValidEnvVarValue(value string) bool {
	if len(value) == 0 || len(value) > 1000 {
		return false
	}

	// Caracteres peligrosos que podrían ser usados para inyección
	dangerousChars := []string{
		"`", "$", "$(", "${", "&&", "||", ";", "|", "&",
		"<", ">", ">>", "<<", "\n", "\r", "\t",
	}

	for _, dangerous := range dangerousChars {
		if strings.Contains(value, dangerous) {
			return false
		}
	}

	// Prevenir valores que empiecen con caracteres sospechosos
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") {
		return false
	}

	return true
}
