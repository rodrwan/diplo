package handlers

import (
	"fmt"
	"net/http"

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
