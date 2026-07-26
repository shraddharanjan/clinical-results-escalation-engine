package task

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
	"go.opentelemetry.io/otel/trace"
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
	metrics         *telemetry.Metrics
	workerID        string
	pollInterval    time.Duration
	leaseDuration   time.Duration
	renewalInterval time.Duration
	retryDelay      time.Duration
}

func NewWorker(
	repository TaskStore,
	processor Processor,
	metrics *telemetry.Metrics,
	workerID string,
	pollInterval time.Duration,
	leaseDuration time.Duration,
	renewalInterval time.Duration,
	retryDelay time.Duration,
) (*Worker, error) {
	switch {
	case repository == nil:
		return nil, fmt.Errorf(
			"task repository is required",
		)

	case processor == nil:
		return nil, fmt.Errorf(
			"task processor is required",
		)

	case workerID == "":
		return nil, fmt.Errorf(
			"worker ID is required",
		)

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
		metrics:         metrics,
		workerID:        workerID,
		pollInterval:    pollInterval,
		leaseDuration:   leaseDuration,
		renewalInterval: renewalInterval,
		retryDelay:      retryDelay,
	}, nil
}

func (w *Worker) Run(
	ctx context.Context,
) error {
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
			log.Printf(
				"worker %s stopping",
				w.workerID,
			)

			return nil
		}

		claim, err := w.claimOne(ctx)

		if errors.Is(err, ErrNoClaimableTask) {
			if err := waitForContext(
				ctx,
				w.pollInterval,
			); err != nil {
				log.Printf(
					"worker %s stopping",
					w.workerID,
				)

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

			if err := waitForContext(
				ctx,
				w.pollInterval,
			); err != nil {
				return nil
			}

			continue
		}

		w.metrics.RecordTaskClaim(
			ctx,
			claim.Task.Severity,
			claim.Recovered,
		)

		if claim.Recovered {
			previousOwner := "unknown"

			if claim.PreviousLeaseOwner != nil {
				previousOwner =
					*claim.PreviousLeaseOwner
			}

			log.Printf(
				"worker %s recovered task %s previously owned by %s",
				w.workerID,
				claim.Task.ID,
				previousOwner,
			)
		} else {
			log.Printf(
				"worker %s claimed task %s severity=%s",
				w.workerID,
				claim.Task.ID,
				claim.Task.Severity,
			)
		}

		w.processClaim(
			ctx,
			claim.Task,
			claim.Recovered,
		)
	}
}

func (w *Worker) claimOne(
	ctx context.Context,
) (Claim, error) {
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
	recovered bool,
) {
	tracer := otel.Tracer(
		"clinical-results-escalation-engine/worker",
	)

	tracedContext, span := tracer.Start(
		parentContext,
		"task.process",
	)
	defer span.End()

	processingContext, cancelProcessing :=
		context.WithCancelCause(tracedContext)

	defer cancelProcessing(nil)

	processingStartedAt := time.Now()

	defer func() {
		w.metrics.RecordProcessingDuration(
			processingContext,
			task.Severity,
			time.Since(processingStartedAt),
		)
	}()

	span.SetAttributes(
		attribute.String(
			"clinical.task.id",
			task.ID.String(),
		),
		attribute.String(
			"clinical.task.severity",
			task.Severity,
		),
		attribute.String(
			"clinical.task.assigned_team",
			task.AssignedTeam,
		),
		attribute.Int(
			"clinical.task.escalation_level",
			task.EscalationLevel,
		),
		attribute.Int(
			"clinical.task.attempt_count",
			task.AttemptCount,
		),
		attribute.String(
			"worker.id",
			w.workerID,
		),
		attribute.Bool(
			"clinical.task.recovered",
			recovered,
		),
	)

	if recovered {
		span.AddEvent(
			"task recovered after lease expiry",
		)
	}

	processResult := make(chan error, 1)

	go func() {
		processResult <- w.processor.Process(
			processingContext,
			task,
		)
	}()

	renewalTicker := time.NewTicker(
		w.renewalInterval,
	)
	defer renewalTicker.Stop()

	for {
		select {
		case <-parentContext.Done():
			cancelProcessing(
				parentContext.Err(),
			)

			span.AddEvent(
				"worker stopped while processing",
			)

			log.Printf(
				"worker %s stopped processing task %s; lease will expire",
				w.workerID,
				task.ID,
			)

			return

		case processingError := <-processResult:
			if processingError == nil {
				span.SetStatus(
					codes.Ok,
					"task processed",
				)

				log.Printf(
					"worker %s processed task %s successfully",
					w.workerID,
					task.ID,
				)

				return
			}

			if errors.Is(
				processingError,
				context.Canceled,
			) {
				span.AddEvent(
					"processing cancelled",
				)

				return
			}

			span.RecordError(processingError)
			span.SetStatus(
				codes.Error,
				processingError.Error(),
			)

			if errors.Is(
				processingError,
				ErrPermanentProcessing,
			) {
				w.markTaskFailed(
					task,
					processingError,
				)

				return
			}

			w.releaseTaskForRetry(
				task,
				processingError,
			)

			return

		case <-renewalTicker.C:
			renewalContext, cancelRenewal :=
				context.WithTimeout(
					parentContext,
					5*time.Second,
				)

			newExpiry, err :=
				w.repository.RenewLease(
					renewalContext,
					task.ID.String(),
					w.workerID,
					w.leaseDuration,
				)

			cancelRenewal()

			if errors.Is(err, ErrLeaseLost) {
				cancelProcessing(ErrLeaseLost)

				span.RecordError(ErrLeaseLost)
				span.SetStatus(
					codes.Error,
					"task lease lost",
				)
				span.AddEvent(
					"task lease lost",
				)

				log.Printf(
					"worker %s lost lease for task %s",
					w.workerID,
					task.ID,
				)

				return
			}

			if err != nil {
				span.RecordError(err)
				span.AddEvent(
					"lease renewal failed",
				)

				log.Printf(
					"worker %s failed to renew task %s lease: %v",
					w.workerID,
					task.ID,
					err,
				)

				continue
			}

			w.metrics.RecordLeaseRenewal(
				processingContext,
			)

			span.AddEvent(
				"task lease renewed",
				trace.WithAttributes(
					attribute.String(
						"lease.expires_at",
						newExpiry.Format(time.RFC3339),
					),
				),
			)

			log.Printf(
				"worker %s renewed task %s lease until %s",
				w.workerID,
				task.ID,
				newExpiry.Format(
					time.RFC3339,
				),
			)
		}
	}
}

func (w *Worker) markTaskFailed(
	task Task,
	processingError error,
) {
	failureContext, cancelFailure :=
		context.WithTimeout(
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
}

func (w *Worker) releaseTaskForRetry(
	task Task,
	processingError error,
) {
	releaseContext, cancelRelease :=
		context.WithTimeout(
			context.Background(),
			5*time.Second,
		)

	err := w.repository.ReleaseForRetry(
		releaseContext,
		task,
		w.workerID,
		w.retryDelay,
		processingError,
	)

	cancelRelease()

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
