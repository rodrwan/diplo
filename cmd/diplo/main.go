package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rodrwan/diplo/internal/server"
	"github.com/sirupsen/logrus"
)

const (
	DefaultPort = 8080
	DefaultHost = "0.0.0.0"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	logrus.Info("=== Diplo - PaaS Local en Go ===")
	logrus.Info("Iniciando servidor...")

	// Crear servidor
	srv := server.NewServer(DefaultHost, DefaultPort)

	// Configurar shutdown graceful
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Manejar señales para shutdown graceful
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logrus.Info("Señal de shutdown recibida. Cerrando servidor...")
		cancel()
	}()

	// Iniciar servidor
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("Error iniciando servidor: %v", err)
		}
	}()

	// Esperar shutdown
	<-ctx.Done()

	// Shutdown graceful
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logrus.Errorf("Error durante shutdown: %v", err)
	}

	logrus.Info("Diplo terminado correctamente")
}
