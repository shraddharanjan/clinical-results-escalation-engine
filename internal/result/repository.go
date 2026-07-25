package result

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		pool: pool,
	}
}

func (r *PostgresRepository) Create(
	ctx context.Context,
	result Result,
) (Result, error) {
	const query = `
		INSERT INTO clinical_results (
			source_system,
			source_result_id,
			patient_reference,
			test_code,
			numeric_value,
			unit,
			reported_at,
			severity,
			matched_rule,
			raw_payload
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10
		)
		RETURNING
			id,
			received_at
	`

	err := r.pool.QueryRow(
		ctx,
		query,
		result.SourceSystem,
		result.SourceResultID,
		result.PatientReference,
		result.TestCode,
		result.NumericValue,
		result.Unit,
		result.ReportedAt,
		result.Severity,
		result.MatchedRule,
		result.RawPayload,
	).Scan(
		&result.ID,
		&result.ReceivedAt,
	)

	if err == nil {
		return result, nil
	}

	var postgresError *pgconn.PgError

	if errors.As(err, &postgresError) &&
		postgresError.Code == "23505" {
		return Result{}, ErrDuplicateResult
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("insert result returned no row")
	}

	return Result{}, fmt.Errorf("insert clinical result: %w", err)
}