package task

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const defaultReadLimit = 200

// ReadModel combines a task with the clinical-result fields needed by the UI.
type ReadModel struct {
	ID                   uuid.UUID  `json:"id"`
	ResultID             uuid.UUID  `json:"result_id"`
	TaskType             string     `json:"task_type"`
	Status               Status     `json:"status"`
	Severity             string     `json:"severity"`
	AssignedTeam         string     `json:"assigned_team"`
	AssignedUser         *string    `json:"assigned_user,omitempty"`
	EscalationLevel      int        `json:"escalation_level"`
	AvailableAt          time.Time  `json:"available_at"`
	AcknowledgementDueAt *time.Time `json:"acknowledgement_due_at,omitempty"`
	LeaseOwner           *string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt       *time.Time `json:"lease_expires_at,omitempty"`
	AttemptCount         int        `json:"attempt_count"`
	Version              int64      `json:"version"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`

	PatientReference string    `json:"patient_reference"`
	TestCode         string    `json:"test_code"`
	NumericValue     float64   `json:"value"`
	Unit             string    `json:"unit"`
	ReportedAt       time.Time `json:"reported_at"`
}

// ListForAPI returns the most recently created tasks and joins the result
// information required by the clinician dashboard.
func (r *PostgresRepository) ListForAPI(
	ctx context.Context,
) ([]ReadModel, error) {
	const query = `
		SELECT
			t.id,
			t.result_id,
			t.task_type,
			t.status,
			t.severity,
			t.assigned_team,
			t.assigned_user,
			t.escalation_level,
			t.available_at,
			t.acknowledgement_due_at,
			t.lease_owner,
			t.lease_expires_at,
			t.attempt_count,
			t.version,
			t.created_at,
			t.updated_at,
			r.patient_reference,
			r.test_code,
			r.numeric_value,
			r.unit,
			r.reported_at
		FROM clinical_tasks AS t
		INNER JOIN clinical_results AS r
			ON r.id = t.result_id
		ORDER BY
			CASE t.severity
				WHEN 'critical' THEN 1
				WHEN 'urgent' THEN 2
				ELSE 3
			END,
			t.created_at DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(
		ctx,
		query,
		defaultReadLimit,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"query clinical tasks: %w",
			err,
		)
	}
	defer rows.Close()

	tasks := make([]ReadModel, 0)

	for rows.Next() {
		var task ReadModel

		if err := rows.Scan(
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
			&task.PatientReference,
			&task.TestCode,
			&task.NumericValue,
			&task.Unit,
			&task.ReportedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan clinical task: %w",
				err,
			)
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate clinical tasks: %w",
			err,
		)
	}

	return tasks, nil
}
