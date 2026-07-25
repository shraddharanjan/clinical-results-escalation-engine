package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/audit"
)

var (
	ErrNoClaimableTask   = errors.New("no claimable task available")
	ErrLeaseLost         = errors.New("task lease was lost")
	ErrTaskNotFound      = errors.New("clinical task was not found")
	ErrTaskStateConflict = errors.New(
		"clinical task state or version has changed",
	)
)

type Claim struct {
	Task                   Task
	Recovered              bool
	PreviousStatus         Status
	PreviousLeaseOwner     *string
	PreviousLeaseExpiresAt *time.Time
}

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
) (Claim, error) {
	if workerID == "" {
		return Claim{}, fmt.Errorf("worker ID is required")
	}

	if leaseDuration <= 0 {
		return Claim{}, fmt.Errorf(
			"lease duration must be greater than zero",
		)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Claim{}, fmt.Errorf(
			"begin task claim transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	claim, err := claimTask(
		ctx,
		tx,
		workerID,
		leaseDuration,
	)
	if err != nil {
		return Claim{}, err
	}

	if err := insertClaimAuditEvent(
		ctx,
		tx,
		claim,
		workerID,
	); err != nil {
		return Claim{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Claim{}, fmt.Errorf(
			"commit task claim transaction: %w",
			err,
		)
	}

	return claim, nil
}

func claimTask(
	ctx context.Context,
	tx pgx.Tx,
	workerID string,
	leaseDuration time.Duration,
) (Claim, error) {
	const query = `
		WITH claimable_task AS (
			SELECT
				id,
				status::text AS previous_status,
				lease_owner AS previous_lease_owner,
				lease_expires_at AS previous_lease_expires_at
			FROM clinical_tasks
			WHERE
				(
					status IN ('pending', 'escalated')
					AND available_at <= now()
					AND (
						lease_expires_at IS NULL
						OR lease_expires_at <= now()
					)
				)
				OR
				(
					status = 'processing'
					AND (
						lease_expires_at IS NULL
						OR lease_expires_at <= now()
					)
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
			lease_expires_at =
				now() + ($2 * interval '1 millisecond'),
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
			task.updated_at,
			claimable_task.previous_status,
			claimable_task.previous_lease_owner,
			claimable_task.previous_lease_expires_at
	`

	var claim Claim

	err := tx.QueryRow(
		ctx,
		query,
		workerID,
		leaseDuration.Milliseconds(),
	).Scan(
		&claim.Task.ID,
		&claim.Task.ResultID,
		&claim.Task.TaskType,
		&claim.Task.Status,
		&claim.Task.Severity,
		&claim.Task.AssignedTeam,
		&claim.Task.AssignedUser,
		&claim.Task.EscalationLevel,
		&claim.Task.AvailableAt,
		&claim.Task.AcknowledgementDueAt,
		&claim.Task.LeaseOwner,
		&claim.Task.LeaseExpiresAt,
		&claim.Task.AttemptCount,
		&claim.Task.Version,
		&claim.Task.CreatedAt,
		&claim.Task.UpdatedAt,
		&claim.PreviousStatus,
		&claim.PreviousLeaseOwner,
		&claim.PreviousLeaseExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, ErrNoClaimableTask
	}

	if err != nil {
		return Claim{}, fmt.Errorf(
			"claim clinical task: %w",
			err,
		)
	}

	claim.Recovered = claim.PreviousStatus == StatusProcessing

	return claim, nil
}

func insertClaimAuditEvent(
	ctx context.Context,
	tx pgx.Tx,
	claim Claim,
	workerID string,
) error {
	eventType := audit.EventTaskClaimed

	if claim.Recovered {
		eventType = audit.EventTaskRecoveredAfterLeaseExpiry
	}

	payload, err := json.Marshal(map[string]any{
		"worker_id":                 workerID,
		"previous_status":           claim.PreviousStatus,
		"new_status":                claim.Task.Status,
		"previous_lease_owner":      claim.PreviousLeaseOwner,
		"previous_lease_expires_at": claim.PreviousLeaseExpiresAt,
		"lease_expires_at":          claim.Task.LeaseExpiresAt,
		"attempt_count":             claim.Task.AttemptCount,
		"version":                   claim.Task.Version,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal task claim audit payload: %w",
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
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.Exec(
		ctx,
		query,
		audit.AggregateClinicalTask,
		claim.Task.ID,
		eventType,
		"worker",
		workerID,
		payload,
	)
	if err != nil {
		return fmt.Errorf(
			"insert %s audit event: %w",
			eventType,
			err,
		)
	}

	return nil
}

func (r *PostgresRepository) RenewLease(
	ctx context.Context,
	taskID string,
	workerID string,
	leaseDuration time.Duration,
) (time.Time, error) {
	if leaseDuration <= 0 {
		return time.Time{}, fmt.Errorf(
			"lease duration must be greater than zero",
		)
	}

	const query = `
		UPDATE clinical_tasks
		SET
			lease_expires_at =
				now() + ($3 * interval '1 millisecond'),
			updated_at = now()
		WHERE
			id = $1
			AND status = 'processing'
			AND lease_owner = $2
			AND lease_expires_at > now()
		RETURNING lease_expires_at
	`

	var leaseExpiresAt time.Time

	err := r.pool.QueryRow(
		ctx,
		query,
		taskID,
		workerID,
		leaseDuration.Milliseconds(),
	).Scan(&leaseExpiresAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrLeaseLost
	}

	if err != nil {
		return time.Time{}, fmt.Errorf(
			"renew task lease: %w",
			err,
		)
	}

	return leaseExpiresAt, nil
}

func (r *PostgresRepository) ReleaseForRetry(
	ctx context.Context,
	task Task,
	workerID string,
	retryDelay time.Duration,
	processingError error,
) error {
	if retryDelay < 0 {
		return fmt.Errorf("retry delay cannot be negative")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf(
			"begin release-for-retry transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	const updateQuery = `
		UPDATE clinical_tasks
		SET
			status = 'pending',
			available_at =
				now() + ($3 * interval '1 millisecond'),
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $4,
			version = version + 1,
			updated_at = now()
		WHERE
			id = $1
			AND status = 'processing'
			AND lease_owner = $2
			AND lease_expires_at > now()
		RETURNING version, available_at
	`

	errorMessage := "unknown processing error"
	if processingError != nil {
		errorMessage = processingError.Error()
	}

	var (
		newVersion  int64
		availableAt time.Time
	)

	err = tx.QueryRow(
		ctx,
		updateQuery,
		task.ID,
		workerID,
		retryDelay.Milliseconds(),
		errorMessage,
	).Scan(
		&newVersion,
		&availableAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf(
			"release task for retry: %w",
			err,
		)
	}

	payload, err := json.Marshal(map[string]any{
		"worker_id":    workerID,
		"reason":       errorMessage,
		"available_at": availableAt,
		"version":      newVersion,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal release-for-retry payload: %w",
			err,
		)
	}

	const auditQuery = `
		INSERT INTO audit_events (
			aggregate_type,
			aggregate_id,
			event_type,
			actor_type,
			actor_id,
			payload
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.Exec(
		ctx,
		auditQuery,
		audit.AggregateClinicalTask,
		task.ID,
		audit.EventTaskReleasedForRetry,
		"worker",
		workerID,
		payload,
	)
	if err != nil {
		return fmt.Errorf(
			"insert release-for-retry audit event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit release-for-retry transaction: %w",
			err,
		)
	}

	return nil
}

func (r *PostgresRepository) MarkFailed(
	ctx context.Context,
	task Task,
	workerID string,
	processingError error,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf(
			"begin mark-failed transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	errorMessage := "unknown permanent processing failure"

	if processingError != nil {
		errorMessage = processingError.Error()
	}

	const updateQuery = `
		UPDATE clinical_tasks
		SET
			status = 'failed',
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $3,
			version = version + 1,
			updated_at = now()
		WHERE
			id = $1
			AND status = 'processing'
			AND lease_owner = $2
			AND lease_expires_at > now()
		RETURNING version
	`

	var newVersion int64

	err = tx.QueryRow(
		ctx,
		updateQuery,
		task.ID,
		workerID,
		errorMessage,
	).Scan(&newVersion)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf(
			"mark task failed: %w",
			err,
		)
	}

	payload, err := json.Marshal(map[string]any{
		"worker_id": workerID,
		"reason":    errorMessage,
		"version":   newVersion,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal task failed payload: %w",
			err,
		)
	}

	const auditQuery = `
		INSERT INTO audit_events (
			aggregate_type,
			aggregate_id,
			event_type,
			actor_type,
			actor_id,
			payload
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.Exec(
		ctx,
		auditQuery,
		audit.AggregateClinicalTask,
		task.ID,
		audit.EventTaskFailed,
		"worker",
		workerID,
		payload,
	)
	if err != nil {
		return fmt.Errorf(
			"insert task failed audit event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit mark-failed transaction: %w",
			err,
		)
	}

	return nil
}

func (r *PostgresRepository) Acknowledge(
	ctx context.Context,
	taskID uuid.UUID,
	clinicianID string,
	expectedVersion int64,
) (Task, error) {
	if clinicianID == "" {
		return Task{}, fmt.Errorf("clinician ID is required")
	}

	if expectedVersion <= 0 {
		return Task{}, fmt.Errorf(
			"expected version must be greater than zero",
		)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Task{}, fmt.Errorf(
			"begin acknowledgement transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	const updateQuery = `
		UPDATE clinical_tasks
		SET
			status = 'acknowledged',
			assigned_user = $2,
			acknowledgement_due_at = NULL,
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = NULL,
			version = version + 1,
			updated_at = now()
		WHERE
			id = $1
			AND status = 'awaiting_ack'
			AND version = $3
		RETURNING
			id,
			result_id,
			task_type,
			status,
			severity::text,
			assigned_team,
			assigned_user,
			escalation_level,
			available_at,
			acknowledgement_due_at,
			lease_owner,
			lease_expires_at,
			attempt_count,
			version,
			created_at,
			updated_at
	`

	var acknowledgedTask Task

	err = tx.QueryRow(
		ctx,
		updateQuery,
		taskID,
		clinicianID,
		expectedVersion,
	).Scan(
		&acknowledgedTask.ID,
		&acknowledgedTask.ResultID,
		&acknowledgedTask.TaskType,
		&acknowledgedTask.Status,
		&acknowledgedTask.Severity,
		&acknowledgedTask.AssignedTeam,
		&acknowledgedTask.AssignedUser,
		&acknowledgedTask.EscalationLevel,
		&acknowledgedTask.AvailableAt,
		&acknowledgedTask.AcknowledgementDueAt,
		&acknowledgedTask.LeaseOwner,
		&acknowledgedTask.LeaseExpiresAt,
		&acknowledgedTask.AttemptCount,
		&acknowledgedTask.Version,
		&acknowledgedTask.CreatedAt,
		&acknowledgedTask.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		exists, lookupErr := taskExists(
			ctx,
			tx,
			taskID,
		)
		if lookupErr != nil {
			return Task{}, lookupErr
		}

		if !exists {
			return Task{}, ErrTaskNotFound
		}

		return Task{}, ErrTaskStateConflict
	}

	if err != nil {
		return Task{}, fmt.Errorf(
			"acknowledge clinical task: %w",
			err,
		)
	}

	payload, err := json.Marshal(map[string]any{
		"clinician_id":     clinicianID,
		"previous_status":  StatusAwaitingAck,
		"new_status":       acknowledgedTask.Status,
		"escalation_level": acknowledgedTask.EscalationLevel,
		"version":          acknowledgedTask.Version,
	})
	if err != nil {
		return Task{}, fmt.Errorf(
			"marshal task acknowledgement payload: %w",
			err,
		)
	}

	const auditQuery = `
		INSERT INTO audit_events (
			aggregate_type,
			aggregate_id,
			event_type,
			actor_type,
			actor_id,
			payload
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = tx.Exec(
		ctx,
		auditQuery,
		audit.AggregateClinicalTask,
		acknowledgedTask.ID,
		audit.EventTaskAcknowledged,
		"clinician",
		clinicianID,
		payload,
	)
	if err != nil {
		return Task{}, fmt.Errorf(
			"insert task acknowledged audit event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf(
			"commit acknowledgement transaction: %w",
			err,
		)
	}

	return acknowledgedTask, nil
}

func taskExists(
	ctx context.Context,
	tx pgx.Tx,
	taskID uuid.UUID,
) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM clinical_tasks
			WHERE id = $1
		)
	`

	var exists bool

	if err := tx.QueryRow(
		ctx,
		query,
		taskID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf(
			"check whether clinical task exists: %w",
			err,
		)
	}

	return exists, nil
}
