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

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
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

	pollInterval, err := durationFromEnvironment(
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
