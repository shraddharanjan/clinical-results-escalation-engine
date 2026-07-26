package escalation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/telemetry"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type TaskEscalator interface {
	EscalateOne(
		ctx context.Context,
		schedulerID string,
	) (Outcome, error)
}

type Scheduler struct {
	repository   TaskEscalator
	metrics      *telemetry.Metrics
	schedulerID  string
	pollInterval time.Duration
}

func NewScheduler(
	repository TaskEscalator,
	metrics *telemetry.Metrics,
	schedulerID string,
	pollInterval time.Duration,
) (*Scheduler, error) {
	switch {
	case repository == nil:
		return nil, fmt.Errorf(
			"escalation repository is required",
		)

	case schedulerID == "":
		return nil, fmt.Errorf(
			"scheduler ID is required",
		)

	case pollInterval <= 0:
		return nil, fmt.Errorf(
			"poll interval must be greater than zero",
		)
	}

	return &Scheduler{
		repository:   repository,
		metrics:      metrics,
		schedulerID:  schedulerID,
		pollInterval: pollInterval,
	}, nil
}

func (s *Scheduler) Run(
	ctx context.Context,
) error {
	log.Printf(
		"scheduler %s started: poll=%s",
		s.schedulerID,
		s.pollInterval,
	)

	for {
		if ctx.Err() != nil {
			log.Printf(
				"scheduler %s stopping",
				s.schedulerID,
			)

			return nil
		}

		outcome, err := s.escalateOne(ctx)

		if errors.Is(err, ErrNoOverdueTask) {
			if err := waitForContext(
				ctx,
				s.pollInterval,
			); err != nil {
				return nil
			}

			continue
		}

		if errors.Is(
			err,
			clinicaltask.ErrTaskStateConflict,
		) {
			continue
		}

		if err != nil {
			log.Printf(
				"scheduler %s failed to process overdue task: %v",
				s.schedulerID,
				err,
			)

			if err := waitForContext(
				ctx,
				s.pollInterval,
			); err != nil {
				return nil
			}

			continue
		}

		s.metrics.RecordEscalation(
			ctx,
			outcome.Task.EscalationLevel,
		)

		if outcome.Exhausted {
			log.Printf(
				"scheduler %s exhausted escalation for task %s at level %d; task marked failed",
				s.schedulerID,
				outcome.Task.ID,
				outcome.Task.EscalationLevel,
			)

			continue
		}

		log.Printf(
			"scheduler %s escalated task %s to level %d team=%s",
			s.schedulerID,
			outcome.Task.ID,
			outcome.Task.EscalationLevel,
			outcome.Task.AssignedTeam,
		)
	}
}

func (s *Scheduler) escalateOne(
	ctx context.Context,
) (Outcome, error) {
	tracer := otel.Tracer(
		"clinical-results-escalation-engine/escalation",
	)

	escalationContext, span := tracer.Start(
		ctx,
		"task.escalate",
	)
	defer span.End()

	escalationContext, cancel :=
		context.WithTimeout(
			escalationContext,
			5*time.Second,
		)
	defer cancel()

	outcome, err := s.repository.EscalateOne(
		escalationContext,
		s.schedulerID,
	)

	if errors.Is(err, ErrNoOverdueTask) {
		return Outcome{}, err
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			err.Error(),
		)

		return Outcome{}, err
	}

	span.SetAttributes(
		attribute.String(
			"clinical.task.id",
			outcome.Task.ID.String(),
		),
		attribute.Int(
			"clinical.task.escalation_level",
			outcome.Task.EscalationLevel,
		),
		attribute.String(
			"clinical.task.assigned_team",
			outcome.Task.AssignedTeam,
		),
		attribute.String(
			"scheduler.id",
			s.schedulerID,
		),
		attribute.Bool(
			"clinical.task.escalation_exhausted",
			outcome.Exhausted,
		),
	)

	if outcome.Exhausted {
		span.SetStatus(
			codes.Ok,
			"maximum escalation level reached",
		)
	} else {
		span.SetStatus(
			codes.Ok,
			"task escalated",
		)
	}

	return outcome, nil
}

func waitForContext(
	ctx context.Context,
	duration time.Duration,
) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}
