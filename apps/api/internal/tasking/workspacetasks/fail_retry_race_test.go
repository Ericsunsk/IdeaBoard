package workspacetasks_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// TestFailRetryRaceKeepsTerminalClosed pins P1-4: Fail (running/pending ->
// failed + Release) and Retry (live -> pending + requeue) race on the same
// row lock; Retry can commit before Fail, or lose with ErrStateConflict.
// A released task never revives to pending.
func TestFailRetryRaceKeepsTerminalClosed(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()

	// Phase 1: concurrent Fail vs Retry from the same running lease.
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	_ = env.service.StagedEvents()

	// Each concurrent caller owns its staged-event buffer; both services
	// still contend on the same task row and settlement dependencies.
	failService := workspacetasks.NewService(env.serviceDeps)
	retryService := workspacetasks.NewService(env.serviceDeps)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var failErr, retryErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, failErr = failService.Fail(ctx, env.ws, created.ID, "race fail")
	}()
	go func() {
		defer wg.Done()
		<-start
		_, retryErr = retryService.Retry(ctx, env.ws, created.ID, true)
	}()
	close(start)
	wg.Wait()

	wins := 0
	for _, operation := range []struct {
		name    string
		service *workspacetasks.Service
		err     error
		status  workspacetasks.Status
	}{
		{"Fail", failService, failErr, workspacetasks.StatusFailed},
		{"Retry", retryService, retryErr, workspacetasks.StatusPending},
	} {
		if operation.err == nil {
			wins++
			assertSingleStagedStatus(t, operation.service, env, created.ID, operation.status)
			continue
		}
		if !errors.Is(operation.err, workspacetasks.ErrStateConflict) {
			t.Fatalf("%s race err = %v, want nil or ErrStateConflict", operation.name, operation.err)
		}
		if staged := operation.service.StagedEvents(); len(staged) != 0 {
			t.Fatalf("conflicting %s staged events: %+v", operation.name, staged)
		}
	}
	// Both orders converge to failed without revival:
	// - Retry first (running->pending) then Fail (pending->failed): wins=2.
	// - Fail first (running->failed) then Retry blocked: wins=1.
	// A released task must never revive to pending, so the final state
	// is always failed regardless of the interleaving.
	if wins < 1 || wins > 2 {
		t.Fatalf("race wins = %d, want 1 or 2", wins)
	}
	stored, err := env.store.GetTask(ctx, env.ws, created.ID)
	if err != nil {
		t.Fatalf("GetTask after race: %v", err)
	}

	// Phase 2: released-then-retry is closed. If Fail won, the task is
	// failed with its hold released and Retry must conflict, never revive
	// to pending. If Retry won, Fail the requeued task first to reach the
	// same released state, then prove the terminal stays closed.
	if stored.IsTerminal() {
		if stored.Status != workspacetasks.StatusFailed {
			t.Fatalf("terminal status = %q, want failed (Fail won the race)", stored.Status)
		}
		if _, err := env.service.Retry(ctx, env.ws, created.ID, true); !errors.Is(err, workspacetasks.ErrStateConflict) {
			t.Fatalf("Retry after released err = %v, want ErrStateConflict (no pending revival)", err)
		}
		after, err := env.store.GetTask(ctx, env.ws, created.ID)
		if err != nil {
			t.Fatalf("GetTask after blocked retry: %v", err)
		}
		if after.Status != workspacetasks.StatusFailed {
			t.Fatalf("status after blocked retry = %q, want still failed", after.Status)
		}
		return
	}
	// Retry won: pending with the hold intact. Converge it to released
	// through Fail, then prove the terminal stays closed.
	if stored.Status != workspacetasks.StatusPending {
		t.Fatalf("live status = %q, want pending (Retry won the race)", stored.Status)
	}
	if _, err := env.service.Fail(ctx, env.ws, created.ID, "converge after retry win"); err != nil {
		t.Fatalf("Fail after retry win: %v", err)
	}
	_ = env.service.StagedEvents()
	if _, err := env.service.Retry(ctx, env.ws, created.ID, true); !errors.Is(err, workspacetasks.ErrStateConflict) {
		t.Fatalf("Retry after released err = %v, want ErrStateConflict", err)
	}
}
