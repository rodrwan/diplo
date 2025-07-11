package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/models"
	"github.com/rodrwan/diplo/internal/runtime"
	"github.com/sirupsen/logrus"
)

// LXCContext contiene los componentes necesarios para el sistema LXC
type LXCContext struct {
	lxcClient   runtime.ContainerRuntime
	queries     database.Querier
	logChannels map[string]chan string
	logMu       sync.RWMutex
}

// NewLXCContext crea un nuevo contexto LXC
func NewLXCContext(queries database.Querier, logChannels map[string]chan string) (*LXCContext, error) {
	lxcClient, err := runtime.NewLXCClient()
	if err != nil {
		return nil, fmt.Errorf("error creando LXC client: %w", err)
	}

	return &LXCContext{
		lxcClient:   lxcClient,
		queries:     queries,
		logChannels: logChannels,
	}, nil
}

// LXCDeployHandler maneja deployments usando LXC
func LXCDeployHandler(ctx *LXCContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	var req models.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return Response{Code: http.StatusBadRequest, Message: "Invalid JSON"}, err
	}

	if req.RepoURL == "" {
		return Response{Code: http.StatusBadRequest, Message: "repo_url is required"}, errors.New("repo_url is required")
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

		// Iniciar redeploy en background
		go redeployExistingAppLXC(ctx, &existingApp)

		// Responder inmediatamente
		response := map[string]any{
			"id":       existingApp.ID,
			"name":     existingApp.Name,
			"repo_url": existingApp.RepoUrl,
			"port":     existingApp.Port,
			"url":      fmt.Sprintf("http://localhost:%d", existingApp.Port),
			"status":   "redeploying",
			"message":  "Redeploy iniciado para aplicación existente",
			"platform": "lxc",
		}

		return Response{Code: http.StatusOK, Data: response}, nil
	}

	// Si no existe, crear nueva aplicación
	app := &database.App{
		ID:      database.GenerateAppID(),
		Name:    req.Name,
		RepoUrl: req.RepoURL,
	}

	// Asignar puerto libre (será asignado dinámicamente)
	port := 0

	// Guardar en base de datos
	if err := ctx.queries.CreateApp(r.Context(), database.CreateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  req.RepoURL,
		Language: sql.NullString{String: "Unknown", Valid: true}, // Se detectará automáticamente
		Port:     int64(port),
		Status:   database.StatusDeploying,
	}); err != nil {
		logrus.Errorf("Error guardando aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error guardando aplicación"}, err
	}

	// Iniciar deployment usando LXC
	go deployAppLXC(ctx, app)

	// Responder inmediatamente
	response := map[string]any{
		"id":       app.ID,
		"name":     app.Name,
		"repo_url": app.RepoUrl,
		"port":     "pending", // Se asignará dinámicamente
		"url":      "pending", // Se generará cuando se asigne el puerto
		"status":   "deploying",
		"message":  "Aplicación creada y deployment iniciado con LXC",
		"platform": "lxc",
	}

	return Response{Code: http.StatusCreated, Data: response}, nil
}

// deployAppLXC despliega una aplicación usando LXC
func deployAppLXC(ctx *LXCContext, app *database.App) {
	logrus.Infof("Iniciando deployment LXC de: %s (%s)", app.Name, app.ID)

	// Enviar log inicial
	sendLogMessageLXC(ctx, app.ID, "info", "Iniciando deployment con LXC...")

	// Actualizar estado
	app.Status = database.StatusDeploying
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: sql.NullString{String: "Unknown", Valid: true},
		Port:     app.Port,
		Status:   app.Status,
	})

	// Crear request para el LXC runtime
	createReq := &runtime.CreateContainerRequest{
		Name:        app.Name,
		Image:       "ubuntu:focal", // Imagen base para LXC
		Command:     []string{"/bin/bash", "-c", "echo 'Container created'"},
		Environment: make(map[string]string),
		Labels:      make(map[string]string),
		Ports:       []runtime.PortMapping{},
		Volumes:     []runtime.VolumeMount{},
	}

	// Crear container LXC
	container, err := ctx.lxcClient.CreateContainer(createReq)
	if err != nil {
		logrus.Errorf("Error creando container LXC: %v", err)
		handleLXCDeployError(ctx, app, fmt.Sprintf("Error creando container: %v", err))
		return
	}

	sendLogMessageLXC(ctx, app.ID, "info", fmt.Sprintf("Container LXC creado: %s", container.ID))

	// Iniciar container
	if err := ctx.lxcClient.StartContainer(context.Background(), container.ID); err != nil {
		logrus.Errorf("Error iniciando container LXC: %v", err)
		handleLXCDeployError(ctx, app, fmt.Sprintf("Error iniciando container: %v", err))
		return
	}

	sendLogMessageLXC(ctx, app.ID, "info", "Container LXC iniciado exitosamente")

	// Simular deployment exitoso (en una implementación real, aquí se haría el deployment del código)
	time.Sleep(5 * time.Second)

	// Actualizar base de datos con deployment exitoso
	app.Status = database.StatusRunning
	app.Port = 8080 // Puerto por defecto
	app.Language = sql.NullString{String: "Unknown", Valid: true}

	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: app.Language,
		Port:     app.Port,
		Status:   app.Status,
	})

	sendLogMessageLXC(ctx, app.ID, "info", "Deployment completado exitosamente")
	logrus.Infof("Deployment LXC completado para %s (%s)", app.Name, app.ID)
}

// redeployExistingAppLXC redespliega una aplicación existente
func redeployExistingAppLXC(ctx *LXCContext, app *database.App) {
	logrus.Infof("Iniciando redeploy LXC de: %s (%s)", app.Name, app.ID)

	// Enviar log inicial
	sendLogMessageLXC(ctx, app.ID, "info", "Iniciando redeploy con LXC...")

	// Actualizar estado
	app.Status = database.StatusDeploying
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: app.Language,
		Port:     app.Port,
		Status:   app.Status,
	})

	// Obtener container existente
	container, err := ctx.lxcClient.GetContainer(app.ID)
	if err != nil {
		logrus.Errorf("Error obteniendo container existente: %v", err)
		handleLXCDeployError(ctx, app, fmt.Sprintf("Error obteniendo container: %v", err))
		return
	}

	// Reiniciar container
	if err := ctx.lxcClient.RestartContainer(context.Background(), container.ID); err != nil {
		logrus.Errorf("Error reiniciando container LXC: %v", err)
		handleLXCDeployError(ctx, app, fmt.Sprintf("Error reiniciando container: %v", err))
		return
	}

	sendLogMessageLXC(ctx, app.ID, "info", "Container LXC reiniciado exitosamente")

	// Simular redeploy exitoso
	time.Sleep(3 * time.Second)

	// Actualizar estado
	app.Status = database.StatusRunning
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: app.Language,
		Port:     app.Port,
		Status:   app.Status,
	})

	sendLogMessageLXC(ctx, app.ID, "info", "Redeploy completado exitosamente")
	logrus.Infof("Redeploy LXC completado para %s (%s)", app.Name, app.ID)
}

// handleLXCDeployError maneja errores en el deployment
func handleLXCDeployError(ctx *LXCContext, app *database.App, errorMsg string) {
	logrus.Errorf("Error en deployment LXC de %s: %s", app.Name, errorMsg)

	// Enviar log de error
	sendLogMessageLXC(ctx, app.ID, "error", errorMsg)

	// Actualizar estado en base de datos
	app.Status = database.StatusError
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: app.Language,
		Port:     app.Port,
		Status:   app.Status,
	})
}

// sendLogMessageLXC envía un mensaje de log a través del canal SSE
func sendLogMessageLXC(ctx *LXCContext, appID, logType, message string) {
	ctx.logMu.RLock()
	defer ctx.logMu.RUnlock()

	if ch, ok := ctx.logChannels[appID]; ok {
		select {
		case ch <- fmt.Sprintf("[%s] %s: %s", time.Now().Format("15:04:05"), logType, message):
		default:
			// Canal bloqueado, no hacer nada
		}
	}
}

// LXCStatusHandler maneja el endpoint de estado LXC
func LXCStatusHandler(ctx *LXCContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		// Devolver estado general del sistema LXC
		info, err := ctx.lxcClient.GetRuntimeInfo()
		if err != nil {
			return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo información LXC"}, err
		}

		response := map[string]any{
			"runtime_type": "lxc",
			"available":    true,
			"info":         info,
			"timestamp":    time.Now(),
		}

		return Response{Code: http.StatusOK, Data: response}, nil
	}

	// Obtener estado de aplicación específica
	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		if err == sql.ErrNoRows {
			return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, nil
		}
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	// Obtener estado del container
	container, err := ctx.lxcClient.GetContainer(app.ID)
	if err != nil {
		// Si no se encuentra el container, devolver estado de la app desde la DB
		response := map[string]any{
			"id":                app.ID,
			"name":              app.Name,
			"repo_url":          app.RepoUrl,
			"language":          app.Language.String,
			"port":              app.Port,
			"url":               fmt.Sprintf("http://localhost:%d", app.Port),
			"status":            app.Status.String,
			"deployment_status": app.Status.String,
			"container_status":  "not_found",
			"created_at":        app.CreatedAt,
			"platform":          "lxc",
		}

		return Response{Code: http.StatusOK, Data: response}, nil
	}

	// Devolver estado completo
	response := map[string]any{
		"id":                app.ID,
		"name":              app.Name,
		"repo_url":          app.RepoUrl,
		"language":          app.Language.String,
		"port":              app.Port,
		"url":               fmt.Sprintf("http://localhost:%d", app.Port),
		"status":            app.Status.String,
		"deployment_status": app.Status.String,
		"container_status":  string(container.Status),
		"container_id":      container.ID,
		"created_at":        app.CreatedAt,
		"platform":          "lxc",
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

// LXCStopHandler maneja el endpoint para detener aplicaciones LXC
func LXCStopHandler(ctx *LXCContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		return Response{Code: http.StatusBadRequest, Message: "app_id is required"}, nil
	}

	// Obtener aplicación
	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		if err == sql.ErrNoRows {
			return Response{Code: http.StatusNotFound, Message: "Aplicación no encontrada"}, nil
		}
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	// Detener container
	if err := ctx.lxcClient.StopContainer(context.Background(), app.ID); err != nil {
		logrus.Errorf("Error deteniendo container LXC %s: %v", app.ID, err)
		return Response{Code: http.StatusInternalServerError, Message: "Error deteniendo container"}, err
	}

	// Actualizar estado en base de datos
	app.Status = database.StatusIdle
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: app.Language,
		Port:     app.Port,
		Status:   app.Status,
	})

	sendLogMessageLXC(ctx, app.ID, "info", "Aplicación detenida exitosamente")

	response := map[string]any{
		"id":      app.ID,
		"name":    app.Name,
		"status":  app.Status.String,
		"message": "Aplicación detenida exitosamente",
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

// LXCListHandler maneja el endpoint para listar aplicaciones LXC
func LXCListHandler(ctx *LXCContext, w http.ResponseWriter, r *http.Request) (Response, error) {
	// Obtener todas las aplicaciones de la base de datos
	apps, err := ctx.queries.GetAllApps(r.Context())
	if err != nil {
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicaciones"}, err
	}

	// Obtener containers LXC
	containers, err := ctx.lxcClient.ListContainers(context.Background())
	if err != nil {
		logrus.Warnf("Error obteniendo containers LXC: %v", err)
		containers = []*runtime.Container{}
	}

	// Mapear containers por ID
	containerMap := make(map[string]*runtime.Container)
	for _, container := range containers {
		containerMap[container.ID] = container
	}

	// Construir respuesta
	var response []map[string]any
	for _, app := range apps {
		appData := map[string]any{
			"id":         app.ID,
			"name":       app.Name,
			"repo_url":   app.RepoUrl,
			"language":   app.Language.String,
			"port":       app.Port,
			"url":        fmt.Sprintf("http://localhost:%d", app.Port),
			"status":     app.Status.String,
			"created_at": app.CreatedAt,
			"platform":   "lxc",
		}

		// Agregar información del container si existe
		if container, exists := containerMap[app.ID]; exists {
			appData["container_status"] = string(container.Status)
			appData["container_id"] = container.ID
		} else {
			appData["container_status"] = "not_found"
		}

		response = append(response, appData)
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

// Cleanup limpia los recursos del contexto LXC
func (ctx *LXCContext) Cleanup() error {
	if ctx.lxcClient != nil {
		return ctx.lxcClient.Close()
	}
	return nil
}
