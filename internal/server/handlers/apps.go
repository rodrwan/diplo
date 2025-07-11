package handlers

import (
	"context"
	"fmt"
	"net/http"
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
