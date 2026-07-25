package task

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeTaskClaimer struct {
	mu         sync.Mutex
	task       Task
	claimCount int
}

func (f *fakeTaskClaimer) ClaimOne(
	_ context.Context,
	workerID string,
	_ time.Duration,
) (Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.claimCount > 0 {
		return Task{}, ErrNoClaimableTask
	}

	f.claimCount++

	leaseOwner := workerID

	f.task = Task{
		ID:           uuid.New(),
		Status:       StatusProcessing,
		Severity:     "critical",
		AssignedTeam: "acute-medicine",
		LeaseOwner:   &leaseOwner,
		AttemptCount: 1,
		Version:      2,
	}

	return f.task, nil
}

func TestWorkerClaimsTask(t *testing.T) {
	t.Parallel()

	repository := &fakeTaskClaimer{}

	worker, err := NewWorker(
		repository,
		"test-worker",
		10*time.Millisecond,
		30*time.Second,
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		40*time.Millisecond,
	)
	defer cancel()

	if err := worker.Run(ctx); err != nil {
		t.Fatalf("run worker: %v", err)
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()

	if repository.claimCount != 1 {
		t.Fatalf(
			"expected one successful claim, got %d",
			repository.claimCount,
		)
	}

	if repository.task.LeaseOwner == nil {
		t.Fatal("expected task to have a lease owner")
	}

	if *repository.task.LeaseOwner != "test-worker" {
		t.Fatalf(
			"expected lease owner test-worker, got %s",
			*repository.task.LeaseOwner,
		)
	}
}
