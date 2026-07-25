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

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/escalation"
	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/database"
	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/telemetry"
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
			"clinical-escalation-scheduler",
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

	applicationMetrics, err :=
		telemetry.NewMetrics()
	if err != nil {
		return fmt.Errorf(
			"create application metrics: %w",
			err,
		)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf(
			"DATABASE_URL is required",
		)
	}

	schedulerID := os.Getenv("SCHEDULER_ID")

	if schedulerID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf(
				"read hostname: %w",
				err,
			)
		}

		schedulerID = fmt.Sprintf(
			"scheduler-%s-%d",
			hostname,
			os.Getpid(),
		)
	}

	pollInterval, err :=
		durationFromEnvironment(
			"SCHEDULER_POLL_INTERVAL",
			time.Second,
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

	repository := escalation.NewRepository(
		databasePool,
	)

	scheduler, err := escalation.NewScheduler(
		repository,
		applicationMetrics,
		schedulerID,
		pollInterval,
	)
	if err != nil {
		return fmt.Errorf(
			"create escalation scheduler: %w",
			err,
		)
	}

	if err := scheduler.Run(ctx); err != nil {
		return fmt.Errorf(
			"run escalation scheduler: %w",
			err,
		)
	}

	return nil
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
