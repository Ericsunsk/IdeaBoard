package providerinfra

import (
	"sync"
	"testing"
	"time"
)

func newTestWorkerResilience() *WorkerResilience {
	cfg := DefaultWorkerResilienceConfig()
	cfg.MinCalls = 1
	return NewWorkerResilience(cfg, nil)
}

func acquireWorkerSend(t *testing.T, guard *WorkerResilience) func(time.Duration, bool) {
	t.Helper()
	finish, err := guard.checkAndAcquire("test/model")
	if err != nil {
		t.Fatalf("acquire send: %v", err)
	}
	return finish
}

func expireWorkerCooldown(guard *WorkerResilience) {
	guard.mu.Lock()
	guard.states["test/model"].openUntil = time.Now().Add(-time.Second)
	guard.mu.Unlock()
}

func assertWorkerSendRejected(t *testing.T, guard *WorkerResilience) {
	t.Helper()
	if finish, err := guard.checkAndAcquire("test/model"); err == nil {
		finish(0, false)
		t.Fatal("send admitted while breaker must remain open")
	}
}

func TestWorkerResilienceOldSendCannotResolveHalfOpenProbe(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "old success", true: "old failure"}[failed], func(t *testing.T) {
			guard := newTestWorkerResilience()
			old := acquireWorkerSend(t, guard)
			trigger := acquireWorkerSend(t, guard)
			trigger(0, true)
			expireWorkerCooldown(guard)
			probe := acquireWorkerSend(t, guard)

			old(0, failed)
			assertWorkerSendRejected(t, guard)
			probe(0, false)
			next := acquireWorkerSend(t, guard)
			next(0, false)
		})
	}
}

func TestWorkerResilienceOldSendCannotPolluteRecoveredWindow(t *testing.T) {
	guard := newTestWorkerResilience()
	old := acquireWorkerSend(t, guard)
	trigger := acquireWorkerSend(t, guard)
	trigger(0, true)
	expireWorkerCooldown(guard)
	probe := acquireWorkerSend(t, guard)
	probe(0, false)

	old(guard.cfg.SlowCallThreshold, true)
	next := acquireWorkerSend(t, guard)
	next(0, false)
	if got := len(guard.states["test/model"].outcomes); got != 1 {
		t.Fatalf("recovered window has %d outcomes, want only the new generation's send", got)
	}
}

func TestWorkerResilienceProbeFailureAndSlownessReopen(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "slow", true: "failed"}[failed], func(t *testing.T) {
			guard := newTestWorkerResilience()
			trigger := acquireWorkerSend(t, guard)
			trigger(0, true)
			expireWorkerCooldown(guard)
			probe := acquireWorkerSend(t, guard)
			latency := time.Duration(0)
			if !failed {
				latency = guard.cfg.SlowCallThreshold
			}
			probe(latency, failed)
			assertWorkerSendRejected(t, guard)
			expireWorkerCooldown(guard)
			retryProbe := acquireWorkerSend(t, guard)
			retryProbe(0, false)
		})
	}
}

func TestWorkerResilienceCompletionReleasesSlotExactlyOnce(t *testing.T) {
	cfg := DefaultWorkerResilienceConfig()
	cfg.MaxConcurrentPerModel = 1
	guard := NewWorkerResilience(cfg, nil)
	finish := acquireWorkerSend(t, guard)
	assertWorkerSendRejected(t, guard)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() { finish(0, false) })
	}
	wg.Wait()
	next := acquireWorkerSend(t, guard)
	assertWorkerSendRejected(t, guard)
	next(0, false)
	if got := len(guard.states["test/model"].outcomes); got != 2 {
		t.Fatalf("window has %d outcomes, want each send recorded once", got)
	}
}

func TestWorkerResilienceNilFailsClosed(t *testing.T) {
	var guard *WorkerResilience
	if _, err := guard.checkAndAcquire("test/model"); err == nil {
		t.Fatal("nil guard admitted a send")
	}
}
