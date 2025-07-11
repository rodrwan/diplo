package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/dto"
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

	// Detener y eliminar contenedor si existe (StopContainer ya incluye remove)
	if app.ContainerID.String != "" {
		if err := ctx.docker.StopContainer(app.ContainerID.String); err != nil {
			logrus.Warnf("Error deteniendo contenedor %s: %v", app.ContainerID.String, err)
		}
	}

	// Eliminar aplicación de la base de datos
	if err := ctx.queries.DeleteApp(r.Context(), appID); err != nil {
		logrus.Errorf("Error eliminando aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error eliminando aplicación"}, err
	}

	return Response{Code: http.StatusOK, Message: "Aplicación eliminada exitosamente"}, nil
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
