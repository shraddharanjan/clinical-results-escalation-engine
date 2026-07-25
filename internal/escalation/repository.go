package escalation

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

var ErrNoOverdueTask = errors.New(
	"no overdue acknowledgement task available",
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

func (r *Repository) EscalateOne(
	ctx context.Context,
	schedulerID string,
) (clinicaltask.Task, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"begin escalation transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	overdueTask, err := selectOverdueTask(
		ctx,
		tx,
	)
	if err != nil {
		return clinicaltask.Task{}, err
	}

	previousLevel := overdueTask.EscalationLevel
	previousTeam := overdueTask.AssignedTeam
	previousDeadline := overdueTask.AcknowledgementDueAt

	nextLevel := previousLevel + 1
	nextTeam, acknowledgementTimeout :=
		escalationTarget(nextLevel)

	nextDeadline := time.Now().UTC().Add(
		acknowledgementTimeout,
	)

	const updateQuery = `
		UPDATE clinical_tasks
		SET
			status = 'pending',
			assigned_team = $2,
			assigned_user = NULL,
			escalation_level = $3,
			available_at = now(),
			acknowledgement_due_at = $4,
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = NULL,
			version = version + 1,
			updated_at = now()
		WHERE
			id = $1
			AND status = 'awaiting_ack'
			AND version = $5
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

	var escalatedTask clinicaltask.Task

	err = tx.QueryRow(
		ctx,
		updateQuery,
		overdueTask.ID,
		nextTeam,
		nextLevel,
		nextDeadline,
		overdueTask.Version,
	).Scan(
		&escalatedTask.ID,
		&escalatedTask.ResultID,
		&escalatedTask.TaskType,
		&escalatedTask.Status,
		&escalatedTask.Severity,
		&escalatedTask.AssignedTeam,
		&escalatedTask.AssignedUser,
		&escalatedTask.EscalationLevel,
		&escalatedTask.AvailableAt,
		&escalatedTask.AcknowledgementDueAt,
		&escalatedTask.LeaseOwner,
		&escalatedTask.LeaseExpiresAt,
		&escalatedTask.AttemptCount,
		&escalatedTask.Version,
		&escalatedTask.CreatedAt,
		&escalatedTask.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return clinicaltask.Task{},
			clinicaltask.ErrTaskStateConflict
	}

	if err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"escalate clinical task: %w",
			err,
		)
	}

	deadlinePayload, err := json.Marshal(map[string]any{
		"previous_deadline": previousDeadline,
		"escalation_level":  previousLevel,
		"assigned_team":     previousTeam,
	})
	if err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"marshal deadline-missed payload: %w",
			err,
		)
	}

	if err := insertAuditEvent(
		ctx,
		tx,
		overdueTask.ID,
		audit.EventAcknowledgementDeadlineMissed,
		schedulerID,
		deadlinePayload,
	); err != nil {
		return clinicaltask.Task{}, err
	}

	escalationPayload, err := json.Marshal(map[string]any{
		"previous_level":  previousLevel,
		"new_level":       nextLevel,
		"previous_team":   previousTeam,
		"new_team":        nextTeam,
		"previous_status": clinicaltask.StatusAwaitingAck,
		"new_status":      clinicaltask.StatusPending,
		"new_deadline":    nextDeadline,
		"version":         escalatedTask.Version,
	})
	if err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"marshal task escalation payload: %w",
			err,
		)
	}

	if err := insertAuditEvent(
		ctx,
		tx,
		overdueTask.ID,
		audit.EventTaskEscalated,
		schedulerID,
		escalationPayload,
	); err != nil {
		return clinicaltask.Task{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"commit escalation transaction: %w",
			err,
		)
	}

	return escalatedTask, nil
}

func selectOverdueTask(
	ctx context.Context,
	tx pgx.Tx,
) (clinicaltask.Task, error) {
	const query = `
		SELECT
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
		FROM clinical_tasks
		WHERE
			status = 'awaiting_ack'
			AND acknowledgement_due_at <= now()
		ORDER BY
			CASE severity
				WHEN 'critical' THEN 3
				WHEN 'urgent' THEN 2
				WHEN 'routine' THEN 1
			END DESC,
			acknowledgement_due_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`

	var task clinicaltask.Task

	err := tx.QueryRow(
		ctx,
		query,
	).Scan(
		&task.ID,
		&task.ResultID,
		&task.TaskType,
		&task.Status,
		&task.Severity,
		&task.AssignedTeam,
		&task.AssignedUser,
		&task.EscalationLevel,
		&task.AvailableAt,
		&task.AcknowledgementDueAt,
		&task.LeaseOwner,
		&task.LeaseExpiresAt,
		&task.AttemptCount,
		&task.Version,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return clinicaltask.Task{}, ErrNoOverdueTask
	}

	if err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"select overdue clinical task: %w",
			err,
		)
	}

	return task, nil
}

func escalationTarget(
	level int,
) (string, time.Duration) {
	switch level {
	case 1:
		return "medical-registrar", 5 * time.Minute

	case 2:
		return "consultant-on-call", 10 * time.Minute

	default:
		return "site-operations-team", 15 * time.Minute
	}
}

func insertAuditEvent(
	ctx context.Context,
	tx pgx.Tx,
	taskID any,
	eventType string,
	schedulerID string,
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
		"scheduler",
		schedulerID,
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
