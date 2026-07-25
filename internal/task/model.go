package task

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending      Status = "pending"
	StatusProcessing   Status = "processing"
	StatusAwaitingAck  Status = "awaiting_ack"
	StatusAcknowledged Status = "acknowledged"
	StatusCompleted    Status = "completed"
	StatusEscalated    Status = "escalated"
	StatusFailed       Status = "failed"
)

const TaskTypeReviewResult = "review_clinical_result"

type Task struct {
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
	AttemptCount         int        `json:"attempt_count"`
	Version              int64      `json:"version"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}
