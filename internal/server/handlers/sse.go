package handlers

import (
	"bufio"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// LogsSSEHandler maneja las conexiones SSE para logs en tiempo real
func LogsSSEHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	vars := mux.Vars(r)
	appID := vars["id"]

	app, err := ctx.queries.GetApp(r.Context(), appID)
	if err != nil {
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	// Configurar headers para SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Crear canal para logs si no existe
	ctx.logMu.Lock()
	if _, exists := ctx.logChannels[appID]; !exists {
		ctx.logChannels[appID] = make(chan string, 100) // Buffer de 100 mensajes
	}
	logChan := ctx.logChannels[appID]
	ctx.logMu.Unlock()

	// Enviar mensaje inicial
	fmt.Fprintf(w, "data: %s\n\n", `{"type": "connected", "message": "Conexión SSE establecida"}`)
	w.(http.Flusher).Flush()

	// Escuchar logs del contenedor si está ejecutándose
	if app.ContainerID.String != "" {
		go streamContainerLogs(ctx, app.ContainerID.String, logChan)
	}

	// Escuchar canal de logs
	for {
		select {
		case logMsg := <-logChan:
			fmt.Fprintf(w, "data: %s\n\n", logMsg)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			// Cliente desconectado
			ctx.logMu.Lock()
			delete(ctx.logChannels, appID)
			ctx.logMu.Unlock()
			return Response{Code: http.StatusOK, Message: "Conexión SSE cerrada"}, nil
		}
	}
}

// streamContainerLogs obtiene logs del contenedor en tiempo real
func streamContainerLogs(ctx *Context, containerID string, logChan chan<- string) {
	logs, err := ctx.docker.GetContainerLogsStream(containerID)
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
