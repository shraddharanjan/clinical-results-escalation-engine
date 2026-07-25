package result

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/audit"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type WorkflowCreation struct {
	Result Result
	Task   clinicaltask.Task
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		pool: pool,
	}
}

func (r *PostgresRepository) CreateWorkflow(
	ctx context.Context,
	result Result,
	classification Classification,
) (WorkflowCreation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WorkflowCreation{}, fmt.Errorf(
			"begin result workflow transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	createdResult, err := insertResult(ctx, tx, result)
	if err != nil {
		return WorkflowCreation{}, err
	}

	createdTask, err := insertTask(
		ctx,
		tx,
		createdResult,
		classification,
	)
	if err != nil {
		return WorkflowCreation{}, err
	}

	if err := insertInitialAuditEvents(
		ctx,
		tx,
		createdResult,
		createdTask,
		classification,
	); err != nil {
		return WorkflowCreation{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkflowCreation{}, fmt.Errorf(
			"commit result workflow transaction: %w",
			err,
		)
	}

	return WorkflowCreation{
		Result: createdResult,
		Task:   createdTask,
	}, nil
}

func insertResult(
	ctx context.Context,
	tx pgx.Tx,
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

	err := tx.QueryRow(
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

	return Result{}, fmt.Errorf(
		"insert clinical result: %w",
		err,
	)
}

func insertTask(
	ctx context.Context,
	tx pgx.Tx,
	result Result,
	classification Classification,
) (clinicaltask.Task, error) {
	assignedTeam := assignedTeamFor(classification.Severity)
	acknowledgementDueAt := acknowledgementDeadlineFor(
		classification.Severity,
		time.Now().UTC(),
	)

	task := clinicaltask.Task{
		ResultID:             result.ID,
		TaskType:             clinicaltask.TaskTypeReviewResult,
		Status:               clinicaltask.StatusPending,
		Severity:             string(classification.Severity),
		AssignedTeam:         assignedTeam,
		EscalationLevel:      0,
		AcknowledgementDueAt: acknowledgementDueAt,
	}

	const query = `
		INSERT INTO clinical_tasks (
			result_id,
			task_type,
			status,
			severity,
			assigned_team,
			escalation_level,
			acknowledgement_due_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		)
		RETURNING
			id,
			available_at,
			attempt_count,
			version,
			created_at,
			updated_at
	`

	err := tx.QueryRow(
		ctx,
		query,
		task.ResultID,
		task.TaskType,
		task.Status,
		task.Severity,
		task.AssignedTeam,
		task.EscalationLevel,
		task.AcknowledgementDueAt,
	).Scan(
		&task.ID,
		&task.AvailableAt,
		&task.AttemptCount,
		&task.Version,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return clinicaltask.Task{}, fmt.Errorf(
			"insert clinical task: %w",
			err,
		)
	}

	return task, nil
}

func insertInitialAuditEvents(
	ctx context.Context,
	tx pgx.Tx,
	result Result,
	task clinicaltask.Task,
	classification Classification,
) error {
	resultReceivedPayload, err := json.Marshal(map[string]any{
		"source_system":     result.SourceSystem,
		"source_result_id":  result.SourceResultID,
		"patient_reference": result.PatientReference,
		"test_code":         result.TestCode,
		"reported_at":       result.ReportedAt,
		"received_at":       result.ReceivedAt,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal result received audit payload: %w",
			err,
		)
	}

	resultClassifiedPayload, err := json.Marshal(map[string]any{
		"severity":     classification.Severity,
		"matched_rule": classification.MatchedRule,
		"reason":       classification.Reason,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal classification audit payload: %w",
			err,
		)
	}

	taskCreatedPayload, err := json.Marshal(map[string]any{
		"task_type":              task.TaskType,
		"status":                 task.Status,
		"severity":               task.Severity,
		"assigned_team":          task.AssignedTeam,
		"escalation_level":       task.EscalationLevel,
		"acknowledgement_due_at": task.AcknowledgementDueAt,
	})
	if err != nil {
		return fmt.Errorf(
			"marshal task created audit payload: %w",
			err,
		)
	}

	events := []struct {
		aggregateType string
		aggregateID   any
		eventType     string
		payload       []byte
	}{
		{
			aggregateType: audit.AggregateClinicalResult,
			aggregateID:   result.ID,
			eventType:     audit.EventResultReceived,
			payload:       resultReceivedPayload,
		},
		{
			aggregateType: audit.AggregateClinicalResult,
			aggregateID:   result.ID,
			eventType:     audit.EventResultClassified,
			payload:       resultClassifiedPayload,
		},
		{
			aggregateType: audit.AggregateClinicalTask,
			aggregateID:   task.ID,
			eventType:     audit.EventTaskCreated,
			payload:       taskCreatedPayload,
		},
	}

	const query = `
		INSERT INTO audit_events (
			aggregate_type,
			aggregate_id,
			event_type,
			actor_type,
			payload
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5
		)
	`

	for _, event := range events {
		if _, err := tx.Exec(
			ctx,
			query,
			event.aggregateType,
			event.aggregateID,
			event.eventType,
			"system",
			event.payload,
		); err != nil {
			return fmt.Errorf(
				"insert %s audit event: %w",
				event.eventType,
				err,
			)
		}
	}

	return nil
}

func assignedTeamFor(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "acute-medicine"

	case SeverityUrgent:
		return "ward-medical-team"

	default:
		return "routine-results-team"
	}
}

func acknowledgementDeadlineFor(
	severity Severity,
	now time.Time,
) *time.Time {
	var deadline time.Time

	switch severity {
	case SeverityCritical:
		deadline = now.Add(5 * time.Minute)

	case SeverityUrgent:
		deadline = now.Add(30 * time.Minute)

	default:
		deadline = now.Add(24 * time.Hour)
	}

	return &deadline
}
