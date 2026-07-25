package task

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type TaskStore interface {
	ClaimOne(
		ctx context.Context,
		workerID string,
		leaseDuration time.Duration,
	) (Claim, error)

	RenewLease(
		ctx context.Context,
		taskID string,
		workerID string,
		leaseDuration time.Duration,
	) (time.Time, error)

	ReleaseForRetry(
		ctx context.Context,
		task Task,
		workerID string,
		retryDelay time.Duration,
		processingError error,
	) error

	MarkFailed(
		ctx context.Context,
		task Task,
		workerID string,
		processingError error,
	) error
}
type Worker struct {
	repository      TaskStore
	processor       Processor
	workerID        string
	pollInterval    time.Duration
	leaseDuration   time.Duration
	renewalInterval time.Duration
	retryDelay      time.Duration
}

func NewWorker(
	repository TaskStore,
	processor Processor,
	workerID string,
	pollInterval time.Duration,
	leaseDuration time.Duration,
	renewalInterval time.Duration,
	retryDelay time.Duration,
) (*Worker, error) {
	switch {
	case repository == nil:
		return nil, fmt.Errorf("task repository is required")

	case processor == nil:
		return nil, fmt.Errorf("task processor is required")

	case workerID == "":
		return nil, fmt.Errorf("worker ID is required")

	case pollInterval <= 0:
		return nil, fmt.Errorf(
			"poll interval must be greater than zero",
		)

	case leaseDuration <= 0:
		return nil, fmt.Errorf(
			"lease duration must be greater than zero",
		)

	case renewalInterval <= 0:
		return nil, fmt.Errorf(
			"renewal interval must be greater than zero",
		)

	case renewalInterval >= leaseDuration:
		return nil, fmt.Errorf(
			"renewal interval must be shorter than lease duration",
		)

	case retryDelay < 0:
		return nil, fmt.Errorf(
			"retry delay cannot be negative",
		)
	}

	return &Worker{
		repository:      repository,
		processor:       processor,
		workerID:        workerID,
		pollInterval:    pollInterval,
		leaseDuration:   leaseDuration,
		renewalInterval: renewalInterval,
		retryDelay:      retryDelay,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	log.Printf(
		"worker %s started: poll=%s lease=%s renewal=%s retry=%s",
		w.workerID,
		w.pollInterval,
		w.leaseDuration,
		w.renewalInterval,
		w.retryDelay,
	)

	for {
		if ctx.Err() != nil {
			log.Printf("worker %s stopping", w.workerID)
			return nil
		}

		claim, err := w.claimOne(ctx)
		if errors.Is(err, ErrNoClaimableTask) {
			if err := waitForContext(ctx, w.pollInterval); err != nil {
				log.Printf("worker %s stopping", w.workerID)
				return nil
			}

			continue
		}

		if err != nil {
			log.Printf(
				"worker %s failed to claim task: %v",
				w.workerID,
				err,
			)

			if err := waitForContext(ctx, w.pollInterval); err != nil {
				return nil
			}

			continue
		}
		if claim.Recovered {
			previousOwner := "unknown"

			if claim.PreviousLeaseOwner != nil {
				previousOwner = *claim.PreviousLeaseOwner
			}

			log.Printf(
				"worker %s recovered task %s previously owned by %s",
				w.workerID,
				claim.Task.ID,
				previousOwner,
			)
		}

		w.processClaim(ctx, claim.Task)
	}
}

func (w *Worker) claimOne(ctx context.Context) (Claim, error) {
	claimContext, cancel := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancel()

	return w.repository.ClaimOne(
		claimContext,
		w.workerID,
		w.leaseDuration,
	)
}

func (w *Worker) processClaim(
	parentContext context.Context,
	task Task,
) {
	processingContext, cancelProcessing :=
		context.WithCancelCause(parentContext)

	defer cancelProcessing(nil)

	processResult := make(chan error, 1)

	go func() {
		processResult <- w.processor.Process(
			processingContext,
			task,
		)
	}()

	renewalTicker := time.NewTicker(w.renewalInterval)
	defer renewalTicker.Stop()

	for {
		select {
		case <-parentContext.Done():
			cancelProcessing(parentContext.Err())

			log.Printf(
				"worker %s stopped processing task %s; lease will expire",
				w.workerID,
				task.ID,
			)

			return

		case processingError := <-processResult:
			if processingError == nil {
				log.Printf(
					"worker %s processed task %s successfully",
					w.workerID,
					task.ID,
				)

				return
			}

			if errors.Is(processingError, context.Canceled) {
				return
			}

			releaseContext, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			if errors.Is(
				processingError,
				ErrPermanentProcessing,
			) {
				failureContext, cancelFailure := context.WithTimeout(
					context.Background(),
					5*time.Second,
				)

				err := w.repository.MarkFailed(
					failureContext,
					task,
					w.workerID,
					processingError,
				)

				cancelFailure()

				if errors.Is(err, ErrLeaseLost) {
					log.Printf(
						"worker %s could not fail task %s because ownership was lost",
						w.workerID,
						task.ID,
					)

					return
				}

				if err != nil {
					log.Printf(
						"worker %s failed to mark task %s failed: %v",
						w.workerID,
						task.ID,
						err,
					)

					return
				}

				log.Printf(
					"worker %s marked task %s permanently failed: %v",
					w.workerID,
					task.ID,
					processingError,
				)

				return
			}
			err := w.repository.ReleaseForRetry(
				releaseContext,
				task,
				w.workerID,
				w.retryDelay,
				processingError,
			)

			cancel()

			if errors.Is(err, ErrLeaseLost) {
				log.Printf(
					"worker %s could not release task %s because ownership was lost",
					w.workerID,
					task.ID,
				)

				return
			}

			if err != nil {
				log.Printf(
					"worker %s failed to release task %s: %v",
					w.workerID,
					task.ID,
					err,
				)

				return
			}

			log.Printf(
				"worker %s released task %s for retry: %v",
				w.workerID,
				task.ID,
				processingError,
			)

			return

		case <-renewalTicker.C:
			renewalContext, cancel := context.WithTimeout(
				parentContext,
				5*time.Second,
			)

			newExpiry, err := w.repository.RenewLease(
				renewalContext,
				task.ID.String(),
				w.workerID,
				w.leaseDuration,
			)

			cancel()

			if errors.Is(err, ErrLeaseLost) {
				cancelProcessing(ErrLeaseLost)

				log.Printf(
					"worker %s lost lease for task %s",
					w.workerID,
					task.ID,
				)

				return
			}

			if err != nil {
				log.Printf(
					"worker %s failed to renew task %s lease: %v",
					w.workerID,
					task.ID,
					err,
				)

				continue
			}

			log.Printf(
				"worker %s renewed task %s lease until %s",
				w.workerID,
				task.ID,
				newExpiry.Format(time.RFC3339),
			)
		}
	}
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
