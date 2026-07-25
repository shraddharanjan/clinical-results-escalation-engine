package audit

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	AggregateClinicalResult = "clinical_result"
	AggregateClinicalTask   = "clinical_task"

	EventResultReceived                = "result_received"
	EventResultClassified              = "result_classified"
	EventTaskCreated                   = "task_created"
	EventTaskClaimed                   = "task_claimed"
	EventTaskRecoveredAfterLeaseExpiry = "task_recovered_after_lease_expiry"
	EventTaskReleasedForRetry          = "task_released_for_retry"
	EventNotificationRequested         = "notification_requested"

	EventNotificationDelivered       = "notification_delivered"
	EventNotificationTemporaryFailed = "notification_temporary_failed"
	EventNotificationPermanentFailed = "notification_permanent_failed"
	EventTaskAwaitingAcknowledgement = "task_awaiting_acknowledgement"
	EventTaskFailed                  = "task_failed"

	EventTaskAcknowledged              = "task_acknowledged"
	EventAcknowledgementDeadlineMissed = "acknowledgement_deadline_missed"
	EventTaskEscalated                 = "task_escalated"
)

type Event struct {
	SequenceNumber int64           `json:"sequence_number"`
	EventID        uuid.UUID       `json:"event_id"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    uuid.UUID       `json:"aggregate_id"`
	EventType      string          `json:"event_type"`
	ActorType      string          `json:"actor_type"`
	ActorID        *string         `json:"actor_id,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
	Payload        json.RawMessage `json:"payload"`
}
