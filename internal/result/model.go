package result

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Severity string

const (
	SeverityRoutine  Severity = "routine"
	SeverityUrgent   Severity = "urgent"
	SeverityCritical Severity = "critical"
)

type Result struct {
	ID               uuid.UUID       `json:"id"`
	SourceSystem     string          `json:"source_system"`
	SourceResultID   string          `json:"source_result_id"`
	PatientReference string          `json:"patient_reference"`
	TestCode         string          `json:"test_code"`
	NumericValue     float64         `json:"value"`
	Unit             string          `json:"unit"`
	ReportedAt       time.Time       `json:"reported_at"`
	ReceivedAt       time.Time       `json:"received_at"`
	Severity         Severity        `json:"severity"`
	MatchedRule      *string         `json:"matched_rule,omitempty"`
	RawPayload       json.RawMessage `json:"-"`
}

type CreateResultInput struct {
	SourceSystem     string    `json:"source_system"`
	SourceResultID   string    `json:"source_result_id"`
	PatientReference string    `json:"patient_reference"`
	TestCode         string    `json:"test_code"`
	NumericValue     float64   `json:"value"`
	Unit             string    `json:"unit"`
	ReportedAt       time.Time `json:"reported_at"`
}

type Classification struct {
	Severity    Severity `json:"severity"`
	MatchedRule string   `json:"matched_rule"`
	Reason      string   `json:"reason"`
}
