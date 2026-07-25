package audit

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	AggregateClinicalResult = "clinical_result"
	AggregateClinicalTask   = "clinical_task"

	EventResultReceived   = "result_received"
	EventResultClassified = "result_classified"
	EventTaskCreated      = "task_created"
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
