package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/shraddharanjan/clinical-results-escalation-engine/internal/platform/telemetry"
	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

type AttemptStore interface {
	GetOrCreateAttempt(
		ctx context.Context,
		task clinicaltask.Task,
		recipient string,
		channel string,
		idempotencyKey string,
	) (Attempt, error)

	MarkRequested(
		ctx context.Context,
		attempt Attempt,
		workerID string,
	) error

	MarkTemporaryFailure(
		ctx context.Context,
		attempt Attempt,
		workerID string,
		deliveryError error,
		nextAttemptAt time.Time,
	) error

	MarkPermanentFailure(
		ctx context.Context,
		attempt Attempt,
		workerID string,
		deliveryError error,
	) error

	MarkDeliveredAndAwaitingAck(
		ctx context.Context,
		task clinicaltask.Task,
		attempt Attempt,
		workerID string,
		delivery Delivery,
	) error
}

type Processor struct {
	repository AttemptStore
	provider   Provider
	metrics    *telemetry.Metrics
	retryDelay time.Duration
}

func NewProcessor(
	repository AttemptStore,
	provider Provider,
	metrics *telemetry.Metrics,
	retryDelay time.Duration,
) (*Processor, error) {
	if repository == nil {
		return nil, fmt.Errorf(
			"notification repository is required",
		)
	}

	if provider == nil {
		return nil, fmt.Errorf(
			"notification provider is required",
		)
	}

	if retryDelay < 0 {
		return nil, fmt.Errorf(
			"notification retry delay cannot be negative",
		)
	}

	return &Processor{
		repository: repository,
		provider:   provider,
		metrics:    metrics,
		retryDelay: retryDelay,
	}, nil
}

func (p *Processor) Process(
	ctx context.Context,
	task clinicaltask.Task,
) error {
	tracer := otel.Tracer(
		"clinical-results-escalation-engine/notification",
	)

	ctx, span := tracer.Start(
		ctx,
		"notification.process",
	)
	defer span.End()

	if task.LeaseOwner == nil {
		err := fmt.Errorf(
			"task %s has no lease owner",
			task.ID,
		)

		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			err.Error(),
		)

		return err
	}

	workerID := *task.LeaseOwner
	recipient := task.AssignedTeam
	channel := "push"

	span.SetAttributes(
		attribute.String(
			"clinical.task.id",
			task.ID.String(),
		),
		attribute.String(
			"clinical.task.severity",
			task.Severity,
		),
		attribute.Int(
			"clinical.task.escalation_level",
			task.EscalationLevel,
		),
		attribute.String(
			"notification.channel",
			channel,
		),
		attribute.String(
			"notification.recipient_type",
			"clinical_team",
		),
	)

	idempotencyKey := fmt.Sprintf(
		"task:%s:level:%d:recipient:%s:channel:%s",
		task.ID,
		task.EscalationLevel,
		recipient,
		channel,
	)

	attempt, err := p.repository.GetOrCreateAttempt(
		ctx,
		task,
		recipient,
		channel,
		idempotencyKey,
	)
	if err != nil {
		wrappedError := fmt.Errorf(
			"get or create notification attempt: %w",
			err,
		)

		span.RecordError(wrappedError)
		span.SetStatus(
			codes.Error,
			wrappedError.Error(),
		)

		return wrappedError
	}

	span.SetAttributes(
		attribute.Int(
			"notification.attempt_count",
			attempt.AttemptCount,
		),
	)

	if attempt.Status == StatusDelivered {
		delivery := Delivery{
			ProviderReference: valueOrEmpty(
				attempt.ProviderReference,
			),
			AcceptedAt: valueOrNow(
				attempt.DeliveredAt,
			),
			Deduplicated: true,
		}

		if err := p.repository.
			MarkDeliveredAndAwaitingAck(
				ctx,
				task,
				attempt,
				workerID,
				delivery,
			); err != nil {
			span.RecordError(err)
			span.SetStatus(
				codes.Error,
				err.Error(),
			)

			return err
		}

		p.metrics.RecordNotification(
			ctx,
			string(StatusDelivered),
			channel,
		)

		span.SetAttributes(
			attribute.Bool(
				"notification.deduplicated",
				true,
			),
		)

		span.SetStatus(
			codes.Ok,
			"previous delivery reconciled",
		)

		return nil
	}

	if err := p.repository.MarkRequested(
		ctx,
		attempt,
		workerID,
	); err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			err.Error(),
		)

		return err
	}

	message := Message{
		TaskID:         task.ID,
		IdempotencyKey: idempotencyKey,
		Recipient:      recipient,
		Channel:        channel,
		Body: fmt.Sprintf(
			"Synthetic %s clinical result requires acknowledgement.",
			task.Severity,
		),
	}

	delivery, err := p.provider.Send(
		ctx,
		message,
	)

	if errors.Is(err, ErrTemporaryDelivery) {
		nextAttemptAt := time.Now().
			UTC().
			Add(p.retryDelay)

		recordError :=
			p.repository.MarkTemporaryFailure(
				ctx,
				attempt,
				workerID,
				err,
				nextAttemptAt,
			)

		if recordError != nil {
			wrappedError := fmt.Errorf(
				"record temporary notification failure: %w",
				recordError,
			)

			span.RecordError(wrappedError)
			span.SetStatus(
				codes.Error,
				wrappedError.Error(),
			)

			return wrappedError
		}

		p.metrics.RecordNotification(
			ctx,
			string(StatusTemporaryFailed),
			channel,
		)

		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			"temporary notification failure",
		)

		return err
	}

	if errors.Is(err, ErrPermanentDelivery) {
		recordError :=
			p.repository.MarkPermanentFailure(
				ctx,
				attempt,
				workerID,
				err,
			)

		if recordError != nil {
			wrappedError := fmt.Errorf(
				"record permanent notification failure: %w",
				recordError,
			)

			span.RecordError(wrappedError)
			span.SetStatus(
				codes.Error,
				wrappedError.Error(),
			)

			return wrappedError
		}

		p.metrics.RecordNotification(
			ctx,
			string(StatusPermanentFailed),
			channel,
		)

		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			"permanent notification failure",
		)

		return fmt.Errorf(
			"%w: %v",
			clinicaltask.ErrPermanentProcessing,
			err,
		)
	}

	if err != nil {
		wrappedError := fmt.Errorf(
			"send notification: %w",
			err,
		)

		span.RecordError(wrappedError)
		span.SetStatus(
			codes.Error,
			wrappedError.Error(),
		)

		return wrappedError
	}

	if err := p.repository.
		MarkDeliveredAndAwaitingAck(
			ctx,
			task,
			attempt,
			workerID,
			delivery,
		); err != nil {
		span.RecordError(err)
		span.SetStatus(
			codes.Error,
			err.Error(),
		)

		return err
	}

	p.metrics.RecordNotification(
		ctx,
		string(StatusDelivered),
		channel,
	)

	span.SetAttributes(
		attribute.Bool(
			"notification.deduplicated",
			delivery.Deduplicated,
		),
	)

	span.SetStatus(
		codes.Ok,
		"notification delivered",
	)

	return nil
}

func valueOrEmpty(
	value *string,
) string {
	if value == nil {
		return ""
	}

	return *value
}

func valueOrNow(
	value *time.Time,
) time.Time {
	if value == nil {
		return time.Now().UTC()
	}

	return *value
}
