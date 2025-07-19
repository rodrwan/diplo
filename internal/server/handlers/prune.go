package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rodrwan/diplo/internal/database"
	"github.com/rodrwan/diplo/internal/models"
	"github.com/rodrwan/diplo/internal/runtime"
	"github.com/sirupsen/logrus"
)

func PruneImagesHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	logrus.Info("Manual prune of dangling images requested")

	// Ejecutar limpieza de imágenes dangling
	err := ctx.docker.PruneDanglingImages()
	if err != nil {
		logrus.Errorf("Error durante limpieza manual de imágenes: %v", err)
		response := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Error limpiando imágenes dangling: %v", err),
		}

		return Response{Code: http.StatusInternalServerError, Data: response}, err
	}

	response := map[string]interface{}{
		"success": true,
		"message": "Imágenes dangling limpiadas exitosamente",
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}

func RecoverContainersHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error) {
	logrus.Info("Recuperación manual de contenedores solicitada")

	// Obtener todas las aplicaciones de la base de datos
	apps, err := ctx.queries.GetAllApps(r.Context())
	if err != nil {
		logrus.Errorf("Error obteniendo aplicaciones para recuperación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicaciones"}, err
	}

	// Determinar runtime preferido
	preferredRuntime := runtime.RuntimeTypeDocker // Default a Docker
	availableRuntimes := []runtime.RuntimeType{runtime.RuntimeTypeDocker}

	// Intentar detectar containerd si está disponible
	containerdClient, err := runtime.NewContainerdClient("", "")
	if err == nil {
		defer containerdClient.Close()
		availableRuntimes = append(availableRuntimes, runtime.RuntimeTypeContainerd)
		preferredRuntime = runtime.RuntimeTypeContainerd
		logrus.Info("Containerd detectado, usando como runtime preferido")
	} else {
		logrus.Info("Containerd no disponible, usando Docker")
	}

	// Obtener contenedores ejecutándose según el runtime
	var runningContainers []*runtime.Container
	var runningContainerMap map[string]bool

	switch preferredRuntime {
	case runtime.RuntimeTypeDocker:
		// Usar Docker client
		dockerContainers, err := ctx.docker.GetRunningContainers()
		if err != nil {
			logrus.Errorf("Error obteniendo contenedores Docker: %v", err)
			return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo contenedores"}, err
		}
		logrus.Infof("🐳 Encontrados %d contenedores Docker ejecutándose", len(dockerContainers))

		// Convertir a formato Container
		runningContainers = make([]*runtime.Container, 0, len(dockerContainers))
		runningContainerMap = make(map[string]bool)
		for _, container := range dockerContainers {
			containerName := container.ID
			if len(container.Names) > 0 {
				containerName = container.Names[0]
			}
			runningContainers = append(runningContainers, &runtime.Container{
				ID:        container.ID,
				Name:      containerName,
				Image:     container.Image,
				Status:    runtime.ContainerStatusRunning,
				Runtime:   runtime.RuntimeTypeDocker,
				CreatedAt: time.Unix(container.Created, 0),
			})
			runningContainerMap[container.ID] = true
		}

	case runtime.RuntimeTypeContainerd:
		// Usar containerd client
		containerdContainers, err := containerdClient.GetRunningContainers()
		if err != nil {
			logrus.Errorf("Error obteniendo contenedores containerd: %v", err)
			return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo contenedores"}, err
		}
		logrus.Infof("🔧 Encontrados %d contenedores containerd ejecutándose", len(containerdContainers))

		runningContainers = containerdContainers
		runningContainerMap = make(map[string]bool)
		for _, container := range containerdContainers {
			runningContainerMap[container.ID] = true
		}

	default:
		return Response{Code: http.StatusInternalServerError, Message: "Runtime no soportado"}, fmt.Errorf("runtime no soportado: %s", preferredRuntime)
	}

	// Procesar cada aplicación
	recoveredCount := 0
	errorCount := 0
	skippedCount := 0

	for _, app := range apps {
		// Solo procesar aplicaciones que estaban en estado "running"
		if app.Status.String != "running" {
			skippedCount++
			continue
		}

		// Verificar si el contenedor está realmente ejecutándose
		containerID := app.ContainerID.String
		if containerID == "" {
			logrus.Warnf("App %s marcada como running pero sin container_id", app.ID)
			errorCount++
			continue
		}

		if runningContainerMap[containerID] {
			// Contenedor está ejecutándose - verificar que esté healthy
			var status string
			var statusErr error

			switch preferredRuntime {
			case runtime.RuntimeTypeDocker:
				status, statusErr = ctx.docker.GetContainerStatus(containerID)
			case runtime.RuntimeTypeContainerd:
				status, statusErr = containerdClient.GetContainerStatus(containerID)
			default:
				statusErr = fmt.Errorf("runtime no soportado para health check")
			}

			if statusErr != nil {
				logrus.Warnf("Error verificando estado del contenedor %s: %v", containerID, statusErr)
				errorCount++
				continue
			}

			if strings.Contains(strings.ToUpper(status), "RUNNING") {
				logrus.Infof("App %s ya está ejecutándose correctamente", app.ID)
				recoveredCount++
			} else {
				logrus.Warnf("Contenedor %s no está running (estado: %s)", containerID, status)
				errorCount++
			}
		} else {
			// Contenedor no está ejecutándose - intentar recrearlo
			logrus.Infof("Intentando recrear contenedor para app %s", app.ID)

			// Obtener variables de entorno
			envVars, err := ctx.queries.GetAppEnvVars(r.Context(), app.ID)
			if err != nil {
				logrus.Errorf("Error obteniendo variables de entorno para app %s: %v", app.ID, err)
				errorCount++
				continue
			}

			// Convertir a formato models.EnvVar
			envVarsList := make([]models.EnvVar, 0, len(envVars))
			for _, env := range envVars {
				value := env.Value

				// Descifrar valores secretos si es necesario
				if env.IsSecret.Bool {
					if decryptedValue, err := DecryptValue(env.Value); err != nil {
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
				logrus.Errorf("No hay image_id disponible para recrear contenedor de app %s", app.ID)
				errorCount++
				continue
			}

			// Ejecutar nuevo contenedor según el runtime
			var containerID string
			var containerErr error

			switch preferredRuntime {
			case runtime.RuntimeTypeDocker:
				containerID, containerErr = ctx.docker.RunContainer(&app, imageID, envVarsList)
			case runtime.RuntimeTypeContainerd:
				// Para containerd, usar Docker como fallback por ahora
				logrus.Warnf("Recreación de contenedor containerd no implementada, usando Docker como fallback")
				containerID, containerErr = ctx.docker.RunContainer(&app, imageID, envVarsList)
			default:
				containerErr = fmt.Errorf("runtime no soportado para recrear contenedor: %s", preferredRuntime)
			}

			if containerErr != nil {
				logrus.Errorf("Error ejecutando contenedor para app %s: %v", app.ID, containerErr)
				errorCount++
				continue
			}

			// Actualizar aplicación con nuevo container_id
			app.ContainerID = sql.NullString{String: containerID, Valid: true}
			app.Status = database.StatusRunning
			app.UpdatedAt = sql.NullTime{Time: time.Now(), Valid: true}
			app.ErrorMsg = sql.NullString{String: "", Valid: true}

			if err := ctx.queries.UpdateApp(r.Context(), database.UpdateAppParams{
				ID:          app.ID,
				Name:        app.Name,
				RepoUrl:     app.RepoUrl,
				Language:    app.Language,
				Port:        app.Port,
				Status:      app.Status,
				ErrorMsg:    app.ErrorMsg,
				ContainerID: app.ContainerID,
				UpdatedAt:   app.UpdatedAt,
			}); err != nil {
				logrus.Errorf("Error actualizando app %s después de recrear contenedor: %v", app.ID, err)
				errorCount++
				continue
			}

			logrus.Infof("Contenedor recreado exitosamente para app %s", app.ID)
			recoveredCount++
		}
	}

	response := map[string]interface{}{
		"success":            true,
		"message":            "Recuperación de contenedores completada",
		"recovered":          recoveredCount,
		"errors":             errorCount,
		"skipped":            skippedCount,
		"total_apps":         len(apps),
		"running_containers": len(runningContainers),
		"runtime_used":       string(preferredRuntime),
		"available_runtimes": availableRuntimes,
	}

	return Response{Code: http.StatusOK, Data: response}, nil
}
