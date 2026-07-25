package task

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotificationStageNotImplemented = errors.New(
	"notification processing is not implemented yet",
)

type Processor interface {
	Process(ctx context.Context, task Task) error
}

type PlaceholderProcessor struct {
	processingDuration time.Duration
}

func NewPlaceholderProcessor(
	processingDuration time.Duration,
) (*PlaceholderProcessor, error) {
	if processingDuration <= 0 {
		return nil, fmt.Errorf(
			"processing duration must be greater than zero",
		)
	}

	return &PlaceholderProcessor{
		processingDuration: processingDuration,
	}, nil
}

func (p *PlaceholderProcessor) Process(
	ctx context.Context,
	_ Task,
) error {
	timer := time.NewTimer(p.processingDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)

	case <-timer.C:
		return ErrNotificationStageNotImplemented
	}
}
