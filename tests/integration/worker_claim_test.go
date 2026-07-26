package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	clinicaltask "github.com/shraddharanjan/clinical-results-escalation-engine/internal/task"
)

func TestConcurrentWorkersClaimDifferentTasks(
	t *testing.T,
) {
	pool := openTestDatabase(t)

	const taskCount = 25

	ctx := context.Background()

	for index := 0; index < taskCount; index++ {
		resultID := uuid.New()
		taskID := uuid.New()

		_, err := pool.Exec(
			ctx,
			`
				INSERT INTO clinical_results (
					id,
					source_system,
					source_result_id,
					patient_reference,
					test_code,
					numeric_value,
					unit,
					reported_at,
					severity,
					matched_rule,
					raw_payload
				)
				VALUES (
					$1,
					'integration-test',
					$2,
					$3,
					'serum_potassium',
					6.8,
					'mmol/L',
					now(),
					'critical',
					'test-rule',
					'{}'::jsonb
				)
			`,
			resultID,
			fmt.Sprintf(
				"CONCURRENT-%03d",
				index,
			),
			fmt.Sprintf(
				"P-%03d",
				index,
			),
		)
		if err != nil {
			t.Fatalf(
				"insert test result: %v",
				err,
			)
		}

		_, err = pool.Exec(
			ctx,
			`
				INSERT INTO clinical_tasks (
					id,
					result_id,
					task_type,
					status,
					severity,
					assigned_team
				)
				VALUES (
					$1,
					$2,
					'review_clinical_result',
					'pending',
					'critical',
					'acute-medicine'
				)
			`,
			taskID,
			resultID,
		)
		if err != nil {
			t.Fatalf(
				"insert test task: %v",
				err,
			)
		}
	}

	repository := clinicaltask.NewPostgresRepository(
		pool,
	)

	var (
		waitGroup   sync.WaitGroup
		mutex       sync.Mutex
		claimed     = make(map[uuid.UUID]string)
		errorsFound []error
	)

	for workerNumber := 0; workerNumber < 10; workerNumber++ {
		waitGroup.Add(1)

		go func(number int) {
			defer waitGroup.Done()

			workerID := fmt.Sprintf(
				"worker-%d",
				number,
			)

			for {
				claim, err := repository.ClaimOne(
					context.Background(),
					workerID,
					30*time.Second,
				)

				if err == clinicaltask.ErrNoClaimableTask {
					return
				}

				if err != nil {
					mutex.Lock()
					errorsFound = append(
						errorsFound,
						err,
					)
					mutex.Unlock()

					return
				}

				mutex.Lock()

				if existingWorker, exists :=
					claimed[claim.Task.ID]; exists {
					errorsFound = append(
						errorsFound,
						fmt.Errorf(
							"task %s claimed by both %s and %s",
							claim.Task.ID,
							existingWorker,
							workerID,
						),
					)
				}

				claimed[claim.Task.ID] = workerID

				mutex.Unlock()
			}
		}(workerNumber)
	}

	waitGroup.Wait()

	if len(errorsFound) > 0 {
		t.Fatalf(
			"concurrent claim errors: %v",
			errorsFound,
		)
	}

	if len(claimed) != taskCount {
		t.Fatalf(
			"expected %d claimed tasks, got %d",
			taskCount,
			len(claimed),
		)
	}

	var duplicateClaimEvents int

	err := pool.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM (
				SELECT aggregate_id
				FROM audit_events
				WHERE event_type = 'task_claimed'
				GROUP BY aggregate_id
				HAVING COUNT(*) > 1
			) AS duplicates
		`,
	).Scan(&duplicateClaimEvents)
	if err != nil {
		t.Fatalf(
			"count duplicate claim events: %v",
			err,
		)
	}

	if duplicateClaimEvents != 0 {
		t.Fatalf(
			"expected no duplicate claims, got %d",
			duplicateClaimEvents,
		)
	}
}
