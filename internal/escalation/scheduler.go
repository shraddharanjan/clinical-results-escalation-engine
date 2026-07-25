package escalation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type TaskEscalator interface {
	EscalateOne(
		ctx context.Context,
		schedulerID string,
	) (clinicaltask.Task, error)
}

type Scheduler struct {
	repository   TaskEscalator
	schedulerID  string
	pollInterval time.Duration
}

func NewScheduler(
	repository TaskEscalator,
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

		escalatedTask, err := s.escalateOne(ctx)

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
				"scheduler %s failed to escalate task: %v",
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

		log.Printf(
			"scheduler %s escalated task %s to level %d team=%s",
			s.schedulerID,
			escalatedTask.ID,
			escalatedTask.EscalationLevel,
			escalatedTask.AssignedTeam,
		)
	}
}

func (s *Scheduler) escalateOne(
	ctx context.Context,
) (clinicaltask.Task, error) {
	escalationContext, cancel :=
		context.WithTimeout(
			ctx,
			5*time.Second,
		)
	defer cancel()

	return s.repository.EscalateOne(
		escalationContext,
		s.schedulerID,
	)
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
