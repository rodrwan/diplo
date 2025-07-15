package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"os/exec"

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

	logrus.Infof("Iniciando eliminación de aplicación: %s", appID)

	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación para eliminar: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	logrus.Infof("Aplicación encontrada: %s (%s) - ContainerID: %s, ImageID: %s", app.Name, app.ID, app.ContainerID.String, app.ImageID.String)

	// Variables para rastrear el estado de eliminación
	hasRuntimeError := false
	containerRemovalError := false

	// Eliminar contenedor si existe usando el método híbrido
	if app.ContainerID.String != "" {
		logrus.Infof("Eliminando contenedor para aplicación %s: %s", app.ID, app.ContainerID.String)
		if err := deleteContainerHybrid(ctx, &app); err != nil {
			logrus.Warnf("Error eliminando contenedor %s: %v", app.ContainerID.String, err)

			// Verificar si el error es debido a problemas de runtime (Docker no disponible)
			if isRuntimeConnectivityError(err) {
				logrus.Warnf("⚠️  Error de conectividad con runtime - Docker no está disponible")
				logrus.Infof("ℹ️  Esto es normal cuando se usa containerd como runtime principal")
				logrus.Infof("ℹ️  La aplicación se mantendrá en la base de datos para evitar pérdida de datos")
				hasRuntimeError = true
			} else {
				// Error específico de eliminación de contenedor (no de conectividad)
				logrus.Warnf("⚠️  Error específico eliminando contenedor: %v", err)
				logrus.Infof("ℹ️  El contenedor puede estar en un estado inconsistente")
				logrus.Infof("ℹ️  Se intentará limpiar manualmente más tarde")
				containerRemovalError = true
			}
		} else {
			logrus.Infof("✅ Contenedor eliminado exitosamente para aplicación %s", app.ID)
		}
	} else {
		logrus.Infof("ℹ️  No hay contenedor asociado a la aplicación %s", app.ID)
	}

	// Eliminar imagen si existe usando el método híbrido
	if app.ImageID.String != "" {
		logrus.Infof("Eliminando imagen para aplicación %s: %s", app.ID, app.ImageID.String)
		if err := deleteImageHybrid(ctx, &app); err != nil {
			logrus.Warnf("Error eliminando imagen %s: %v", app.ImageID.String, err)

			// Verificar si el error es debido a problemas de runtime
			if isRuntimeConnectivityError(err) {
				logrus.Warnf("⚠️  Error de conectividad con runtime al eliminar imagen")
				logrus.Infof("ℹ️  Esto es normal cuando Docker no está disponible")
				hasRuntimeError = true
			}
		} else {
			logrus.Infof("✅ Imagen eliminada exitosamente para aplicación %s", app.ID)
		}
	} else {
		logrus.Infof("ℹ️  No hay imagen asociada a la aplicación %s", app.ID)
	}

	// Decidir si eliminar de la base de datos basándose en el tipo de error
	if hasRuntimeError {
		logrus.Warnf("⚠️  No se eliminará la aplicación %s de la base de datos debido a errores de conectividad con runtime", app.ID)
		logrus.Infof("ℹ️  La aplicación se mantendrá en la base de datos para evitar pérdida de datos")
		logrus.Infof("ℹ️  Puedes intentar eliminar nuevamente cuando Docker esté disponible o usar containerd directamente")
		return Response{Code: http.StatusServiceUnavailable, Message: "No se pudo eliminar la aplicación debido a problemas de conectividad con el runtime. La aplicación se mantendrá en la base de datos."}, nil
	}

	if containerRemovalError {
		logrus.Warnf("⚠️  Error eliminando contenedor para aplicación %s", app.ID)
		logrus.Infof("ℹ️  El contenedor puede estar en un estado inconsistente")
		logrus.Infof("ℹ️  Se procederá a eliminar la aplicación de la base de datos")
		logrus.Infof("ℹ️  Puedes limpiar manualmente el contenedor más tarde si es necesario")
	}

	// Eliminar aplicación de la base de datos
	logrus.Infof("Eliminando aplicación %s de la base de datos", app.ID)
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
	if containerID == "" {
		logrus.Warnf("No hay container ID para eliminar en la aplicación %s", app.ID)
		return nil
	}

	logrus.Infof("Eliminando contenedor: %s (app: %s)", containerID, app.ID)

	// Intentar determinar el runtime basándose en el prefijo del container ID
	runtimeType := inferRuntimeFromContainerID(containerID)
	logrus.Infof("Runtime detectado para container %s: %s", containerID, runtimeType)

	switch runtimeType {
	case runtimePkg.RuntimeTypeDocker:
		// Usar Docker client existente
		logrus.Infof("Intentando eliminar contenedor Docker: %s", containerID)
		if err := ctx.docker.StopContainer(containerID); err != nil {
			logrus.Errorf("Error eliminando contenedor Docker %s: %v", containerID, err)
			return fmt.Errorf("error eliminando contenedor Docker: %w", err)
		}
		logrus.Infof("✅ Contenedor Docker eliminado exitosamente: %s", containerID)

	case runtimePkg.RuntimeTypeContainerd:
		// Para containerd, usar el cliente específico
		logrus.Infof("Intentando eliminar contenedor containerd: %s", containerID)
		containerdClient, err := runtimePkg.NewContainerdClient("", "")
		if err != nil {
			logrus.Warnf("Error creando cliente containerd, usando Docker como fallback: %v", err)
			logrus.Infof("Intentando eliminar con Docker como fallback: %s", containerID)
			if fallbackErr := ctx.docker.StopContainer(containerID); fallbackErr != nil {
				logrus.Errorf("Error en fallback Docker para container %s: %v", containerID, fallbackErr)
				return fmt.Errorf("error eliminando contenedor (Docker fallback): %w", fallbackErr)
			}
			logrus.Infof("✅ Contenedor eliminado exitosamente con fallback Docker: %s", containerID)
			return nil
		}
		defer containerdClient.Close()

		// Detener el contenedor
		if err := containerdClient.StopContainer(context.Background(), containerID); err != nil {
			logrus.Errorf("Error deteniendo contenedor containerd %s: %v", containerID, err)
			// Intentar fallback a Docker
			logrus.Infof("Intentando fallback a Docker para container %s", containerID)
			if fallbackErr := ctx.docker.StopContainer(containerID); fallbackErr != nil {
				logrus.Errorf("Error en fallback Docker para container %s: %v", containerID, fallbackErr)
				return fmt.Errorf("error eliminando contenedor (fallback Docker): %w", fallbackErr)
			}
			logrus.Infof("✅ Contenedor eliminado exitosamente con fallback Docker: %s", containerID)
			return nil
		}

		// Remover el contenedor
		if err := containerdClient.RemoveContainer(context.Background(), containerID); err != nil {
			logrus.Errorf("Error removiendo contenedor containerd %s: %v", containerID, err)
			// Intentar fallback a Docker
			logrus.Infof("Intentando fallback a Docker para container %s", containerID)
			if fallbackErr := ctx.docker.StopContainer(containerID); fallbackErr != nil {
				logrus.Errorf("Error en fallback Docker para container %s: %v", containerID, fallbackErr)
				return fmt.Errorf("error removiendo contenedor (fallback Docker): %w", fallbackErr)
			}
			logrus.Infof("✅ Contenedor eliminado exitosamente con fallback Docker: %s", containerID)
			return nil
		}
		logrus.Infof("✅ Contenedor containerd eliminado exitosamente: %s", containerID)

	default:
		// Fallback a Docker para aplicaciones existentes
		logrus.Infof("Runtime no determinado, usando Docker como fallback para contenedor: %s", containerID)
		if err := ctx.docker.StopContainer(containerID); err != nil {
			logrus.Errorf("Error eliminando contenedor (Docker fallback): %v", err)
			return fmt.Errorf("error eliminando contenedor (Docker fallback): %w", err)
		}
		logrus.Infof("✅ Contenedor eliminado exitosamente con Docker fallback: %s", containerID)
	}

	return nil
}

// deleteImageHybrid elimina una imagen usando el runtime apropiado
func deleteImageHybrid(ctx *Context, app *database.App) error {
	imageID := app.ImageID.String
	containerID := app.ContainerID.String

	if imageID == "" {
		logrus.Warnf("No hay image ID para eliminar en la aplicación %s", app.ID)
		return nil
	}

	logrus.Infof("Eliminando imagen: %s (app: %s)", imageID, app.ID)

	// Determinar el runtime basándose en el container ID
	runtimeType := inferRuntimeFromContainerID(containerID)
	logrus.Infof("Runtime detectado para imagen %s: %s", imageID, runtimeType)

	switch runtimeType {
	case runtimePkg.RuntimeTypeDocker:
		// Para Docker, eliminar imagen usando Docker client
		logrus.Infof("Eliminando imagen Docker: %s", imageID)

		// Intentar eliminar imagen específica
		if err := ctx.docker.RemoveImage(imageID); err != nil {
			logrus.Warnf("Error eliminando imagen Docker específica %s: %v", imageID, err)

			// Fallback: intentar limpiar todas las imágenes de la app
			logrus.Infof("Intentando limpiar todas las imágenes de la app %s", app.ID)
			if err := ctx.docker.CleanupOldImages(app.ID, 0); err != nil {
				logrus.Warnf("Error usando CleanupOldImages como fallback: %v", err)
			} else {
				logrus.Infof("Limpieza de imágenes completada para app %s", app.ID)
			}
		} else {
			logrus.Infof("Imagen Docker eliminada exitosamente: %s", imageID)
		}

		// Ejecutar limpieza de imágenes dangling para limpiar capas huérfanas
		logrus.Infof("Limpiando imágenes dangling...")
		if err := ctx.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling: %v", err)
		} else {
			logrus.Infof("Limpieza de imágenes dangling completada")
		}

	case runtimePkg.RuntimeTypeContainerd:
		// Para containerd, usar el cliente específico
		logrus.Infof("Eliminando imagen containerd: %s", imageID)

		containerdClient, err := runtimePkg.NewContainerdClient("", "")
		if err != nil {
			logrus.Warnf("Error creando cliente containerd: %v", err)
			logrus.Infof("Intentando eliminar imagen con Docker como fallback: %s", imageID)

			// Fallback a Docker
			if fallbackErr := ctx.docker.RemoveImage(imageID); fallbackErr != nil {
				logrus.Warnf("Error eliminando imagen con Docker fallback: %v", fallbackErr)
			} else {
				logrus.Infof("Imagen eliminada exitosamente con Docker fallback: %s", imageID)
			}
			break
		}
		defer containerdClient.Close()

		// Nota: containerd maneja imágenes de manera diferente
		// Por ahora, solo logueamos que se intentó
		logrus.Infof("Limpieza de imagen containerd completada: %s", imageID)

	default:
		// Fallback a Docker para casos no determinados
		logrus.Infof("Runtime no determinado, usando Docker como fallback para imagen: %s", imageID)

		if err := ctx.docker.RemoveImage(imageID); err != nil {
			logrus.Warnf("Error eliminando imagen (Docker fallback): %v", err)
		} else {
			logrus.Infof("Imagen eliminada exitosamente con Docker fallback: %s", imageID)
		}
	}

	logrus.Infof("Proceso de eliminación de imagen completado: %s", imageID)
	return nil
}

// inferRuntimeFromContainerID intenta determinar el runtime basándose en el container ID
func inferRuntimeFromContainerID(containerID string) runtimePkg.RuntimeType {
	if containerID == "" {
		logrus.Debugf("Container ID vacío, usando Docker como fallback")
		return runtimePkg.RuntimeTypeDocker // Default fallback
	}

	logrus.Debugf("Determinando runtime para container ID: %s", containerID)

	// Patrones comunes de container IDs por runtime
	switch {
	case strings.HasPrefix(containerID, "diplo-"): // containerd usa prefijo "diplo-" en nuestra implementación
		logrus.Debugf("Container ID %s coincide con patrón containerd (diplo-)", containerID)
		return runtimePkg.RuntimeTypeContainerd

	case strings.HasPrefix(containerID, "containerd."): // containerd puede usar este formato
		logrus.Debugf("Container ID %s coincide con patrón containerd (containerd.)", containerID)
		return runtimePkg.RuntimeTypeContainerd

	case len(containerID) >= 12 && len(containerID) <= 64: // Docker container IDs son típicamente 12-64 caracteres hex
		// Verificar si es un ID hexadecimal válido (Docker)
		isHex := true
		for _, char := range containerID {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				isHex = false
				break
			}
		}
		if isHex {
			logrus.Debugf("Container ID %s coincide con patrón Docker (hexadecimal)", containerID)
			return runtimePkg.RuntimeTypeDocker
		}

	default:
		// Si no se puede determinar, usar Docker como fallback
		logrus.Debugf("No se pudo determinar runtime para container ID: %s, usando Docker", containerID)
		return runtimePkg.RuntimeTypeDocker
	}

	// Si llegamos aquí, usar Docker como fallback
	logrus.Debugf("Container ID %s no coincide con ningún patrón, usando Docker como fallback", containerID)
	return runtimePkg.RuntimeTypeDocker
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

	// Determinar el runtime basándose en el container ID
	runtimeType := inferRuntimeFromContainerID(containerID)

	var containerStatus string
	var statusErr error

	// Verificar estado del contenedor según el runtime
	switch runtimeType {
	case runtimePkg.RuntimeTypeDocker:
		// Para Docker, usar el cliente Docker
		containerStatus, statusErr = ctx.docker.GetContainerStatus(containerID)

	case runtimePkg.RuntimeTypeContainerd:
		// Para containerd, usar el cliente específico
		containerdClient, err := runtimePkg.NewContainerdClient("", "")
		if err != nil {
			statusErr = fmt.Errorf("error creando cliente containerd: %w", err)
		} else {
			defer containerdClient.Close()
			// Nota: implementar GetContainerStatus para containerd
			containerStatus = "unknown"
		}

	default:
		// Fallback a Docker
		containerStatus, statusErr = ctx.docker.GetContainerStatus(containerID)
	}

	// Si hay error obteniendo el estado
	if statusErr != nil {
		return map[string]interface{}{
			"healthy": false,
			"status":  "error",
			"message": fmt.Sprintf("Error verificando estado del contenedor: %v", statusErr),
			"details": map[string]interface{}{
				"container_id": containerID,
				"runtime":      string(runtimeType),
				"error":        statusErr.Error(),
			},
		}, nil
	}

	// Si el contenedor no está running, no está healthy
	if !strings.Contains(strings.ToUpper(containerStatus), "RUNNING") {
		return map[string]interface{}{
			"healthy": false,
			"status":  "container_not_running",
			"message": fmt.Sprintf("Contenedor no está ejecutándose: %s", containerStatus),
			"details": map[string]interface{}{
				"container_id":     containerID,
				"container_status": containerStatus,
				"runtime":          string(runtimeType),
				"port":             app.Port,
			},
		}, nil
	}

	// Usar localhost para healthcheck
	healthcheckURL := fmt.Sprintf("http://localhost:%d", app.Port)

	// Hacer ping HTTP interno al contenedor
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Crear request con timeout
	req, err := http.NewRequestWithContext(context.Background(), "GET", healthcheckURL, nil)
	if err != nil {
		return map[string]interface{}{
			"healthy": false,
			"status":  "request_error",
			"message": fmt.Sprintf("Error creando request: %v", err),
			"details": map[string]interface{}{
				"url":     healthcheckURL,
				"runtime": string(runtimeType),
				"error":   err.Error(),
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
				"url":          healthcheckURL,
				"error":        err.Error(),
				"container_id": containerID,
				"runtime":      string(runtimeType),
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
			"url":              healthcheckURL,
			"http_status_code": resp.StatusCode,
			"container_id":     containerID,
			"container_status": containerStatus,
			"runtime":          string(runtimeType),
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
	// Validar que el valor no esté vacío
	if value == "" {
		return false
	}

	// Validar longitud máxima (4KB)
	if len(value) > 4096 {
		return false
	}

	// Validar que no contenga caracteres de control peligrosos
	for _, char := range value {
		if char < 32 && char != 9 && char != 10 && char != 13 { // Tab, LF, CR
			return false
		}
	}

	return true
}

// isRuntimeConnectivityError verifica si un error es debido a problemas de conectividad con el runtime
func isRuntimeConnectivityError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Errores comunes de conectividad con Docker
	dockerConnectivityErrors := []string{
		"Cannot connect to the Docker daemon",
		"docker daemon is not running",
		"connection refused",
		"no such host",
		"timeout",
		"network is unreachable",
		"permission denied",
		"operation not permitted",
		"docker daemon is not accessible",
		"failed to dial",
		"unable to connect",
	}

	// Errores comunes de conectividad con containerd
	containerdConnectivityErrors := []string{
		"connection refused",
		"no such host",
		"timeout",
		"network is unreachable",
		"permission denied",
		"operation not permitted",
		"containerd is not running",
		"failed to dial",
		"unable to connect",
		"socket not found",
		"ctr: command not found",
	}

	// Errores específicos de eliminación de contenedores (NO son errores de conectividad)
	containerRemovalErrors := []string{
		"exit status 1",
		"container not found",
		"task not found",
		"container is running",
		"container is not stopped",
		"container is not removed",
		"container is not deleted",
	}

	// Verificar errores específicos de eliminación de contenedores primero
	for _, removalErr := range containerRemovalErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(removalErr)) {
			return false // Estos NO son errores de conectividad
		}
	}

	// Verificar errores de Docker
	for _, dockerErr := range dockerConnectivityErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(dockerErr)) {
			return true
		}
	}

	// Verificar errores de containerd
	for _, containerdErr := range containerdConnectivityErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(containerdErr)) {
			return true
		}
	}

	return false
}

// cleanupOrphanedContainerdContainers limpia contenedores huérfanos de containerd
func cleanupOrphanedContainerdContainers() error {
	logrus.Info("🧹 Iniciando limpieza manual de contenedores huérfanos de containerd...")

	// Listar todos los contenedores en el namespace diplo
	listCmd := exec.Command("ctr", "-n", "diplo", "containers", "list")
	output, err := listCmd.Output()
	if err != nil {
		logrus.Warnf("Error listando contenedores containerd: %v", err)
		return fmt.Errorf("error listando contenedores: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	cleanedCount := 0

	// Estrategia 1: Detener todas las tareas con SIGKILL (como en container_prune.sh)
	logrus.Info("🔪 Deteniendo todas las tareas con SIGKILL...")
	killAllCmd := exec.Command("ctr", "-n", "diplo", "tasks", "ls")
	killAllOutput, killAllErr := killAllCmd.Output()
	if killAllErr == nil {
		killAllLines := strings.Split(string(killAllOutput), "\n")
		for _, line := range killAllLines {
			line = strings.TrimSpace(line)
			if line == "" || strings.Contains(line, "TASK") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) > 0 {
				taskName := fields[0]
				if strings.HasPrefix(taskName, "diplo-") {
					logrus.Debugf("Deteniendo tarea: %s", taskName)
					killCmd := exec.Command("ctr", "-n", "diplo", "tasks", "kill", "--signal", "SIGKILL", taskName)
					killCmd.Run() // Ignorar errores
				}
			}
		}
	}

	// Estrategia 2: Matar procesos containerd-shim relacionados
	logrus.Info("🔪 Matando procesos containerd-shim...")
	shimCmd := exec.Command("pkill", "-f", "containerd-shim")
	shimCmd.Run() // Ignorar errores

	// Esperar a que se detengan completamente
	logrus.Info("⏳ Esperando a que se detengan...")
	time.Sleep(5 * time.Second)

	// Estrategia 3: Verificar que las tareas se detuvieron
	logrus.Info("📋 Verificando tareas...")
	verifyCmd := exec.Command("ctr", "-n", "diplo", "tasks", "ls")
	verifyCmd.Run() // Solo para mostrar el estado

	// Estrategia 4: Eliminar contenedores uno por uno
	logrus.Info("🗑️  Eliminando contenedores...")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "NAME") {
			continue // Saltar encabezados y líneas vacías
		}

		// Extraer el nombre del contenedor (primer campo)
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		containerName := fields[0]
		if !strings.HasPrefix(containerName, "diplo-") {
			continue // Solo procesar contenedores de Diplo
		}

		logrus.Infof("Procesando contenedor huérfano: %s", containerName)

		// Intentar detener la tarea si está corriendo (con SIGKILL)
		killCmd := exec.Command("ctr", "-n", "diplo", "tasks", "kill", "--signal", "SIGKILL", containerName)
		if killErr := killCmd.Run(); killErr != nil {
			logrus.Debugf("Tarea %s ya estaba detenida o no existe: %v", containerName, killErr)
		}

		// Esperar un momento
		time.Sleep(1 * time.Second)

		// Eliminar la tarea
		taskDeleteCmd := exec.Command("ctr", "-n", "diplo", "tasks", "delete", containerName)
		if taskErr := taskDeleteCmd.Run(); taskErr != nil {
			logrus.Debugf("No se pudo eliminar tarea %s: %v", containerName, taskErr)
		}

		// Esperar un momento
		time.Sleep(1 * time.Second)

		// Eliminar el contenedor
		containerDeleteCmd := exec.Command("ctr", "-n", "diplo", "containers", "delete", containerName)
		if containerErr := containerDeleteCmd.Run(); containerErr != nil {
			logrus.Debugf("No se pudo eliminar contenedor %s: %v", containerName, containerErr)
		} else {
			logrus.Infof("✅ Contenedor huérfano eliminado: %s", containerName)
			cleanedCount++
		}
	}

	// Estrategia 5: Limpiar snapshots
	logrus.Info("🧽 Limpiando snapshots...")
	snapshotCmd := exec.Command("ctr", "-n", "diplo", "snapshots", "prune")
	snapshotCmd.Run() // Ignorar errores

	// Estrategia 6: Verificar resultado final
	logrus.Info("📋 Estado final:")

	// Verificar tareas
	tasksCmd := exec.Command("ctr", "-n", "diplo", "tasks", "ls")
	if tasksOutput, tasksErr := tasksCmd.Output(); tasksErr == nil {
		logrus.Info("   Tareas:")
		logrus.Info(string(tasksOutput))
	} else {
		logrus.Info("   No hay tareas")
	}

	// Verificar contenedores
	containersCmd := exec.Command("ctr", "-n", "diplo", "containers", "ls")
	if containersOutput, containersErr := containersCmd.Output(); containersErr == nil {
		logrus.Info("   Contenedores:")
		logrus.Info(string(containersOutput))
	} else {
		logrus.Info("   No hay contenedores")
	}

	if cleanedCount > 0 {
		logrus.Infof("🧹 Limpieza completada: %d contenedores huérfanos eliminados", cleanedCount)
	} else {
		logrus.Info("🧹 No se encontraron contenedores huérfanos para limpiar")
	}

	return nil
}

// CleanupOrphanedContainersHandler limpia contenedores huérfanos de containerd
func CleanupOrphanedContainersHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	logrus.Info("🧹 Iniciando limpieza manual de contenedores huérfanos...")

	if err := cleanupOrphanedContainerdContainers(); err != nil {
		logrus.Errorf("Error durante limpieza de contenedores huérfanos: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error durante limpieza de contenedores huérfanos"}, err
	}

	response := map[string]interface{}{
		"message": "Limpieza de contenedores huérfanos completada",
		"status":  "success",
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

// AggressiveCleanupContainersHandler ejecuta la limpieza agresiva usando el script container_prune.sh
func AggressiveCleanupContainersHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	logrus.Info("🚨 Iniciando limpieza agresiva de contenedores...")

	// Ejecutar el script de limpieza agresiva
	scriptPath := "./scripts/container_prune.sh"
	cmd := exec.Command("sudo", scriptPath)

	// Capturar output del script
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("Error ejecutando limpieza agresiva: %v", err)
		logrus.Errorf("Output del script: %s", string(output))
		return Response{Code: http.StatusInternalServerError, Message: "Error durante limpieza agresiva"}, err
	}

	logrus.Infof("Limpieza agresiva completada. Output: %s", string(output))

	response := map[string]interface{}{
		"message": "Limpieza agresiva de contenedores completada",
		"status":  "success",
		"output":  string(output),
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}
