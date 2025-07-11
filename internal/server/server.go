package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/docker"
	"github.com/rodrwan/diplo/internal/runtime"
	"github.com/rodrwan/diplo/internal/server/handlers"
	"github.com/rodrwan/diplo/internal/templates"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
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

	// Configurar rutas
	srv.setupRoutes()

	return srv
}

func (s *Server) setupRoutes() {
	// Middleware CORS
	s.router.Use(s.corsMiddleware)

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
	// SSE endpoint para logs en tiempo real
	api.HandleFunc("/apps/{id}/logs", ctx.ServeHTTP(handlers.LogsSSEHandler)).Methods("GET")

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
