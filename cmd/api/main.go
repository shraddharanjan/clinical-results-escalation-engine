package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/api"
	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/api/handlers"
	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/database"
	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/telemetry"
	clinicalresult "github.com/shraddharanjan/clinical-results-escalation-engine/internal/result"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		log.Printf(
			"warning: could not load .env file: %v",
			err,
		)
	}

	telemetryConfig, err :=
		telemetry.ConfigFromEnvironment(
			"clinical-results-api",
		)
	if err != nil {
		return fmt.Errorf(
			"load telemetry configuration: %w",
			err,
		)
	}

	telemetryProviders, err := telemetry.Initialise(
		context.Background(),
		telemetryConfig,
	)
	if err != nil {
		return fmt.Errorf(
			"initialise telemetry: %w",
			err,
		)
	}

	defer shutdownTelemetry(telemetryProviders)

	applicationMetrics, err := telemetry.NewMetrics()
	if err != nil {
		return fmt.Errorf(
			"create application metrics: %w",
			err,
		)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	httpPort := os.Getenv("HTTP_PORT")

	if httpPort == "" {
		httpPort = "8080"
	}

	ctx := context.Background()

	databasePool, err := database.Connect(
		ctx,
		databaseURL,
	)
	if err != nil {
		return fmt.Errorf(
			"connect to database: %w",
			err,
		)
	}
	defer databasePool.Close()

	resultRepository :=
		clinicalresult.NewPostgresRepository(
			databasePool,
		)

	resultService := clinicalresult.NewService(
		resultRepository,
		applicationMetrics,
	)

	taskRepository :=
		clinicaltask.NewPostgresRepository(
			databasePool,
		)

	healthHandler := handlers.NewHealthHandler()

	resultHandler := handlers.NewResultHandler(
		resultService,
	)

	acknowledgementHandler :=
		handlers.NewAcknowledgementHandler(
			taskRepository,
			applicationMetrics,
		)

	router := api.NewRouter(
		healthHandler,
		resultHandler,
		acknowledgementHandler,
	)

	server := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf(
			"API listening on http://localhost:%s",
			httpPort,
		)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdownSignals := make(chan os.Signal, 1)

	signal.Notify(
		shutdownSignals,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	select {
	case receivedSignal := <-shutdownSignals:
		log.Printf(
			"received shutdown signal: %s",
			receivedSignal,
		)

	case err := <-serverErrors:
		return fmt.Errorf(
			"HTTP server error: %w",
			err,
		)
	}

	shutdownContext, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf(
			"gracefully shut down HTTP server: %w",
			err,
		)
	}

	log.Println("API stopped")

	return nil
}

func shutdownTelemetry(
	providers *telemetry.Providers,
) {
	shutdownContext, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := providers.Shutdown(
		shutdownContext,
	); err != nil {
		log.Printf(
			"shut down telemetry: %v",
			err,
		)
	}
}
