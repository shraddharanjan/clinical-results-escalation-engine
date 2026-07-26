package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestDatabase(
	t *testing.T,
) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")

	if databaseURL == "" {
		t.Skip(
			"TEST_DATABASE_URL is not configured",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(
		ctx,
		databaseURL,
	)
	if err != nil {
		t.Fatalf(
			"create test database pool: %v",
			err,
		)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		t.Fatalf(
			"ping test database: %v",
			err,
		)
	}

	t.Cleanup(pool.Close)

	resetTestDatabase(t, pool)

	return pool
}

func resetTestDatabase(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()

	const query = `
		TRUNCATE TABLE
			fake_provider_deliveries,
			notification_attempts,
			audit_events,
			clinical_tasks,
			clinical_results
		RESTART IDENTITY
		CASCADE
	`

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if _, err := pool.Exec(
		ctx,
		query,
	); err != nil {
		t.Fatalf(
			"reset integration database: %v",
			err,
		)
	}
}
