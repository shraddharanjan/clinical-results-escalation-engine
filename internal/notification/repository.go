package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/audit"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(
	pool *pgxpool.Pool,
) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (r *Repository) GetOrCreateAttempt(
	ctx context.Context,
	task clinicaltask.Task,
	recipient string,
	channel string,
	idempotencyKey string,
) (Attempt, error) {
	const insertQuery = `
		INSERT INTO notification_attempts (
			task_id,
			escalation_level,
			recipient,
			channel,
			idempotency_key,
			status,
			attempt_count
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			'pending',
			0
		)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING
			id,
			task_id,
			escalation_level,
			recipient,
			channel,
			idempotency_key,
			status,
			provider_reference,
			attempt_count,
			last_error,
			next_attempt_at,
			created_at,
			updated_at,
			delivered_at
	`

	attempt, err := scanAttempt(
		r.pool.QueryRow(
			ctx,
			insertQuery,
			task.ID,
			task.EscalationLevel,
			recipient,
			channel,
			idempotencyKey,
		),
	)

	if err == nil {
		return attempt, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, fmt.Errorf(
			"create notification attempt: %w",
			err,
		)
	}

	const selectQuery = `
		SELECT
			id,
			task_id,
			escalation_level,
			recipient,
			channel,
			idempotency_key,
			status,
			provider_reference,
			attempt_count,
			last_error,
			next_attempt_at,
			created_at,
			updated_at,
			delivered_at
		FROM notification_attempts
		WHERE idempotency_key = $1
	`

	attempt, err = scanAttempt(
		r.pool.QueryRow(
			ctx,
			selectQuery,
			idempotencyKey,
		),
	)
	if err != nil {
		return Attempt{}, fmt.Errorf(
			"load notification attempt: %w",
			err,
		)
	}

	return attempt, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAttempt(
	row rowScanner,
) (Attempt, error) {
	var attempt Attempt

	err := row.Scan(
		&attempt.ID,
		&attempt.TaskID,
		&attempt.EscalationLevel,
		&attempt.Recipient,
		&attempt.Channel,
		&attempt.IdempotencyKey,
		&attempt.Status,
		&attempt.ProviderReference,
		&attempt.AttemptCount,
		&attempt.LastError,
		&attempt.NextAttemptAt,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
		&attempt.DeliveredAt,
	)

	return attempt, err
}

func (r *Repository) MarkRequested(
	ctx context.Context,
	attempt Attempt,
	workerID string,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf(
			"begin notification request transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	const updateQuery = `
		UPDATE notification_attempts
		SET
			status = 'pending',
			attempt_count = attempt_count + 1,
			last_error = NULL,
			next_attempt_at = NULL,
			updated_at = now()
		WHERE id = $1
		RETURNING attempt_count
	`

	var attemptCount int

	err = tx.QueryRow(
		ctx,
		updateQuery,
		attempt.ID,
	).Scan(&attemptCount)
	if err != nil {
		return fmt.Errorf(
			"mark notification requested: %w",
			err,
		)
	}

	payload, err := json.Marshal(map[string]any{
		"notification_attempt_id": attempt.ID,
		"idempotency_key":         attempt.IdempotencyKey,
		"recipient":               attempt.Recipient,
		"channel":                 attempt.Channel,
		"attempt_count":           attemptCount,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal notification requested payload: %w",
			err,
		)
	}

	if err := insertAuditEvent(
		ctx,
		tx,
		attempt.TaskID,
		audit.EventNotificationRequested,
		workerID,
		payload,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit notification request transaction: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) MarkTemporaryFailure(
	ctx context.Context,
	attempt Attempt,
	workerID string,
	deliveryError error,
	nextAttemptAt time.Time,
) error {
	return r.markFailure(
		ctx,
		attempt,
		workerID,
		StatusTemporaryFailed,
		audit.EventNotificationTemporaryFailed,
		deliveryError,
		&nextAttemptAt,
	)
}

func (r *Repository) MarkPermanentFailure(
	ctx context.Context,
	attempt Attempt,
	workerID string,
	deliveryError error,
) error {
	return r.markFailure(
		ctx,
		attempt,
		workerID,
		StatusPermanentFailed,
		audit.EventNotificationPermanentFailed,
		deliveryError,
		nil,
	)
}

func (r *Repository) markFailure(
	ctx context.Context,
	attempt Attempt,
	workerID string,
	status Status,
	eventType string,
	deliveryError error,
	nextAttemptAt *time.Time,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf(
			"begin notification failure transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	errorMessage := "unknown notification failure"
	if deliveryError != nil {
		errorMessage = deliveryError.Error()
	}

	const updateQuery = `
		UPDATE notification_attempts
		SET
			status = $2,
			last_error = $3,
			next_attempt_at = $4,
			updated_at = now()
		WHERE id = $1
	`

	_, err = tx.Exec(
		ctx,
		updateQuery,
		attempt.ID,
		status,
		errorMessage,
		nextAttemptAt,
	)
	if err != nil {
		return fmt.Errorf(
			"record notification failure: %w",
			err,
		)
	}

	payload, err := json.Marshal(map[string]any{
		"notification_attempt_id": attempt.ID,
		"idempotency_key":         attempt.IdempotencyKey,
		"recipient":               attempt.Recipient,
		"channel":                 attempt.Channel,
		"error":                   errorMessage,
		"next_attempt_at":         nextAttemptAt,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal notification failure payload: %w",
			err,
		)
	}

	if err := insertAuditEvent(
		ctx,
		tx,
		attempt.TaskID,
		eventType,
		workerID,
		payload,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit notification failure transaction: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) MarkDeliveredAndAwaitingAck(
	ctx context.Context,
	task clinicaltask.Task,
	attempt Attempt,
	workerID string,
	delivery Delivery,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf(
			"begin notification delivery transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	const attemptQuery = `
		UPDATE notification_attempts
		SET
			status = 'delivered',
			provider_reference = $2,
			last_error = NULL,
			next_attempt_at = NULL,
			delivered_at = COALESCE(delivered_at, now()),
			updated_at = now()
		WHERE id = $1
	`

	_, err = tx.Exec(
		ctx,
		attemptQuery,
		attempt.ID,
		delivery.ProviderReference,
	)
	if err != nil {
		return fmt.Errorf(
			"mark notification delivered: %w",
			err,
		)
	}

	const taskQuery = `
		UPDATE clinical_tasks
		SET
			status = 'awaiting_ack',
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = NULL,
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
		taskQuery,
		task.ID,
		workerID,
	).Scan(&newVersion)

	if errors.Is(err, pgx.ErrNoRows) {
		return clinicaltask.ErrLeaseLost
	}

	if err != nil {
		return fmt.Errorf(
			"move task to awaiting acknowledgement: %w",
			err,
		)
	}

	deliveryPayload, err := json.Marshal(map[string]any{
		"notification_attempt_id": attempt.ID,
		"idempotency_key":         attempt.IdempotencyKey,
		"provider_reference":      delivery.ProviderReference,
		"provider_accepted_at":    delivery.AcceptedAt,
		"deduplicated":            delivery.Deduplicated,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal notification delivered payload: %w",
			err,
		)
	}

	if err := insertAuditEvent(
		ctx,
		tx,
		task.ID,
		audit.EventNotificationDelivered,
		workerID,
		deliveryPayload,
	); err != nil {
		return err
	}

	awaitingPayload, err := json.Marshal(map[string]any{
		"previous_status":        clinicaltask.StatusProcessing,
		"new_status":             clinicaltask.StatusAwaitingAck,
		"acknowledgement_due_at": task.AcknowledgementDueAt,
		"version":                newVersion,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal awaiting acknowledgement payload: %w",
			err,
		)
	}

	if err := insertAuditEvent(
		ctx,
		tx,
		task.ID,
		audit.EventTaskAwaitingAcknowledgement,
		workerID,
		awaitingPayload,
	); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit notification delivery transaction: %w",
			err,
		)
	}

	return nil
}

func insertAuditEvent(
	ctx context.Context,
	tx pgx.Tx,
	taskID any,
	eventType string,
	workerID string,
	payload []byte,
) error {
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

	_, err := tx.Exec(
		ctx,
		query,
		audit.AggregateClinicalTask,
		taskID,
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
