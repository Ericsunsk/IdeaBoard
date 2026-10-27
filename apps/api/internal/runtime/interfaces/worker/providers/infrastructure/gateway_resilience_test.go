package providerinfra

import (
	"sync"
	"testing"
	"time"
)

func newTestGatewayResilience() *GatewayResilience {
	cfg := DefaultGatewayResilienceConfig()
	cfg.MinCalls = 1
	return NewGatewayResilience(cfg, nil)
}

func acquireGatewaySend(t *testing.T, guard *GatewayResilience) func(time.Duration, bool) {
	t.Helper()
	finish, err := guard.checkAndAcquire("test/model")
	if err != nil {
		t.Fatalf("acquire send: %v", err)
	}
	return finish
}

func expireGatewayCooldown(guard *GatewayResilience) {
	guard.mu.Lock()
	guard.states["test/model"].openUntil = time.Now().Add(-time.Second)
	guard.mu.Unlock()
}

func assertGatewaySendRejected(t *testing.T, guard *GatewayResilience) {
	t.Helper()
	if finish, err := guard.checkAndAcquire("test/model"); err == nil {
		finish(0, false)
		t.Fatal("send admitted while breaker must remain open")
	}
}

func TestGatewayResilienceOldSendCannotResolveHalfOpenProbe(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "old success", true: "old failure"}[failed], func(t *testing.T) {
			guard := newTestGatewayResilience()
			old := acquireGatewaySend(t, guard)
			trigger := acquireGatewaySend(t, guard)
			trigger(0, true)
			expireGatewayCooldown(guard)
			probe := acquireGatewaySend(t, guard)

			old(0, failed)
			assertGatewaySendRejected(t, guard)
			probe(0, false)
			next := acquireGatewaySend(t, guard)
			next(0, false)
		})
	}
}

func TestGatewayResilienceOldSendCannotPolluteRecoveredWindow(t *testing.T) {
	guard := newTestGatewayResilience()
	old := acquireGatewaySend(t, guard)
	trigger := acquireGatewaySend(t, guard)
	trigger(0, true)
	expireGatewayCooldown(guard)
	probe := acquireGatewaySend(t, guard)
	probe(0, false)

	old(guard.cfg.SlowCallThreshold, true)
	next := acquireGatewaySend(t, guard)
	next(0, false)
	if got := len(guard.states["test/model"].outcomes); got != 1 {
		t.Fatalf("recovered window has %d outcomes, want only the new generation's send", got)
	}
}

func TestGatewayResilienceProbeFailureAndSlownessReopen(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "slow", true: "failed"}[failed], func(t *testing.T) {
			guard := newTestGatewayResilience()
			trigger := acquireGatewaySend(t, guard)
			trigger(0, true)
			expireGatewayCooldown(guard)
			probe := acquireGatewaySend(t, guard)
			latency := time.Duration(0)
			if !failed {
				latency = guard.cfg.SlowCallThreshold
			}
			probe(latency, failed)
			assertGatewaySendRejected(t, guard)
			expireGatewayCooldown(guard)
			retryProbe := acquireGatewaySend(t, guard)
			retryProbe(0, false)
		})
	}
}

func TestGatewayResilienceCompletionReleasesSlotExactlyOnce(t *testing.T) {
	cfg := DefaultGatewayResilienceConfig()
	cfg.MaxConcurrentPerModel = 1
	guard := NewGatewayResilience(cfg, nil)
	finish := acquireGatewaySend(t, guard)
	assertGatewaySendRejected(t, guard)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() { finish(0, false) })
	}
	wg.Wait()
	next := acquireGatewaySend(t, guard)
	assertGatewaySendRejected(t, guard)
	next(0, false)
	if got := len(guard.states["test/model"].outcomes); got != 2 {
		t.Fatalf("window has %d outcomes, want each send recorded once", got)
	}
}

func TestGatewayResilienceNilFailsClosed(t *testing.T) {
	var guard *GatewayResilience
	if _, err := guard.checkAndAcquire("test/model"); err == nil {
		t.Fatal("nil guard admitted a send")
	}
}
