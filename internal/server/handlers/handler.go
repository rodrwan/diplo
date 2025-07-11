package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"
)

type Response struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

type HandlerFunc func(ctx *Context, w http.ResponseWriter, r *http.Request) (Response, error)

// HybridHandlerFunc is a handler function that works with HybridContext
type HybridHandlerFunc func(ctx *HybridContext, w http.ResponseWriter, r *http.Request) (Response, error)

func (ctx *Context) ServeHTTP(handler HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response, err := handler(ctx, w, r)
		if err != nil {
			logrus.Errorf("Error en el handler: %v", err)
			// Response Error as json
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(response.Code)
			json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Code)
		json.NewEncoder(w).Encode(response)
	}
}

func (ctx *HybridContext) ServeHTTP(handler HybridHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response, err := handler(ctx, w, r)
		if err != nil {
			logrus.Errorf("Error en el hybrid handler: %v", err)
			// Response Error as json
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(response.Code)
			json.NewEncoder(w).Encode(response)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Code)
		json.NewEncoder(w).Encode(response)
	}
}
