package notification

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending         Status = "pending"
	StatusDelivered       Status = "delivered"
	StatusTemporaryFailed Status = "temporary_failed"
	StatusPermanentFailed Status = "permanent_failed"
)

type Attempt struct {
	ID                uuid.UUID  `json:"id"`
	TaskID            uuid.UUID  `json:"task_id"`
	EscalationLevel   int        `json:"escalation_level"`
	Recipient         string     `json:"recipient"`
	Channel           string     `json:"channel"`
	IdempotencyKey    string     `json:"idempotency_key"`
	Status            Status     `json:"status"`
	ProviderReference *string    `json:"provider_reference,omitempty"`
	AttemptCount      int        `json:"attempt_count"`
	LastError         *string    `json:"last_error,omitempty"`
	NextAttemptAt     *time.Time `json:"next_attempt_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
}

type Message struct {
	TaskID         uuid.UUID
	IdempotencyKey string
	Recipient      string
	Channel        string
	Body           string
}

type Delivery struct {
	ProviderReference string
	AcceptedAt        time.Time
	Deduplicated      bool
}
