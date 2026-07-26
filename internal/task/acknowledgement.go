package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/audit"
)

type AcknowledgementResult struct {
	Task    Task
	Latency time.Duration
}

func (r *PostgresRepository) AcknowledgeWithLatency(
	ctx context.Context,
	taskID uuid.UUID,
	clinicianID string,
	expectedVersion int64,
) (AcknowledgementResult, error) {
	if strings.TrimSpace(clinicianID) == "" {
		return AcknowledgementResult{}, fmt.Errorf(
			"clinician ID is required",
		)
	}

	if expectedVersion <= 0 {
		return AcknowledgementResult{}, fmt.Errorf(
			"expected version must be greater than zero",
		)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AcknowledgementResult{}, fmt.Errorf(
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
		exists, lookupErr := taskExistsForAcknowledgement(
			ctx,
			tx,
			taskID,
		)
		if lookupErr != nil {
			return AcknowledgementResult{}, lookupErr
		}

		if !exists {
			return AcknowledgementResult{}, ErrTaskNotFound
		}

		return AcknowledgementResult{}, ErrTaskStateConflict
	}

	if err != nil {
		return AcknowledgementResult{}, fmt.Errorf(
			"acknowledge clinical task: %w",
			err,
		)
	}

	latency, err := calculateAcknowledgementLatency(
		ctx,
		tx,
		taskID,
		acknowledgedTask.UpdatedAt,
	)
	if err != nil {
		return AcknowledgementResult{}, err
	}

	payload, err := json.Marshal(map[string]any{
		"clinician_id":               clinicianID,
		"previous_status":            StatusAwaitingAck,
		"new_status":                 acknowledgedTask.Status,
		"escalation_level":           acknowledgedTask.EscalationLevel,
		"acknowledgement_latency_ms": latency.Milliseconds(),
		"version":                    acknowledgedTask.Version,
	})
	if err != nil {
		return AcknowledgementResult{}, fmt.Errorf(
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
		return AcknowledgementResult{}, fmt.Errorf(
			"insert task acknowledged audit event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return AcknowledgementResult{}, fmt.Errorf(
			"commit acknowledgement transaction: %w",
			err,
		)
	}

	return AcknowledgementResult{
		Task:    acknowledgedTask,
		Latency: latency,
	}, nil
}

func calculateAcknowledgementLatency(
	ctx context.Context,
	tx pgx.Tx,
	taskID uuid.UUID,
	acknowledgedAt time.Time,
) (time.Duration, error) {
	const query = `
		SELECT MAX(delivered_at)
		FROM notification_attempts
		WHERE
			task_id = $1
			AND status = 'delivered'
			AND delivered_at IS NOT NULL
	`

	var deliveredAt *time.Time

	if err := tx.QueryRow(
		ctx,
		query,
		taskID,
	).Scan(&deliveredAt); err != nil {
		return 0, fmt.Errorf(
			"load latest notification delivery time: %w",
			err,
		)
	}

	if deliveredAt == nil {
		return 0, nil
	}

	latency := acknowledgedAt.Sub(*deliveredAt)

	if latency < 0 {
		return 0, nil
	}

	return latency, nil
}

func taskExistsForAcknowledgement(
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
