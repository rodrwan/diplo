package server

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/docker"
	"github.com/rodrwan/diplo/internal/dto"
	"github.com/rodrwan/diplo/internal/models"
	"github.com/rodrwan/diplo/internal/templates"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

type Server struct {
	router  *mux.Router
	server  *http.Server
	docker  *docker.Client
	mu      sync.RWMutex
	db      *sql.DB
	queries database.Querier
	// Para SSE - canales de logs por app
	logChannels map[string]chan string
	logMu       sync.RWMutex
}

func NewServer(host string, port int) *Server {
	router := mux.NewRouter()

	srv := &Server{
		router: router,
		server: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			Handler:      router,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		logChannels: make(map[string]chan string),
	}

	// Inicializar base de datos
	db, err := sql.Open("sqlite3", "diplo.db")
	if err != nil {
		logrus.Fatalf("error abriendo base de datos: %v", err)
	}

	queries := database.New(db)
	if err := queries.CreateTables(context.Background()); err != nil {
		logrus.Fatalf("error creando tablas: %v", err)
	}
	srv.db = db
	srv.queries = queries

	// Inicializar cliente Docker
	dockerClient, err := docker.NewClient()
	if err != nil {
		logrus.Fatalf("Error inicializando cliente Docker: %v", err)
	}
	srv.docker = dockerClient

	// Configurar callback para eventos Docker
	srv.docker.SetEventCallback(func(event docker.DockerEvent) {
		// Solo registrar eventos globales en logs, no enviar a todas las apps
		logrus.Debugf("Evento Docker global: %s - %s", event.Type, event.Message)
	})

	// Configurar rutas
	srv.setupRoutes()

	return srv
}

func (s *Server) setupRoutes() {
	// Middleware CORS
	s.router.Use(s.corsMiddleware)

	// Frontend
	s.router.HandleFunc("/", s.frontendHandler).Methods("GET")
	// Health check
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")
	// Página de prueba de eventos Docker
	s.router.HandleFunc("/docker-events", s.dockerEventsHandler).Methods("GET")
	// Gestor de aplicaciones
	s.router.HandleFunc("/apps", s.appsManagerHandler).Methods("GET")

	// API routes
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/deploy", s.deployHandler).Methods("POST")
	api.HandleFunc("/apps", s.listAppsHandler).Methods("GET")
	api.HandleFunc("/apps/{id}", s.getAppHandler).Methods("GET")
	api.HandleFunc("/apps/{id}", s.deleteAppHandler).Methods("DELETE")

	// Maintenance endpoints
	api.HandleFunc("/maintenance/prune-images", s.pruneImagesHandler).Methods("POST")

	// SSE endpoint para logs en tiempo real
	api.HandleFunc("/apps/{id}/logs", s.logsSSEHandler).Methods("GET")
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) frontendHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.Layout()
	components.Render(r.Context(), w)
}

func (s *Server) dockerEventsHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.DockerEvents()
	components.Render(r.Context(), w)
}

func (s *Server) appsManagerHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.AppsManager()
	components.Render(r.Context(), w)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":  "ok",
		"message": "Diplo server running",
		"version": "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) deployHandler(w http.ResponseWriter, r *http.Request) {
	var req models.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.RepoURL == "" {
		http.Error(w, "repo_url is required", http.StatusBadRequest)
		return
	}

	// Verificar si ya existe una aplicación con este repo_url
	existingApp, err := s.queries.GetAppByRepoUrl(r.Context(), req.RepoURL)
	if err != nil && err != sql.ErrNoRows {
		logrus.Errorf("Error verificando aplicación existente: %v", err)
		http.Error(w, "Error verificando aplicación existente", http.StatusInternalServerError)
		return
	}

	// Si existe una app con el mismo repo_url, hacer redeploy
	if err != sql.ErrNoRows {
		logrus.Infof("App existente encontrada para %s, haciendo redeploy: %s", req.RepoURL, existingApp.ID)

		// Actualizar nombre si se proporcionó uno nuevo
		if req.Name != "" && req.Name != existingApp.Name {
			existingApp.Name = req.Name
		}

		// Iniciar redeploy en background
		go s.redeployExistingApp(s.queries, &existingApp)

		// Responder inmediatamente
		response := map[string]any{
			"id":       existingApp.ID,
			"name":     existingApp.Name,
			"repo_url": existingApp.RepoUrl,
			"port":     existingApp.Port,
			"url":      fmt.Sprintf("http://localhost:%d", existingApp.Port),
			"status":   "redeploying",
			"message":  "Redeploy iniciado para aplicación existente",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
		return
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
		http.Error(w, "No se pudo asignar puerto libre", http.StatusInternalServerError)
		return
	}
	app.Port = int64(port)

	// Guardar en base de datos
	if err := s.queries.CreateApp(r.Context(), database.CreateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  req.RepoURL,
		Language: sql.NullString{String: "Go", Valid: true},
		Port:     int64(port),
		Status:   database.StatusDeploying,
	}); err != nil {
		logrus.Errorf("Error guardando aplicación: %v", err)
		http.Error(w, "Error guardando aplicación", http.StatusInternalServerError)
		return
	}

	// Iniciar deployment en background
	go s.deployApp(s.queries, app)

	// Responder inmediatamente
	response := map[string]any{
		"id":       app.ID,
		"name":     app.Name,
		"repo_url": app.RepoUrl,
		"port":     app.Port,
		"url":      fmt.Sprintf("http://localhost:%d", app.Port),
		"status":   "deploying",
		"message":  "Aplicación creada y deployment iniciado",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) listAppsHandler(w http.ResponseWriter, r *http.Request) {
	apps, err := s.queries.GetAllApps(r.Context())
	if err != nil {
		logrus.Errorf("Error obteniendo aplicaciones: %v", err)
		http.Error(w, "Error obteniendo aplicaciones", http.StatusInternalServerError)
		return
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appsDTO)
}

func (s *Server) getAppHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := s.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		http.Error(w, "Error obteniendo aplicación", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app)
}

func (s *Server) deleteAppHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := s.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		http.Error(w, "Error obteniendo aplicación", http.StatusInternalServerError)
		return
	}

	// Detener y eliminar contenedor si existe
	if app.ContainerID.String != "" {
		if err := s.docker.StopContainer(app.ContainerID.String); err != nil {
			logrus.Warnf("Error deteniendo contenedor %s: %v", app.ContainerID.String, err)
		}
	}

	// Eliminar de base de datos
	if err := s.queries.DeleteApp(r.Context(), appID); err != nil {
		logrus.Errorf("Error eliminando aplicación: %v", err)
	}

	response := map[string]interface{}{
		"message": "Aplicación eliminada exitosamente",
		"id":      appID,
	}

	// Limpiar imágenes dangling después de eliminar la app
	go func() {
		if err := s.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling después de eliminar app: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) pruneImagesHandler(w http.ResponseWriter, r *http.Request) {
	logrus.Info("Manual prune of dangling images requested")

	// Ejecutar limpieza de imágenes dangling
	err := s.docker.PruneDanglingImages()
	if err != nil {
		logrus.Errorf("Error durante limpieza manual de imágenes: %v", err)
		response := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Error limpiando imágenes dangling: %v", err),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Imágenes dangling limpiadas exitosamente",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) deployApp(queries database.Querier, app *database.App) {
	logrus.Infof("Iniciando deployment de: %s (%s)", app.Name, app.ID)

	// Configurar callback específico para esta aplicación
	originalCallback := s.docker.GetEventCallback()
	s.docker.SetEventCallback(func(event docker.DockerEvent) {
		// Enviar evento específico para esta aplicación
		s.sendDockerEventToApp(app.ID, event)
	})
	defer s.docker.SetEventCallback(originalCallback)

	// Enviar log inicial
	s.sendLogMessage(app.ID, "info", "Iniciando deployment...")

	// Actualizar estado
	app.Status = database.StatusDeploying
	queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: sql.NullString{String: "Go", Valid: true},
		Port:     app.Port,
		Status:   app.Status,
	})

	// Detectar lenguaje
	s.sendLogMessage(app.ID, "info", "Detectando lenguaje...")
	language, err := detectLanguage(app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error detectando lenguaje: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error detectando lenguaje: %v", err), Valid: true}
		queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: sql.NullString{String: "Go", Valid: true},
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		s.sendLogMessage(app.ID, "error", fmt.Sprintf("Error detectando lenguaje: %v", err))
		return
	}
	app.Language = sql.NullString{String: language, Valid: true}
	logrus.Infof("Lenguaje detectado: %s", language)
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Lenguaje detectado: %s", language))

	// Generar Dockerfile
	s.sendLogMessage(app.ID, "info", "Generando Dockerfile...")
	dockerfile, err := generateDockerfile(app.RepoUrl, strconv.Itoa(int(app.Port)), language)
	if err != nil {
		logrus.Errorf("Error generando Dockerfile: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error generando Dockerfile: %v", err), Valid: true}
		queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		s.sendLogMessage(app.ID, "error", fmt.Sprintf("Error generando Dockerfile: %v", err))
		return
	}

	logrus.Debugf("Dockerfile generado:\n%s", dockerfile)
	s.sendLogMessage(app.ID, "info", "Dockerfile generado exitosamente")

	// Generar tag único basado en el hash del commit
	s.sendLogMessage(app.ID, "info", "Obteniendo hash del último commit...")
	imageTag, err := s.docker.GenerateImageTag(app.ID, app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error generando tag de imagen: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error generando tag de imagen: %v", err), Valid: true}
		queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		s.sendLogMessage(app.ID, "error", fmt.Sprintf("Error generando tag de imagen: %v", err))
		return
	}

	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Tag de imagen generado: %s", imageTag))

	// Construir imagen
	logrus.Infof("Construyendo imagen: %s", imageTag)
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Construyendo imagen Docker: %s", imageTag))

	imageID, err := s.docker.BuildImage(imageTag, dockerfile)
	if err != nil {
		logrus.Errorf("Error construyendo imagen: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error construyendo imagen Docker: %v", err), Valid: true}
		queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		s.sendLogMessage(app.ID, "error", fmt.Sprintf("Error construyendo imagen Docker: %v", err))

		// Limpiar imágenes dangling después de build fallido
		go func() {
			if err := s.docker.PruneDanglingImages(); err != nil {
				logrus.Warnf("Error limpiando imágenes dangling después de build fallido: %v", err)
			}
		}()

		return
	}

	s.sendLogMessage(app.ID, "success", "Imagen construida exitosamente")

	// Ejecutar contenedor
	logrus.Infof("Ejecutando contenedor en puerto %d", app.Port)
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Ejecutando contenedor en puerto %d", app.Port))
	containerID, err := s.docker.RunContainer(app, imageTag)
	if err != nil {
		logrus.Errorf("Error ejecutando contenedor: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error ejecutando contenedor: %v", err), Valid: true}
		queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		s.sendLogMessage(app.ID, "error", fmt.Sprintf("Error ejecutando contenedor: %v", err))
		return
	}

	// Actualizar aplicación
	app.Status = database.StatusRunning
	app.ContainerID = sql.NullString{String: containerID, Valid: true}
	app.ImageID = sql.NullString{String: imageID, Valid: true}
	app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	app.ErrorMsg = sql.NullString{String: "", Valid: true}

	if err := queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación: %v", err)
	}

	// Limpiar imágenes antiguas (mantener solo las 3 más recientes)
	go func() {
		if err := s.docker.CleanupOldImages(app.ID, 3); err != nil {
			logrus.Warnf("Error limpiando imágenes antiguas: %v", err)
		}

		// Limpiar imágenes dangling después del cleanup
		if err := s.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling: %v", err)
		}
	}()

	logrus.Infof("Deployment completado exitosamente: %s en puerto %d", app.ID, app.Port)
	s.sendLogMessage(app.ID, "success", fmt.Sprintf("Deployment completado exitosamente en puerto %d", app.Port))
	s.sendLogMessage(app.ID, "success", fmt.Sprintf("Aplicación disponible en: http://localhost:%d", app.Port))
}

func (s *Server) redeployExistingApp(queries database.Querier, app *database.App) {
	logrus.Infof("Iniciando redeploy de aplicación existente: %s (%s)", app.Name, app.ID)

	// Configurar callback específico para esta aplicación
	originalCallback := s.docker.GetEventCallback()
	s.docker.SetEventCallback(func(event docker.DockerEvent) {
		s.sendDockerEventToApp(app.ID, event)
	})
	defer s.docker.SetEventCallback(originalCallback)

	// Enviar log inicial
	s.sendLogMessage(app.ID, "info", "Iniciando redeploy de aplicación existente...")

	// Actualizar estado a redeploying
	app.Status = database.StatusRedeploying
	app.ErrorMsg = sql.NullString{String: "", Valid: true}
	if err := queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando estado de redeploy: %v", err)
	}

	// Parar contenedor anterior si existe
	if app.ContainerID.String != "" {
		s.sendLogMessage(app.ID, "info", "Deteniendo contenedor anterior...")
		if err := s.docker.StopContainer(app.ContainerID.String); err != nil {
			logrus.Warnf("Error deteniendo contenedor anterior %s: %v", app.ContainerID.String, err)
			s.sendLogMessage(app.ID, "warning", fmt.Sprintf("Error deteniendo contenedor anterior: %v", err))
		} else {
			s.sendLogMessage(app.ID, "info", "Contenedor anterior detenido exitosamente")
		}
		// Limpiar container ID
		app.ContainerID = sql.NullString{String: "", Valid: true}
	}

	// Detectar lenguaje
	s.sendLogMessage(app.ID, "info", "Detectando lenguaje...")
	language, err := detectLanguage(app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error detectando lenguaje en redeploy: %v", err)
		s.handleRedeployError(queries, app, fmt.Sprintf("Error detectando lenguaje: %v", err))
		return
	}
	app.Language = sql.NullString{String: language, Valid: true}
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Lenguaje detectado: %s", language))

	// Generar Dockerfile
	s.sendLogMessage(app.ID, "info", "Generando Dockerfile...")
	dockerfile, err := generateDockerfile(app.RepoUrl, strconv.Itoa(int(app.Port)), language)
	if err != nil {
		logrus.Errorf("Error generando Dockerfile en redeploy: %v", err)
		s.handleRedeployError(queries, app, fmt.Sprintf("Error generando Dockerfile: %v", err))
		return
	}
	s.sendLogMessage(app.ID, "info", "Dockerfile generado exitosamente")

	// Generar nuevo tag único basado en el hash del commit actual
	s.sendLogMessage(app.ID, "info", "Obteniendo hash del último commit...")
	imageTag, err := s.docker.GenerateImageTag(app.ID, app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error generando tag de imagen en redeploy: %v", err)
		s.handleRedeployError(queries, app, fmt.Sprintf("Error generando tag de imagen: %v", err))
		return
	}
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Nuevo tag de imagen generado: %s", imageTag))

	// Construir nueva imagen
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Construyendo nueva imagen: %s", imageTag))
	imageID, err := s.docker.BuildImage(imageTag, dockerfile)
	if err != nil {
		logrus.Errorf("Error construyendo imagen en redeploy: %v", err)
		s.handleRedeployError(queries, app, fmt.Sprintf("Error construyendo imagen Docker: %v", err))

		// Limpiar imágenes dangling después de build fallido
		go func() {
			if err := s.docker.PruneDanglingImages(); err != nil {
				logrus.Warnf("Error limpiando imágenes dangling después de build fallido: %v", err)
			}
		}()
		return
	}
	s.sendLogMessage(app.ID, "success", "Nueva imagen construida exitosamente")

	// Ejecutar nuevo contenedor
	s.sendLogMessage(app.ID, "info", fmt.Sprintf("Ejecutando nuevo contenedor en puerto %d", app.Port))
	containerID, err := s.docker.RunContainer(app, imageTag)
	if err != nil {
		logrus.Errorf("Error ejecutando contenedor en redeploy: %v", err)
		s.handleRedeployError(queries, app, fmt.Sprintf("Error ejecutando contenedor: %v", err))
		return
	}

	// Actualizar aplicación con nueva información
	app.Status = database.StatusRunning
	app.ContainerID = sql.NullString{String: containerID, Valid: true}
	app.ImageID = sql.NullString{String: imageID, Valid: true}
	app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	app.ErrorMsg = sql.NullString{String: "", Valid: true}

	if err := queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación después del redeploy: %v", err)
	}

	// Limpiar imágenes antiguas (mantener solo las 3 más recientes)
	go func() {
		if err := s.docker.CleanupOldImages(app.ID, 3); err != nil {
			logrus.Warnf("Error limpiando imágenes antiguas después del redeploy: %v", err)
		}

		// Limpiar imágenes dangling después del cleanup
		if err := s.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling después del redeploy: %v", err)
		}
	}()

	logrus.Infof("Redeploy completado exitosamente: %s en puerto %d", app.ID, app.Port)
	s.sendLogMessage(app.ID, "success", fmt.Sprintf("Redeploy completado exitosamente en puerto %d", app.Port))
	s.sendLogMessage(app.ID, "success", fmt.Sprintf("Aplicación actualizada disponible en: http://localhost:%d", app.Port))
}

// handleRedeployError maneja errores durante el redeploy
func (s *Server) handleRedeployError(queries database.Querier, app *database.App, errorMsg string) {
	app.Status = database.StatusError
	app.ErrorMsg = sql.NullString{String: errorMsg, Valid: true}
	if err := queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación con error de redeploy: %v", err)
	}
	s.sendLogMessage(app.ID, "error", errorMsg)
}

// logsSSEHandler maneja las conexiones SSE para logs en tiempo real
func (s *Server) logsSSEHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := s.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		http.Error(w, "Error obteniendo aplicación", http.StatusInternalServerError)
		return
	}

	// Configurar headers para SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Crear canal para logs si no existe
	s.logMu.Lock()
	if _, exists := s.logChannels[appID]; !exists {
		s.logChannels[appID] = make(chan string, 100) // Buffer de 100 mensajes
	}
	logChan := s.logChannels[appID]
	s.logMu.Unlock()

	// Enviar mensaje inicial
	fmt.Fprintf(w, "data: %s\n\n", `{"type": "connected", "message": "Conexión SSE establecida"}`)
	w.(http.Flusher).Flush()

	// Escuchar logs del contenedor si está ejecutándose
	if app.ContainerID.String != "" {
		go s.streamContainerLogs(app.ContainerID.String, logChan)
	}

	// Escuchar canal de logs
	for {
		select {
		case logMsg := <-logChan:
			fmt.Fprintf(w, "data: %s\n\n", logMsg)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			// Cliente desconectado
			s.logMu.Lock()
			delete(s.logChannels, appID)
			s.logMu.Unlock()
			return
		}
	}
}

// streamContainerLogs obtiene logs del contenedor en tiempo real
func (s *Server) streamContainerLogs(containerID string, logChan chan<- string) {
	logs, err := s.docker.GetContainerLogsStream(containerID)
	if err != nil {
		logMsg := createLogMessage("error", fmt.Sprintf("Error obteniendo logs: %v", err))
		logChan <- logMsg
		return
	}
	defer logs.Close()

	// Leer logs línea por línea
	scanner := bufio.NewScanner(logs)
	for scanner.Scan() {
		line := scanner.Text()
		// Limpiar la línea de caracteres de control
		cleanLine := sanitizeString(line)
		if cleanLine != "" {
			logMsg := createLogMessage("log", cleanLine)
			logChan <- logMsg
		}
	}

	if err := scanner.Err(); err != nil {
		logMsg := createLogMessage("error", fmt.Sprintf("Error leyendo logs: %v", err))
		logChan <- logMsg
	}
}

// sendLogMessage envía un mensaje de log a todos los clientes conectados
func (s *Server) sendLogMessage(appID, logType, message string) {
	s.logMu.RLock()
	if logChan, exists := s.logChannels[appID]; exists {
		logMsg := createLogMessage(logType, message)
		select {
		case logChan <- logMsg:
		default:
			// Canal lleno, ignorar mensaje
		}
	}
	s.logMu.RUnlock()
}

// sendDockerEventToApp envía un evento Docker específico a una aplicación
func (s *Server) sendDockerEventToApp(appID string, event docker.DockerEvent) {
	// Sanitizar el mensaje del evento
	sanitizedMessage := sanitizeString(event.Message)

	// Sanitizar datos del evento si existen
	var sanitizedData map[string]interface{}
	if event.Data != nil {
		sanitizedData = make(map[string]interface{})
		for k, v := range event.Data {
			if str, ok := v.(string); ok {
				sanitizedData[k] = sanitizeString(str)
			} else {
				sanitizedData[k] = v
			}
		}
	}

	// Convertir evento Docker a formato JSON para SSE
	eventJSON, err := json.Marshal(map[string]interface{}{
		"type":    "docker_event",
		"event":   event.Type,
		"message": sanitizedMessage,
		"data":    sanitizedData,
		"time":    event.Time.Format(time.RFC3339),
	})
	if err != nil {
		logrus.Errorf("Error serializando evento Docker para app %s: %v", appID, err)
		return
	}

	// Enviar evento al canal específico de la aplicación
	s.logMu.RLock()
	if logChan, exists := s.logChannels[appID]; exists {
		select {
		case logChan <- string(eventJSON):
			logrus.Debugf("Evento Docker enviado a app %s: %s", appID, event.Type)
		default:
			// Canal lleno, ignorar evento
			logrus.Debugf("Canal de logs lleno para app %s, ignorando evento", appID)
		}
	}
	s.logMu.RUnlock()
}

func (s *Server) Start() error {
	logrus.Infof("Servidor Diplo iniciado en %s", s.server.Addr)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.docker.Close(); err != nil {
		logrus.Errorf("Error cerrando conexión a Docker: %v", err)
	}

	if err := s.db.Close(); err != nil {
		logrus.Errorf("Error cerrando base de datos: %v", err)
	}

	return s.server.Shutdown(ctx)
}

// sanitizeString limpia una cadena de caracteres de control y caracteres especiales
func sanitizeString(s string) string {
	// Reemplazar caracteres de control comunes
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\b", "\\b")
	s = strings.ReplaceAll(s, "\f", "\\f")

	// Escapar comillas dobles
	s = strings.ReplaceAll(s, `"`, `\"`)

	// Remover caracteres de control no imprimibles
	var result strings.Builder
	for _, r := range s {
		if r >= 32 || r == '\n' || r == '\r' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// createLogMessage crea un mensaje de log JSON válido
func createLogMessage(logType, message string) string {
	// Usar json.Marshal para asegurar JSON válido
	data := map[string]interface{}{
		"type":      logType,
		"message":   sanitizeString(message),
		"timestamp": time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		// Fallback en caso de error
		return fmt.Sprintf(`{"type": "error", "message": "Error serializando log: %v", "timestamp": "%s"}`,
			err, time.Now().Format(time.RFC3339))
	}

	return string(jsonData)
}

// Utils

func findFreePort() (int, error) {
	// Implementar lógica para encontrar puerto libre
	// Por ahora, usar puerto aleatorio entre 3000-9999
	return 3000 + rand.Intn(7000), nil
}

func detectLanguage(repoURL string) (string, error) {
	// Implementar detección de lenguaje
	// Por ahora, usar Go por defecto
	return "go", nil
}

func generateDockerfile(repoURL, port, language string) (string, error) {
	// Implementar generación de Dockerfile según lenguaje
	template := `# Diplo - Dockerfile generado automáticamente
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
RUN git clone %s .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE %s
CMD ["./main"]`

	switch language {
	case "go":
		return fmt.Sprintf(template, repoURL, port), nil
	}

	return "", fmt.Errorf("lenguaje no soportado: %s", language)
}
