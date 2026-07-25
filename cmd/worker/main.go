package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/database"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		log.Printf("warning: could not load .env file: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	workerID, err := resolveWorkerID()
	if err != nil {
		return err
	}

	pollInterval, err := durationFromEnvironment(
		"WORKER_POLL_INTERVAL",
		500*time.Millisecond,
	)
	if err != nil {
		return err
	}

	leaseDuration, err := durationFromEnvironment(
		"WORKER_LEASE_DURATION",
		30*time.Second,
	)
	if err != nil {
		return err
	}

	renewalInterval, err := durationFromEnvironment(
		"WORKER_RENEWAL_INTERVAL",
		10*time.Second,
	)
	if err != nil {
		return err
	}

	processingDuration, err := durationFromEnvironment(
		"WORKER_PROCESSING_DURATION",
		20*time.Second,
	)
	if err != nil {
		return err
	}

	retryDelay, err := durationFromEnvironment(
		"WORKER_RETRY_DELAY",
		30*time.Second,
	)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	databasePool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer databasePool.Close()

	taskRepository := clinicaltask.NewPostgresRepository(
		databasePool,
	)

	processor, err := clinicaltask.NewPlaceholderProcessor(
		processingDuration,
	)
	if err != nil {
		return fmt.Errorf("create task processor: %w", err)
	}

	worker, err := clinicaltask.NewWorker(
		taskRepository,
		processor,
		workerID,
		pollInterval,
		leaseDuration,
		renewalInterval,
		retryDelay,
	)
	if err != nil {
		return fmt.Errorf("create worker: %w", err)
	}
	if err := worker.Run(ctx); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}

	return nil
}

func resolveWorkerID() (string, error) {
	if configuredID := os.Getenv("WORKER_ID"); configuredID != "" {
		return configuredID, nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("read hostname: %w", err)
	}

	return fmt.Sprintf(
		"worker-%s-%d",
		hostname,
		os.Getpid(),
	), nil
}

func durationFromEnvironment(
	name string,
	defaultValue time.Duration,
) (time.Duration, error) {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf(
			"parse %s duration %q: %w",
			name,
			value,
			err,
		)
	}

	if duration <= 0 {
		return 0, fmt.Errorf(
			"%s must be greater than zero",
			name,
		)
	}

	return duration, nil
}
