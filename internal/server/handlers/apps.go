package handlers

import (
	"net/http"

	"github.com/gorilla/mux"
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
		logrus.Errorf("Error obteniendo aplicación: %v", err)
		return Response{Code: http.StatusInternalServerError, Message: "Error obteniendo aplicación"}, err
	}

	// Detener y eliminar contenedor si existe
	if app.ContainerID.String != "" {
		if err := ctx.docker.StopContainer(app.ContainerID.String); err != nil {
			logrus.Warnf("Error deteniendo contenedor %s: %v", app.ContainerID.String, err)
		}
	}

	// Eliminar de base de datos
	if err := ctx.queries.DeleteApp(r.Context(), appID); err != nil {
		logrus.Errorf("Error eliminando aplicación: %v", err)
	}

	response := map[string]interface{}{
		"message": "Aplicación eliminada exitosamente",
		"id":      appID,
	}

	// Limpiar imágenes dangling después de eliminar la app
	go func() {
		if err := ctx.docker.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error limpiando imágenes dangling después de eliminar app: %v", err)
		}
	}()

	return Response{Code: http.StatusOK, Data: response}, nil
}
