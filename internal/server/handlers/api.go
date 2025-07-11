package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/docker"
	"github.com/rodrwan/diplo/internal/models"
	runtimePkg "github.com/rodrwan/diplo/internal/runtime"
	"github.com/sirupsen/logrus"
)

type Context struct {
	docker  *docker.Client
	queries database.Querier
	// Para SSE - canales de logs por app
	logChannels map[string]chan string
	logMu       sync.RWMutex
}

// HybridContext extends Context with runtime factory support
type HybridContext struct {
	*Context
	runtimeFactory interface{}
}

func NewContext(docker *docker.Client, queries database.Querier, logChannels map[string]chan string) *Context {
	return &Context{
		docker:      docker,
		queries:     queries,
		logChannels: logChannels,
	}
}

// NewHybridContext creates a new HybridContext with runtime factory
func NewHybridContext(docker *docker.Client, queries database.Querier, logChannels map[string]chan string, runtimeFactory interface{}) *HybridContext {
	return &HybridContext{
		Context:        NewContext(docker, queries, logChannels),
		runtimeFactory: runtimeFactory,
	}
}

func DeployHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
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
		go redeployExistingApp(ctx, &existingApp)

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

	// Guardar en base de datos
	if err := ctx.queries.CreateApp(r.Context(), database.CreateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  req.RepoURL,
		Language: sql.NullString{String: "Go", Valid: true},
		Port:     int64(port),
		Status:   database.StatusDeploying,
	}); err != nil {
		logrus.Errorf("Error guardando aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error guardando aplicación"}, err
	}

	// Iniciar deployment en background
	go deployApp(ctx, app)

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

	return Response{Code: http.StatusCreated, Data: response}, nil
}

func deployApp(ctx *Context, app *database.App) {
	logrus.Infof("Iniciando deployment de: %s (%s)", app.Name, app.ID)

	// Configurar callback específico para esta aplicación
	originalCallback := ctx.docker.GetEventCallback()
	ctx.docker.SetEventCallback(func(event docker.DockerEvent) {
		// Enviar evento específico para esta aplicación
		sendDockerEventToApp(ctx, app.ID, event)
	})
	defer ctx.docker.SetEventCallback(originalCallback)

	// Enviar log inicial
	sendLogMessage(ctx, app.ID, "info", "Iniciando deployment...")

	// Actualizar estado
	app.Status = database.StatusDeploying
	ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
		ID:       app.ID,
		Name:     app.Name,
		RepoUrl:  app.RepoUrl,
		Language: sql.NullString{String: "Go", Valid: true},
		Port:     app.Port,
		Status:   app.Status,
	})

	// Detectar lenguaje
	sendLogMessage(ctx, app.ID, "info", "Detectando lenguaje...")
	language, err := detectLanguage(app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error detectando lenguaje: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error detectando lenguaje: %v", err), Valid: true}
		ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: sql.NullString{String: "Go", Valid: true},
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		sendLogMessage(ctx, app.ID, "error", fmt.Sprintf("Error detectando lenguaje: %v", err))
		return
	}
	app.Language = sql.NullString{String: language, Valid: true}
	logrus.Infof("Lenguaje detectado: %s", language)
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Lenguaje detectado: %s", language))

	// Generar Dockerfile
	sendLogMessage(ctx, app.ID, "info", "Generando Dockerfile...")
	dockerfile, err := generateDockerfile(app.RepoUrl, strconv.Itoa(int(app.Port)), language)
	if err != nil {
		logrus.Errorf("Error generando Dockerfile: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error generando Dockerfile: %v", err), Valid: true}
		ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		sendLogMessage(ctx, app.ID, "error", fmt.Sprintf("Error generando Dockerfile: %v", err))
		return
	}

	logrus.Debugf("Dockerfile generado:\n%s", dockerfile)
	sendLogMessage(ctx, app.ID, "info", "Dockerfile generado exitosamente")

	// Generar tag único basado en el hash del commit
	sendLogMessage(ctx, app.ID, "info", "Obteniendo hash del último commit...")
	imageTag, err := ctx.docker.GenerateImageTag(app.ID, app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error generando tag de imagen: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error generando tag de imagen: %v", err), Valid: true}
		ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		sendLogMessage(ctx, app.ID, "error", fmt.Sprintf("Error generando tag de imagen: %v", err))
		return
	}

	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Tag de imagen generado: %s", imageTag))

	// Construir imagen
	logrus.Infof("Construyendo imagen: %s", imageTag)
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Construyendo imagen Docker: %s", imageTag))

	imageID, err := ctx.docker.BuildImage(imageTag, dockerfile)
	if err != nil {
		logrus.Errorf("Error construyendo imagen: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error construyendo imagen Docker: %v", err), Valid: true}
		ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		sendLogMessage(ctx, app.ID, "error", fmt.Sprintf("Error construyendo imagen Docker: %v", err))

		// Limpiar imágenes dangling después de build fallido
		go func() {
			if err := ctx.docker.PruneDanglingImages(); err != nil {
				logrus.Warnf("Error limpiando imágenes dangling después de build fallido: %v", err)
			}
		}()

		return
	}

	sendLogMessage(ctx, app.ID, "success", "Imagen construida exitosamente")

	// Ejecutar contenedor
	logrus.Infof("Ejecutando contenedor en puerto %d", app.Port)
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Ejecutando contenedor en puerto %d", app.Port))
	containerID, err := ctx.docker.RunContainer(app, imageTag)
	if err != nil {
		logrus.Errorf("Error ejecutando contenedor: %v", err)
		app.Status = database.StatusError
		app.ErrorMsg = sql.NullString{String: fmt.Sprintf("Error ejecutando contenedor: %v", err), Valid: true}
		ctx.queries.UpdateApp(context.Background(), database.UpdateAppParams{
			ID:       app.ID,
			Name:     app.Name,
			RepoUrl:  app.RepoUrl,
			Language: app.Language,
			Port:     app.Port,
			Status:   app.Status,
			ErrorMsg: app.ErrorMsg,
		})
		sendLogMessage(ctx, app.ID, "error", fmt.Sprintf("Error ejecutando contenedor: %v", err))
		return
	}

	// Actualizar aplicación
	app.Status = database.StatusRunning
	app.ContainerID = sql.NullString{String: containerID, Valid: true}
	app.ImageID = sql.NullString{String: imageID, Valid: true}
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
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación: %v", err)
	}

	// Limpiar imágenes antiguas (mantener solo las 3 más recientes)
	go func() {
		if err := ctx.docker.CleanupOldImages(app.ID, 3); err != nil {
			logrus.Warnf("Error limpiando imágenes antiguas: %v", err)
		}

		// Limpiar imágenes dangling después del cleanup
		if err := ctx.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling: %v", err)
		}
	}()

	logrus.Infof("Deployment completado exitosamente: %s en puerto %d", app.ID, app.Port)
	sendLogMessage(ctx, app.ID, "success", fmt.Sprintf("Deployment completado exitosamente en puerto %d", app.Port))
	sendLogMessage(ctx, app.ID, "success", fmt.Sprintf("Aplicación disponible en: http://localhost:%d", app.Port))
}

func redeployExistingApp(ctx *Context, app *database.App) {
	logrus.Infof("Iniciando redeploy de aplicación existente: %s (%s)", app.Name, app.ID)

	// Configurar callback específico para esta aplicación
	originalCallback := ctx.docker.GetEventCallback()
	ctx.docker.SetEventCallback(func(event docker.DockerEvent) {
		sendDockerEventToApp(ctx, app.ID, event)
	})
	defer ctx.docker.SetEventCallback(originalCallback)

	// Enviar log inicial
	sendLogMessage(ctx, app.ID, "info", "Iniciando redeploy de aplicación existente...")

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
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando estado de redeploy: %v", err)
	}

	// Parar contenedor anterior si existe
	if app.ContainerID.String != "" {
		sendLogMessage(ctx, app.ID, "info", "Deteniendo contenedor anterior...")
		if err := ctx.docker.StopContainer(app.ContainerID.String); err != nil {
			logrus.Warnf("Error deteniendo contenedor anterior %s: %v", app.ContainerID.String, err)
			sendLogMessage(ctx, app.ID, "warning", fmt.Sprintf("Error deteniendo contenedor anterior: %v", err))
		} else {
			sendLogMessage(ctx, app.ID, "info", "Contenedor anterior detenido exitosamente")
		}
		// Limpiar container ID
		app.ContainerID = sql.NullString{String: "", Valid: true}
	}

	// Detectar lenguaje
	sendLogMessage(ctx, app.ID, "info", "Detectando lenguaje...")
	language, err := detectLanguage(app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error detectando lenguaje en redeploy: %v", err)
		handleRedeployError(ctx, app, fmt.Sprintf("Error detectando lenguaje: %v", err))
		return
	}
	app.Language = sql.NullString{String: language, Valid: true}
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Lenguaje detectado: %s", language))

	// Generar Dockerfile
	sendLogMessage(ctx, app.ID, "info", "Generando Dockerfile...")
	dockerfile, err := generateDockerfile(app.RepoUrl, strconv.Itoa(int(app.Port)), language)
	if err != nil {
		logrus.Errorf("Error generando Dockerfile en redeploy: %v", err)
		handleRedeployError(ctx, app, fmt.Sprintf("Error generando Dockerfile: %v", err))
		return
	}
	sendLogMessage(ctx, app.ID, "info", "Dockerfile generado exitosamente")

	// Generar nuevo tag único basado en el hash del commit actual
	sendLogMessage(ctx, app.ID, "info", "Obteniendo hash del último commit...")
	imageTag, err := ctx.docker.GenerateImageTag(app.ID, app.RepoUrl)
	if err != nil {
		logrus.Errorf("Error generando tag de imagen en redeploy: %v", err)
		handleRedeployError(ctx, app, fmt.Sprintf("Error generando tag de imagen: %v", err))
		return
	}
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Nuevo tag de imagen generado: %s", imageTag))

	// Construir nueva imagen
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Construyendo nueva imagen: %s", imageTag))
	imageID, err := ctx.docker.BuildImage(imageTag, dockerfile)
	if err != nil {
		logrus.Errorf("Error construyendo imagen en redeploy: %v", err)
		handleRedeployError(ctx, app, fmt.Sprintf("Error construyendo imagen Docker: %v", err))

		// Limpiar imágenes dangling después de build fallido
		go func() {
			if err := ctx.docker.PruneDanglingImages(); err != nil {
				logrus.Warnf("Error limpiando imágenes dangling después de build fallido: %v", err)
			}
		}()
		return
	}
	sendLogMessage(ctx, app.ID, "success", "Nueva imagen construida exitosamente")

	// Ejecutar nuevo contenedor
	sendLogMessage(ctx, app.ID, "info", fmt.Sprintf("Ejecutando nuevo contenedor en puerto %d", app.Port))
	containerID, err := ctx.docker.RunContainer(app, imageTag)
	if err != nil {
		logrus.Errorf("Error ejecutando contenedor en redeploy: %v", err)
		handleRedeployError(ctx, app, fmt.Sprintf("Error ejecutando contenedor: %v", err))
		return
	}

	// Actualizar aplicación con nueva información
	app.Status = database.StatusRunning
	app.ContainerID = sql.NullString{String: containerID, Valid: true}
	app.ImageID = sql.NullString{String: imageID, Valid: true}
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
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación después del redeploy: %v", err)
	}

	// Limpiar imágenes antiguas (mantener solo las 3 más recientes)
	go func() {
		if err := ctx.docker.CleanupOldImages(app.ID, 3); err != nil {
			logrus.Warnf("Error limpiando imágenes antiguas después del redeploy: %v", err)
		}

		// Limpiar imágenes dangling después del cleanup
		if err := ctx.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling después del redeploy: %v", err)
		}
	}()

	logrus.Infof("Redeploy completado exitosamente: %s en puerto %d", app.ID, app.Port)
	sendLogMessage(ctx, app.ID, "success", fmt.Sprintf("Redeploy completado exitosamente en puerto %d", app.Port))
	sendLogMessage(ctx, app.ID, "success", fmt.Sprintf("Aplicación actualizada disponible en: http://localhost:%d", app.Port))
}

// handleRedeployError maneja errores durante el redeploy
func handleRedeployError(ctx *Context, app *database.App, errorMsg string) {
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
		ImageID:     app.ImageID,
		UpdatedAt:   app.UpdatedAt,
	}); err != nil {
		logrus.Errorf("Error actualizando aplicación con error de redeploy: %v", err)
	}
	sendLogMessage(ctx, app.ID, "error", errorMsg)
}

// sendLogMessage envía un mensaje de log a todos los clientes conectados
func sendLogMessage(ctx *Context, appID, logType, message string) {
	ctx.logMu.RLock()
	if logChan, exists := ctx.logChannels[appID]; exists {
		logMsg := createLogMessage(logType, message)
		select {
		case logChan <- logMsg:
		default:
			// Canal lleno, ignorar mensaje
		}
	}
	ctx.logMu.RUnlock()
}

// sendDockerEventToApp envía un evento Docker específico a una aplicación
func sendDockerEventToApp(ctx *Context, appID string, event docker.DockerEvent) {
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
	ctx.logMu.RLock()
	if logChan, exists := ctx.logChannels[appID]; exists {
		select {
		case logChan <- string(eventJSON):
			logrus.Debugf("Evento Docker enviado a app %s: %s", appID, event.Type)
		default:
			// Canal lleno, ignorar evento
			logrus.Debugf("Canal de logs lleno para app %s, ignorando evento", appID)
		}
	}
	ctx.logMu.RUnlock()
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
	const (
		minPort     = 3000
		maxPort     = 9999
		maxAttempts = 50
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Generar puerto aleatorio en el rango
		port := minPort + rand.Intn(maxPort-minPort+1)

		// Verificar si el puerto está disponible
		if isPortAvailable(port) {
			logrus.Debugf("Puerto libre encontrado: %d (intento %d)", port, attempt+1)
			return port, nil
		}

		logrus.Debugf("Puerto %d ocupado, intentando otro...", port)
	}

	return 0, fmt.Errorf("no se pudo encontrar un puerto libre después de %d intentos", maxAttempts)
}

// isPortAvailable verifica si un puerto está disponible para uso
func isPortAvailable(port int) bool {
	// Intentar abrir el puerto en localhost
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		// Si no se puede abrir, está ocupado
		return false
	}

	// Si se puede abrir, cerrarlo inmediatamente
	listener.Close()
	return true
}

func detectLanguage(repoURL string) (string, error) {
	logrus.Debugf("Detectando lenguaje para repo: %s", repoURL)

	// Crear directorio temporal
	tempDir, err := os.MkdirTemp("", "diplo-detect-*")
	if err != nil {
		return "", fmt.Errorf("error creando directorio temporal: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clonar repositorio con profundidad mínima
	cloneCmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	if err := cloneCmd.Run(); err != nil {
		return "", fmt.Errorf("error clonando repositorio: %w", err)
	}

	// Definir archivos característicos por lenguaje
	languageIndicators := map[string][]string{
		"go":         {"go.mod", "go.sum", "main.go", "*.go"},
		"javascript": {"package.json", "yarn.lock", "package-lock.json", "app.js", "index.js", "server.js"},
		"python":     {"requirements.txt", "setup.py", "pyproject.toml", "Pipfile", "app.py", "main.py", "*.py"},
		"rust":       {"Cargo.toml", "Cargo.lock", "src/main.rs", "src/lib.rs"},
		"java":       {"pom.xml", "build.gradle", "gradlew", "src/main/java"},
		"php":        {"composer.json", "composer.lock", "index.php", "*.php"},
		"ruby":       {"Gemfile", "Gemfile.lock", "config.ru", "*.rb"},
	}

	// Buscar archivos característicos en orden de prioridad
	for language, indicators := range languageIndicators {
		for _, indicator := range indicators {
			var found bool
			if strings.Contains(indicator, "*") {
				// Usar pattern matching para archivos con wildcards
				matches, err := filepath.Glob(filepath.Join(tempDir, indicator))
				if err == nil && len(matches) > 0 {
					found = true
				}
				// También buscar en subdirectorios comunes
				matches, err = filepath.Glob(filepath.Join(tempDir, "src", indicator))
				if err == nil && len(matches) > 0 {
					found = true
				}
			} else {
				// Buscar archivo específico
				if _, err := os.Stat(filepath.Join(tempDir, indicator)); err == nil {
					found = true
				}
			}

			if found {
				logrus.Infof("Lenguaje detectado: %s (encontrado: %s)", language, indicator)
				return language, nil
			}
		}
	}

	// Si no se detecta ningún lenguaje, usar Go como fallback
	logrus.Warnf("No se pudo detectar el lenguaje para %s, usando Go como fallback", repoURL)
	return "go", nil
}

func generateDockerfile(repoURL, port, language string) (string, error) {
	logrus.Debugf("Generando Dockerfile para lenguaje: %s, puerto: %s", language, port)

	// Crear manager de templates Docker
	templateManager := runtimePkg.NewDockerTemplateManager()

	// Parsear puerto como entero
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("puerto inválido: %s", port)
	}

	// Renderizar el Dockerfile usando el template
	dockerfile, err := templateManager.RenderTemplate(language, portInt, repoURL)
	if err != nil {
		return "", fmt.Errorf("error renderizando template para %s: %w", language, err)
	}

	logrus.Debugf("Dockerfile generado exitosamente para %s", language)
	return dockerfile, nil
}
