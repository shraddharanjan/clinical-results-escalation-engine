package result

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidInput    = errors.New("invalid result input")
	ErrDuplicateResult = errors.New("result already exists")
)

type Repository interface {
	CreateWorkflow(
		ctx context.Context,
		result Result,
		classification Classification,
	) (WorkflowCreation, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) Create(
	ctx context.Context,
	input CreateResultInput,
) (WorkflowCreation, Classification, error) {
	if err := validateInput(input); err != nil {
		return WorkflowCreation{}, Classification{}, err
	}

	classification := classify(input)

	rawPayload, err := json.Marshal(input)
	if err != nil {
		return WorkflowCreation{}, Classification{}, fmt.Errorf(
			"marshal result payload: %w",
			err,
		)
	}

	matchedRule := classification.MatchedRule

	newResult := Result{
		SourceSystem:     input.SourceSystem,
		SourceResultID:   input.SourceResultID,
		PatientReference: input.PatientReference,
		TestCode:         input.TestCode,
		NumericValue:     input.NumericValue,
		Unit:             input.Unit,
		ReportedAt:       input.ReportedAt,
		Severity:         classification.Severity,
		MatchedRule:      &matchedRule,
		RawPayload:       rawPayload,
	}

	workflow, err := s.repository.CreateWorkflow(
		ctx,
		newResult,
		classification,
	)
	if err != nil {
		return WorkflowCreation{}, Classification{}, err
	}

	return workflow, classification, nil
}

func validateInput(input CreateResultInput) error {
	switch {
	case strings.TrimSpace(input.SourceSystem) == "":
		return fmt.Errorf("%w: source_system is required", ErrInvalidInput)

	case strings.TrimSpace(input.SourceResultID) == "":
		return fmt.Errorf("%w: source_result_id is required", ErrInvalidInput)

	case strings.TrimSpace(input.PatientReference) == "":
		return fmt.Errorf("%w: patient_reference is required", ErrInvalidInput)

	case strings.TrimSpace(input.TestCode) == "":
		return fmt.Errorf("%w: test_code is required", ErrInvalidInput)

	case strings.TrimSpace(input.Unit) == "":
		return fmt.Errorf("%w: unit is required", ErrInvalidInput)

	case input.ReportedAt.IsZero():
		return fmt.Errorf("%w: reported_at is required", ErrInvalidInput)

	case input.ReportedAt.After(time.Now().Add(5 * time.Minute)):
		return fmt.Errorf(
			"%w: reported_at cannot be in the future",
			ErrInvalidInput,
		)
	}

	return nil
}

func classify(input CreateResultInput) Classification {
	if input.TestCode == "serum_potassium" &&
		input.Unit == "mmol/L" &&
		input.NumericValue >= 6.5 {
		return Classification{
			Severity:    SeverityCritical,
			MatchedRule: "potassium-critical-high-v1",
			Reason:      "serum potassium value is at least 6.5 mmol/L",
		}
	}

	if input.TestCode == "serum_potassium" &&
		input.Unit == "mmol/L" &&
		input.NumericValue >= 5.5 {
		return Classification{
			Severity:    SeverityUrgent,
			MatchedRule: "potassium-urgent-high-v1",
			Reason:      "serum potassium value is at least 5.5 mmol/L",
		}
	}

	return Classification{
		Severity:    SeverityRoutine,
		MatchedRule: "default-routine-v1",
		Reason:      "no urgent or critical rule matched",
	}
}
