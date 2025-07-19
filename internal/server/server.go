package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/docker"
	"github.com/rodrwan/diplo/internal/models"
	"github.com/rodrwan/diplo/internal/runtime"
	"github.com/rodrwan/diplo/internal/server/handlers"
	"github.com/rodrwan/diplo/internal/templates"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
	runtimePkg "github.com/rodrwan/diplo/internal/runtime"
)

type Server struct {
	router         *mux.Router
	server         *http.Server
	docker         *docker.Client
	runtimeFactory runtime.RuntimeFactory
	mu             sync.RWMutex
	db             *sql.DB
	queries        database.Querier
	// Para SSE - canales de logs por app
	logChannels map[string]chan string
}

// ensureDatabaseWritable verifica y corrige permisos de la base de datos
func ensureDatabaseWritable(dbPath string) error {
	// Verificar si el archivo existe
	if _, err := os.Stat(dbPath); err == nil {
		// El archivo existe, verificar permisos
		info, err := os.Stat(dbPath)
		if err != nil {
			return fmt.Errorf("error obteniendo información del archivo de BD: %v", err)
		}

		// Verificar si es escribible
		if info.Mode()&0200 == 0 {
			logrus.Warnf("Base de datos %s no es escribible, intentando corregir permisos...", dbPath)

			// Intentar cambiar permisos
			if err := os.Chmod(dbPath, 0644); err != nil {
				return fmt.Errorf("error cambiando permisos de BD: %v", err)
			}

			logrus.Infof("Permisos de base de datos corregidos exitosamente")
		}
	}

	return nil
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

	// Verificar y corregir permisos de la base de datos
	dbPath := "diplo.db"
	if err := ensureDatabaseWritable(dbPath); err != nil {
		logrus.Fatalf("error verificando permisos de base de datos: %v", err)
	}

	// Inicializar base de datos con permisos de escritura
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_temp_store=MEMORY")
	if err != nil {
		logrus.Fatalf("error abriendo base de datos: %v", err)
	}

	// Verificar que la base de datos es escribible
	if err := db.Ping(); err != nil {
		logrus.Fatalf("error verificando conectividad de base de datos: %v", err)
	}

	// Probar escritura para verificar permisos
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS _test_write (id INTEGER PRIMARY KEY)"); err != nil {
		logrus.Fatalf("error verificando permisos de escritura en base de datos: %v", err)
	}
	if _, err := db.Exec("DROP TABLE _test_write"); err != nil {
		logrus.Warnf("advertencia: no se pudo eliminar tabla de prueba: %v", err)
	}

	queries := database.New(db)
	if err := queries.CreateTables(context.Background()); err != nil {
		logrus.Fatalf("error creando tablas: %v", err)
	}
	srv.db = db
	srv.queries = queries

	// Inicializar runtime factory
	srv.runtimeFactory = runtime.NewDefaultRuntimeFactory()

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

	// Recuperar contenedores existentes al iniciar el servidor
	if err := srv.recoverContainers(); err != nil {
		logrus.Errorf("Error recuperando contenedores: %v", err)
	}

	// Configurar rutas
	srv.setupRoutes()

	return srv
}

func (s *Server) setupRoutes() {
	// Middleware CORS
	s.router.Use(s.corsMiddleware)

	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("internal/templates/static"))))

	// Frontend pages with unified layout
	s.router.HandleFunc("/", s.appsPageHandler).Methods("GET")
	s.router.HandleFunc("/apps", s.appsPageHandler).Methods("GET")
	s.router.HandleFunc("/deploy", s.deployPageHandler).Methods("GET")
	s.router.HandleFunc("/status", s.statusPageHandler).Methods("GET")
	s.router.HandleFunc("/logs", s.logsPageHandler).Methods("GET")
	// Health check
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")

	// API routes - Sistema unificado con detección automática de runtime
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Contexto híbrido para deployment inteligente
	hybridCtx := handlers.NewHybridContext(s.docker, s.queries, s.logChannels, s.runtimeFactory)

	// Endpoints principales con sistema híbrido
	api.HandleFunc("/status", hybridCtx.ServeHTTP(handlers.UnifiedStatusHandler)).Methods("GET")
	api.HandleFunc("/deploy", hybridCtx.ServeHTTP(handlers.UnifiedDeployHandler)).Methods("POST")

	// Contexto tradicional para gestión de apps y env vars
	ctx := handlers.NewContext(s.docker, s.queries, s.logChannels)

	// Endpoints de gestión de aplicaciones
	api.HandleFunc("/apps", ctx.ServeHTTP(handlers.ListAppsHandler)).Methods("GET")
	api.HandleFunc("/apps/{id}", ctx.ServeHTTP(handlers.GetAppHandler)).Methods("GET")
	api.HandleFunc("/apps/{id}", ctx.ServeHTTP(handlers.DeleteAppHandler)).Methods("DELETE")
	api.HandleFunc("/apps/{id}/health", ctx.ServeHTTP(handlers.HealthCheckHandler)).Methods("GET")
	// Environment variables endpoints
	api.HandleFunc("/apps/{id}/env", ctx.ServeHTTP(handlers.ListAppEnvVarsHandler)).Methods("GET")
	api.HandleFunc("/apps/{id}/env", ctx.ServeHTTP(handlers.CreateAppEnvVarHandler)).Methods("POST")
	api.HandleFunc("/apps/{id}/env/{key}", ctx.ServeHTTP(handlers.GetAppEnvVarHandler)).Methods("GET")
	api.HandleFunc("/apps/{id}/env/{key}", ctx.ServeHTTP(handlers.UpdateAppEnvVarHandler)).Methods("PUT")
	api.HandleFunc("/apps/{id}/env/{key}", ctx.ServeHTTP(handlers.DeleteAppEnvVarHandler)).Methods("DELETE")
	// Maintenance endpoints
	api.HandleFunc("/maintenance/prune-images", ctx.ServeHTTP(handlers.PruneImagesHandler)).Methods("POST")
	api.HandleFunc("/maintenance/cleanup-orphaned-containers", ctx.ServeHTTP(handlers.CleanupOrphanedContainersHandler)).Methods("POST")
	api.HandleFunc("/maintenance/aggressive-cleanup", ctx.ServeHTTP(handlers.AggressiveCleanupContainersHandler)).Methods("POST")
	api.HandleFunc("/maintenance/recover-containers", ctx.ServeHTTP(handlers.RecoverContainersHandler)).Methods("POST")
	// SSE endpoint para logs en tiempo real (maneja su propia respuesta)
	api.HandleFunc("/apps/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		// El handler SSE maneja su propia respuesta, no usar el wrapper JSON
		_, err := handlers.LogsSSEHandler(ctx, w, r)
		if err != nil {
			logrus.Errorf("Error en SSE handler: %v", err)
		}
	}).Methods("GET")

	// Endpoints específicos de runtime (opcional - para debugging)
	lxcAPI := s.router.PathPrefix("/api/lxc").Subrouter()
	lxcAPI.HandleFunc("/status", hybridCtx.ServeHTTP(handlers.HybridLXCStatusHandler)).Methods("GET")

	dockerAPI := s.router.PathPrefix("/api/docker").Subrouter()
	dockerAPI.HandleFunc("/status", hybridCtx.ServeHTTP(handlers.HybridDockerStatusHandler)).Methods("GET")
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) appsPageHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.AppsPage()
	components.Render(r.Context(), w)
}

func (s *Server) deployPageHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.DeployPage()
	components.Render(r.Context(), w)
}

func (s *Server) statusPageHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.StatusPage()
	components.Render(r.Context(), w)
}

func (s *Server) logsPageHandler(w http.ResponseWriter, r *http.Request) {
	components := templates.LogsPage()
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

// Start inicia el servidor forzando IPv4
func (s *Server) Start() error {
	listener, err := net.Listen("tcp4", s.server.Addr)
	if err != nil {
		return err
	}

	log.Printf("Servidor escuchando en http://%s\n", s.server.Addr)

	return s.server.Serve(listener)
}

// recoverContainers recupera contenedores que estaban ejecutándose antes del reinicio del servidor
func (s *Server) recoverContainers() error {
	logrus.Info("🔍 Iniciando recuperación de contenedores...")

	// Obtener todas las aplicaciones de la base de datos
	apps, err := s.queries.GetAllApps(context.Background())
	if err != nil {
		return fmt.Errorf("error obteniendo aplicaciones de BD: %v", err)
	}

	if len(apps) == 0 {
		logrus.Info("✅ No hay aplicaciones para recuperar")
		return nil
	}

	logrus.Infof("📋 Encontradas %d aplicaciones en BD", len(apps))

	// Obtener runtime preferido
	preferredRuntime := s.runtimeFactory.GetPreferredRuntime()
	logrus.Infof("🎯 Usando runtime preferido: %s", preferredRuntime)

	// Crear runtime para recuperación
	runtime, err := s.runtimeFactory.CreateRuntime(preferredRuntime)
	if err != nil {
		logrus.Errorf("Error creando runtime %s: %v", preferredRuntime, err)
		// Intentar fallback a Docker
		logrus.Warnf("Intentando fallback a Docker...")
		runtime, err = s.runtimeFactory.CreateRuntime(runtimePkg.RuntimeTypeDocker)
		if err != nil {
			return fmt.Errorf("error creando runtime de fallback: %v", err)
		}
		preferredRuntime = runtimePkg.RuntimeTypeDocker
	}
	defer runtime.Close()

	// Obtener contenedores ejecutándose según el runtime
	var runningContainers []*runtimePkg.Container
	var runningContainerMap map[string]bool

	switch preferredRuntime {
	case runtimePkg.RuntimeTypeDocker:
		// Usar Docker client
		dockerContainers, err := s.docker.GetRunningContainers()
		if err != nil {
			return fmt.Errorf("error obteniendo contenedores Docker: %v", err)
		}
		logrus.Infof("🐳 Encontrados %d contenedores Docker ejecutándose", len(dockerContainers))

		// Convertir a formato Container
		runningContainers = make([]*runtimePkg.Container, 0, len(dockerContainers))
		runningContainerMap = make(map[string]bool)
		for _, container := range dockerContainers {
			containerName := container.ID
			if len(container.Names) > 0 {
				containerName = container.Names[0]
			}
			runningContainers = append(runningContainers, &runtimePkg.Container{
				ID:        container.ID,
				Name:      containerName,
				Image:     container.Image,
				Status:    runtimePkg.ContainerStatusRunning,
				Runtime:   runtimePkg.RuntimeTypeDocker,
				CreatedAt: time.Unix(container.Created, 0),
			})
			runningContainerMap[container.ID] = true
		}

	case runtimePkg.RuntimeTypeContainerd:
		// Usar containerd client
		containerdContainers, err := runtime.GetRunningContainers()
		if err != nil {
			return fmt.Errorf("error obteniendo contenedores containerd: %v", err)
		}
		logrus.Infof("🔧 Encontrados %d contenedores containerd ejecutándose", len(containerdContainers))

		runningContainers = containerdContainers
		runningContainerMap = make(map[string]bool)
		for _, container := range containerdContainers {
			runningContainerMap[container.ID] = true
		}

	default:
		return fmt.Errorf("runtime no soportado: %s", preferredRuntime)
	}

	// Procesar cada aplicación
	recoveredCount := 0
	errorCount := 0

	for _, app := range apps {
		// Solo procesar aplicaciones que estaban en estado "running"
		if app.Status.String != "running" {
			logrus.Debugf("⏭️  Saltando app %s (estado: %s)", app.ID, app.Status.String)
			continue
		}

		// Verificar si el contenedor está realmente ejecutándose
		containerID := app.ContainerID.String
		if containerID == "" {
			logrus.Warnf("⚠️  App %s marcada como running pero sin container_id", app.ID)
			// Marcar como error
			if err := s.updateAppStatus(app.ID, "error", "Contenedor perdido durante reinicio"); err != nil {
				logrus.Errorf("Error actualizando estado de app %s: %v", app.ID, err)
				errorCount++
			}
			continue
		}

		if runningContainerMap[containerID] {
			// Contenedor está ejecutándose - verificar que esté healthy
			logrus.Infof("✅ Contenedor %s para app %s está ejecutándose", containerID, app.ID)

			// Verificar health del contenedor según el runtime
			var status string
			var statusErr error

			switch preferredRuntime {
			case runtimePkg.RuntimeTypeDocker:
				status, statusErr = s.docker.GetContainerStatus(containerID)
			case runtimePkg.RuntimeTypeContainerd:
				status, statusErr = runtime.GetContainerStatus(containerID)
			default:
				statusErr = fmt.Errorf("runtime no soportado para health check")
			}

			if statusErr != nil {
				logrus.Warnf("⚠️  Error verificando estado del contenedor %s: %v", containerID, statusErr)
				// Marcar como error
				if err := s.updateAppStatus(app.ID, "error", fmt.Sprintf("Error verificando contenedor: %v", statusErr)); err != nil {
					logrus.Errorf("Error actualizando estado de app %s: %v", app.ID, err)
					errorCount++
				}
				continue
			}

			if strings.Contains(strings.ToUpper(status), "RUNNING") {
				logrus.Infof("✅ App %s recuperada exitosamente (contenedor: %s)", app.ID, containerID)
				recoveredCount++
			} else {
				logrus.Warnf("⚠️  Contenedor %s no está running (estado: %s)", containerID, status)
				// Marcar como error
				if err := s.updateAppStatus(app.ID, "error", fmt.Sprintf("Contenedor no está running: %s", status)); err != nil {
					logrus.Errorf("Error actualizando estado de app %s: %v", app.ID, err)
					errorCount++
				}
			}
		} else {
			// Contenedor no está ejecutándose - intentar recrearlo
			logrus.Warnf("⚠️  Contenedor %s para app %s no está ejecutándose, intentando recrear...", containerID, app.ID)

			if err := s.recreateContainer(&app, preferredRuntime); err != nil {
				logrus.Errorf("❌ Error recreando contenedor para app %s: %v", app.ID, err)
				// Marcar como error
				if err := s.updateAppStatus(app.ID, "error", fmt.Sprintf("Error recreando contenedor: %v", err)); err != nil {
					logrus.Errorf("Error actualizando estado de app %s: %v", app.ID, err)
					errorCount++
				}
			} else {
				logrus.Infof("✅ Contenedor recreado exitosamente para app %s", app.ID)
				recoveredCount++
			}
		}
	}

	logrus.Infof("🎯 Recuperación completada: %d recuperadas, %d errores", recoveredCount, errorCount)
	return nil
}

// updateAppStatus actualiza el estado de una aplicación en la base de datos
func (s *Server) updateAppStatus(appID, status, errorMsg string) error {
	ctx := context.Background()

	// Obtener la app actual
	app, err := s.queries.GetApp(ctx, appID)
	if err != nil {
		return fmt.Errorf("error obteniendo app %s: %v", appID, err)
	}

	// Actualizar estado
	app.Status = sql.NullString{String: status, Valid: true}
	if errorMsg != "" {
		app.ErrorMsg = sql.NullString{String: errorMsg, Valid: true}
	}
	app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}

	// Guardar en BD
	return s.queries.UpdateApp(ctx, database.UpdateAppParams{
		ID:        app.ID,
		Name:      app.Name,
		RepoUrl:   app.RepoUrl,
		Language:  app.Language,
		Port:      app.Port,
		Status:    app.Status,
		ErrorMsg:  app.ErrorMsg,
		UpdatedAt: app.UpdatedAt,
	})
}

// recreateContainer intenta recrear un contenedor para una aplicación
func (s *Server) recreateContainer(app *database.App, runtimeType runtimePkg.RuntimeType) error {
	logrus.Infof("🔄 Recreando contenedor para app %s", app.ID)

	// Obtener variables de entorno
	envVars, err := s.queries.GetAppEnvVars(context.Background(), app.ID)
	if err != nil {
		return fmt.Errorf("error obteniendo variables de entorno: %v", err)
	}

	// Convertir a formato models.EnvVar
	envVarsList := make([]models.EnvVar, 0, len(envVars))
	for _, env := range envVars {
		value := env.Value

		// Descifrar valores secretos si es necesario
		if env.IsSecret.Bool {
			if decryptedValue, err := handlers.DecryptValue(env.Value); err != nil {
				logrus.Errorf("Error descifrando valor secreto %s: %v", env.Key, err)
				continue
			} else {
				value = decryptedValue
			}
		}

		envVarsList = append(envVarsList, models.EnvVar{
			Name:  env.Key,
			Value: value,
		})
	}

	// Intentar recrear el contenedor usando la imagen existente
	imageID := app.ImageID.String
	if imageID == "" {
		return fmt.Errorf("no hay image_id disponible para recrear contenedor")
	}

	// Ejecutar nuevo contenedor
	var containerID string
	var containerErr error

	switch runtimeType {
	case runtimePkg.RuntimeTypeDocker:
		containerID, containerErr = s.docker.RunContainer(app, imageID, envVarsList)
	case runtimePkg.RuntimeTypeContainerd:
		// Para containerd, necesitamos crear el contenedor usando la interfaz
		// Por ahora, usar Docker como fallback
		logrus.Warnf("Recreación de contenedor containerd no implementada, usando Docker como fallback")
		containerID, containerErr = s.docker.RunContainer(app, imageID, envVarsList)
	default:
		return fmt.Errorf("runtime no soportado para recrear contenedor: %s", runtimeType)
	}

	if containerErr != nil {
		return fmt.Errorf("error ejecutando contenedor: %v", containerErr)
	}

	// Actualizar aplicación con nuevo container_id
	app.ContainerID = sql.NullString{String: containerID, Valid: true}
	app.Status = database.StatusRunning
	app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
	app.ErrorMsg = sql.NullString{String: "", Valid: true}

	return s.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:          app.ID,
		Name:        app.Name,
		RepoUrl:     app.RepoUrl,
		Language:    app.Language,
		Port:        app.Port,
		Status:      app.Status,
		ErrorMsg:    app.ErrorMsg,
		ContainerID: app.ContainerID,
		UpdatedAt:   app.UpdatedAt,
	})
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
