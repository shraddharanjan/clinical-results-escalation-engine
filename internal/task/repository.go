package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/audit"
)

var ErrNoClaimableTask = errors.New("no claimable task available")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		pool: pool,
	}
}

func (r *PostgresRepository) ClaimOne(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) (Task, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Task{}, fmt.Errorf(
			"begin task claim transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	claimedTask, err := claimTask(
		ctx,
		tx,
		workerID,
		leaseDuration,
	)
	if err != nil {
		return Task{}, err
	}

	if err := insertTaskClaimedEvent(
		ctx,
		tx,
		claimedTask,
		workerID,
	); err != nil {
		return Task{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf(
			"commit task claim transaction: %w",
			err,
		)
	}

	return claimedTask, nil
}

func claimTask(
	ctx context.Context,
	tx pgx.Tx,
	workerID string,
	leaseDuration time.Duration,
) (Task, error) {
	const query = `
		WITH claimable_task AS (
			SELECT id
			FROM clinical_tasks
			WHERE
				status IN ('pending', 'escalated')
				AND available_at <= now()
				AND (
					lease_expires_at IS NULL
					OR lease_expires_at <= now()
				)
			ORDER BY
				CASE severity
					WHEN 'critical' THEN 3
					WHEN 'urgent' THEN 2
					WHEN 'routine' THEN 1
				END DESC,
				available_at ASC,
				created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE clinical_tasks AS task
		SET
			status = 'processing',
			lease_owner = $1,
			lease_expires_at = now() + ($2 * interval '1 millisecond'),
			attempt_count = task.attempt_count + 1,
			version = task.version + 1,
			updated_at = now()
		FROM claimable_task
		WHERE task.id = claimable_task.id
		RETURNING
			task.id,
			task.result_id,
			task.task_type,
			task.status,
			task.severity::text,
			task.assigned_team,
			task.assigned_user,
			task.escalation_level,
			task.available_at,
			task.acknowledgement_due_at,
			task.lease_owner,
			task.lease_expires_at,
			task.attempt_count,
			task.version,
			task.created_at,
			task.updated_at
	`

	leaseMilliseconds := leaseDuration.Milliseconds()

	if leaseMilliseconds <= 0 {
		return Task{}, fmt.Errorf(
			"lease duration must be greater than zero",
		)
	}

	var claimedTask Task

	err := tx.QueryRow(
		ctx,
		query,
		workerID,
		leaseMilliseconds,
	).Scan(
		&claimedTask.ID,
		&claimedTask.ResultID,
		&claimedTask.TaskType,
		&claimedTask.Status,
		&claimedTask.Severity,
		&claimedTask.AssignedTeam,
		&claimedTask.AssignedUser,
		&claimedTask.EscalationLevel,
		&claimedTask.AvailableAt,
		&claimedTask.AcknowledgementDueAt,
		&claimedTask.LeaseOwner,
		&claimedTask.LeaseExpiresAt,
		&claimedTask.AttemptCount,
		&claimedTask.Version,
		&claimedTask.CreatedAt,
		&claimedTask.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNoClaimableTask
	}

	if err != nil {
		return Task{}, fmt.Errorf(
			"claim clinical task: %w",
			err,
		)
	}

	return claimedTask, nil
}

func insertTaskClaimedEvent(
	ctx context.Context,
	tx pgx.Tx,
	task Task,
	workerID string,
) error {
	payload, err := json.Marshal(map[string]any{
		"worker_id":        workerID,
		"previous_status":  StatusPending,
		"new_status":       task.Status,
		"severity":         task.Severity,
		"assigned_team":    task.AssignedTeam,
		"attempt_count":    task.AttemptCount,
		"lease_expires_at": task.LeaseExpiresAt,
		"version":          task.Version,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal task claimed audit payload: %w",
			err,
		)
	}

	const query = `
		INSERT INTO audit_events (
			aggregate_type,
			aggregate_id,
			event_type,
			actor_type,
			actor_id,
			payload
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6
		)
	`

	_, err = tx.Exec(
		ctx,
		query,
		audit.AggregateClinicalTask,
		task.ID,
		audit.EventTaskClaimed,
		"worker",
		workerID,
		payload,
	)
	if err != nil {
		return fmt.Errorf(
			"insert task claimed audit event: %w",
			err,
		)
	}

	return nil
}
