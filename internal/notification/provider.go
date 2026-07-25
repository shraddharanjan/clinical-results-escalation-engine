package notification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTemporaryDelivery = errors.New(
		"temporary notification delivery failure",
	)

	ErrPermanentDelivery = errors.New(
		"permanent notification delivery failure",
	)
)

type FailureMode string

const (
	FailureModeSuccess      FailureMode = "success"
	FailureModeTemporary    FailureMode = "temporary"
	FailureModePermanent    FailureMode = "permanent"
	FailureModeLostResponse FailureMode = "lost_response"
)

type Provider interface {
	Send(
		ctx context.Context,
		message Message,
	) (Delivery, error)
}

type FakeProvider struct {
	pool        *pgxpool.Pool
	failureMode FailureMode
	latency     time.Duration
}

func NewFakeProvider(
	pool *pgxpool.Pool,
	failureMode FailureMode,
	latency time.Duration,
) (*FakeProvider, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}

	if latency < 0 {
		return nil, fmt.Errorf(
			"provider latency cannot be negative",
		)
	}

	switch failureMode {
	case FailureModeSuccess,
		FailureModeTemporary,
		FailureModePermanent,
		FailureModeLostResponse:
	default:
		return nil, fmt.Errorf(
			"unsupported fake provider failure mode %q",
			failureMode,
		)
	}

	return &FakeProvider{
		pool:        pool,
		failureMode: failureMode,
		latency:     latency,
	}, nil
}

func (p *FakeProvider) Send(
	ctx context.Context,
	message Message,
) (Delivery, error) {
	if p.latency > 0 {
		timer := time.NewTimer(p.latency)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return Delivery{}, context.Cause(ctx)

		case <-timer.C:
		}
	}

	switch p.failureMode {
	case FailureModeTemporary:
		return Delivery{}, fmt.Errorf(
			"%w: simulated provider outage",
			ErrTemporaryDelivery,
		)

	case FailureModePermanent:
		return Delivery{}, fmt.Errorf(
			"%w: simulated invalid recipient",
			ErrPermanentDelivery,
		)
	}

	delivery, inserted, err := p.acceptIdempotently(
		ctx,
		message,
	)
	if err != nil {
		return Delivery{}, err
	}

	if p.failureMode == FailureModeLostResponse && inserted {
		if err := p.markLostResponseSimulated(
			ctx,
			message.IdempotencyKey,
		); err != nil {
			return Delivery{}, err
		}

		return Delivery{}, fmt.Errorf(
			"%w: provider accepted notification but response was lost",
			ErrTemporaryDelivery,
		)
	}

	delivery.Deduplicated = !inserted

	return delivery, nil
}

func (p *FakeProvider) acceptIdempotently(
	ctx context.Context,
	message Message,
) (Delivery, bool, error) {
	const insertQuery = `
		INSERT INTO fake_provider_deliveries (
			idempotency_key,
			recipient,
			channel,
			message_body
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING
			provider_reference::text,
			accepted_at
	`

	var delivery Delivery

	err := p.pool.QueryRow(
		ctx,
		insertQuery,
		message.IdempotencyKey,
		message.Recipient,
		message.Channel,
		message.Body,
	).Scan(
		&delivery.ProviderReference,
		&delivery.AcceptedAt,
	)

	if err == nil {
		return delivery, true, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, false, fmt.Errorf(
			"insert fake provider delivery: %w",
			err,
		)
	}

	const selectQuery = `
		SELECT
			provider_reference::text,
			accepted_at
		FROM fake_provider_deliveries
		WHERE idempotency_key = $1
	`

	err = p.pool.QueryRow(
		ctx,
		selectQuery,
		message.IdempotencyKey,
	).Scan(
		&delivery.ProviderReference,
		&delivery.AcceptedAt,
	)
	if err != nil {
		return Delivery{}, false, fmt.Errorf(
			"load deduplicated provider delivery: %w",
			err,
		)
	}

	return delivery, false, nil
}

func (p *FakeProvider) markLostResponseSimulated(
	ctx context.Context,
	idempotencyKey string,
) error {
	const query = `
		UPDATE fake_provider_deliveries
		SET lost_response_simulated = TRUE
		WHERE idempotency_key = $1
	`

	_, err := p.pool.Exec(
		ctx,
		query,
		idempotencyKey,
	)
	if err != nil {
		return fmt.Errorf(
			"mark provider response as lost: %w",
			err,
		)
	}

	return nil
}
