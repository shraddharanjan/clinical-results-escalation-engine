package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	retryDelay time.Duration
}

func NewProcessor(
	repository AttemptStore,
	provider Provider,
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
		retryDelay: retryDelay,
	}, nil
}

func (p *Processor) Process(
	ctx context.Context,
	task clinicaltask.Task,
) error {
	if task.LeaseOwner == nil {
		return fmt.Errorf(
			"task %s has no lease owner",
			task.ID,
		)
	}

	workerID := *task.LeaseOwner

	recipient := task.AssignedTeam
	channel := "push"

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
		return fmt.Errorf(
			"get or create notification attempt: %w",
			err,
		)
	}

	if attempt.Status == StatusDelivered {
		delivery := Delivery{
			ProviderReference: valueOrEmpty(
				attempt.ProviderReference,
			),
			AcceptedAt:   valueOrNow(attempt.DeliveredAt),
			Deduplicated: true,
		}

		return p.repository.MarkDeliveredAndAwaitingAck(
			ctx,
			task,
			attempt,
			workerID,
			delivery,
		)
	}

	if err := p.repository.MarkRequested(
		ctx,
		attempt,
		workerID,
	); err != nil {
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
		nextAttemptAt := time.Now().UTC().Add(
			p.retryDelay,
		)

		recordError := p.repository.MarkTemporaryFailure(
			ctx,
			attempt,
			workerID,
			err,
			nextAttemptAt,
		)
		if recordError != nil {
			return fmt.Errorf(
				"record temporary notification failure: %w",
				recordError,
			)
		}

		return err
	}
	if errors.Is(err, ErrPermanentDelivery) {
		recordError := p.repository.MarkPermanentFailure(
			ctx,
			attempt,
			workerID,
			err,
		)
		if recordError != nil {
			return fmt.Errorf(
				"record permanent notification failure: %w",
				recordError,
			)
		}

		return fmt.Errorf(
			"%w: %v",
			clinicaltask.ErrPermanentProcessing,
			err,
		)
	}

	if err != nil {
		return fmt.Errorf(
			"send notification: %w",
			err,
		)
	}

	if err := p.repository.MarkDeliveredAndAwaitingAck(
		ctx,
		task,
		attempt,
		workerID,
		delivery,
	); err != nil {
		return err
	}

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
