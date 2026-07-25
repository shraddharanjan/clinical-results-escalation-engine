package task

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeTaskStore struct {
	mu sync.Mutex

	claim         Claim
	claimReturned bool
	renewalCount  int
	releaseCount  int
	failedCount   int
	releasedError error
}

func (f *fakeTaskStore) ClaimOne(
	_ context.Context,
	workerID string,
	_ time.Duration,
) (Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.claimReturned {
		return Claim{}, ErrNoClaimableTask
	}

	f.claimReturned = true

	leaseOwner := workerID
	leaseExpiry := time.Now().
		Add(50 * time.Millisecond)

	f.claim = Claim{
		Task: Task{
			ID:             uuid.New(),
			Status:         StatusProcessing,
			Severity:       "critical",
			AssignedTeam:   "acute-medicine",
			LeaseOwner:     &leaseOwner,
			LeaseExpiresAt: &leaseExpiry,
			AttemptCount:   1,
			Version:        2,
		},
		PreviousStatus: StatusPending,
	}

	return f.claim, nil
}

func (f *fakeTaskStore) RenewLease(
	_ context.Context,
	_ string,
	_ string,
	_ time.Duration,
) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.renewalCount++

	return time.Now().
		Add(50 * time.Millisecond), nil
}

func (f *fakeTaskStore) ReleaseForRetry(
	_ context.Context,
	_ Task,
	_ string,
	_ time.Duration,
	processingError error,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.releaseCount++
	f.releasedError = processingError

	return nil
}

func (f *fakeTaskStore) MarkFailed(
	_ context.Context,
	_ Task,
	_ string,
	_ error,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.failedCount++

	return nil
}

type slowFailingProcessor struct {
	duration time.Duration
}

func (p slowFailingProcessor) Process(
	ctx context.Context,
	_ Task,
) error {
	timer := time.NewTimer(p.duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)

	case <-timer.C:
		return errors.New(
			"simulated processing failure",
		)
	}
}

func TestWorkerRenewsLeaseAndReleasesFailedTask(
	t *testing.T,
) {
	t.Parallel()

	store := &fakeTaskStore{}

	worker, err := NewWorker(
		store,
		slowFailingProcessor{
			duration: 35 * time.Millisecond,
		},
		nil,
		"test-worker",
		5*time.Millisecond,
		30*time.Millisecond,
		10*time.Millisecond,
		20*time.Millisecond,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		70*time.Millisecond,
	)
	defer cancel()

	if err := worker.Run(ctx); err != nil {
		t.Fatalf(
			"run worker: %v",
			err,
		)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if store.renewalCount == 0 {
		t.Fatal(
			"expected at least one lease renewal",
		)
	}

	if store.releaseCount != 1 {
		t.Fatalf(
			"expected one release, got %d",
			store.releaseCount,
		)
	}

	if store.releasedError == nil {
		t.Fatal(
			"expected processing error to be recorded",
		)
	}
}
