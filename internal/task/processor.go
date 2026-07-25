package task

import (
	"context"
	"errors"
)

var ErrPermanentProcessing = errors.New(
	"permanent processing failure",
)

type Processor interface {
	Process(ctx context.Context, task Task) error
}
