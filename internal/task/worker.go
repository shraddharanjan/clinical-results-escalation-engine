package task

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type TaskClaimer interface {
	ClaimOne(
		ctx context.Context,
		workerID string,
		leaseDuration time.Duration,
	) (Task, error)
}

type Worker struct {
	repository    TaskClaimer
	workerID      string
	pollInterval  time.Duration
	leaseDuration time.Duration
}

func NewWorker(
	repository TaskClaimer,
	workerID string,
	pollInterval time.Duration,
	leaseDuration time.Duration,
) (*Worker, error) {
	if repository == nil {
		return nil, fmt.Errorf("task repository is required")
	}

	if workerID == "" {
		return nil, fmt.Errorf("worker ID is required")
	}

	if pollInterval <= 0 {
		return nil, fmt.Errorf(
			"poll interval must be greater than zero",
		)
	}

	if leaseDuration <= 0 {
		return nil, fmt.Errorf(
			"lease duration must be greater than zero",
		)
	}

	return &Worker{
		repository:    repository,
		workerID:      workerID,
		pollInterval:  pollInterval,
		leaseDuration: leaseDuration,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	log.Printf(
		"worker %s started with poll interval %s and lease duration %s",
		w.workerID,
		w.pollInterval,
		w.leaseDuration,
	)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	// Try immediately rather than waiting for the first ticker event.
	w.claimOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("worker %s stopping", w.workerID)
			return nil

		case <-ticker.C:
			w.claimOnce(ctx)
		}
	}
}

func (w *Worker) claimOnce(ctx context.Context) {
	claimContext, cancel := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancel()

	claimedTask, err := w.repository.ClaimOne(
		claimContext,
		w.workerID,
		w.leaseDuration,
	)
	if errors.Is(err, ErrNoClaimableTask) {
		return
	}

	if err != nil {
		log.Printf(
			"worker %s failed to claim task: %v",
			w.workerID,
			err,
		)
		return
	}

	log.Printf(
		"worker %s claimed task %s: severity=%s team=%s attempt=%d lease_expires_at=%v",
		w.workerID,
		claimedTask.ID,
		claimedTask.Severity,
		claimedTask.AssignedTeam,
		claimedTask.AttemptCount,
		claimedTask.LeaseExpiresAt,
	)
}
