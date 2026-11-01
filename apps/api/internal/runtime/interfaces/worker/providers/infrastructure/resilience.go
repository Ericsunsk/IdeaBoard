// Package providerinfra owns the sole AI egress transport. This file owns
// the process-shared per-model resilience guards: a per-model concurrency
// cap and a per-model circuit breaker (failure/slow-call fuse + half-open
// recovery). Both guards reject fail-closed with a retryable 429 before any
// network I/O so the worker requeues with River backoff instead of burning
// the upstream or stranding the task. Thresholds are code-owned conservative
// defaults overridable through WorkerResilienceConfig (env-backed in
// wiring); nil metrics keeps unit paths uninstrumented.
//
// Timeout layering (ADR-0010): Worker 300s per attempt (runtimeports.WorkerDefaultTimeout)
// nests inside the worker 360s execution budget
// (riverpolicy.WorkspaceExecutionBudget), which nests inside the River 17min
// job timeout (RiverJobTimeoutDrawing). This file adds no timeout of its own.
package providerinfra

import (
	"fmt"
	"strings"
	"sync"
	"time"

	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
)

// WorkerResilienceConfig tunes the per-model guards. Zero values take the
// conservative defaults; out-of-range values are clamped fail-closed to the
// nearest bound (never unbounded: a zero MaxConcurrentPerModel reads as the
// default, never unlimited).
type WorkerResilienceConfig struct {
	// MaxConcurrentPerModel caps in-flight Worker sends per model.
	MaxConcurrentPerModel int
	// WindowSize bounds the outcome ring per model (failure/slow rates are
	// computed over at most this many recent sends).
	WindowSize int
	// MinCalls gates the fuse: fewer recorded sends never open the breaker.
	MinCalls int
	// FailureRate opens the breaker at this windowed error/slow fraction
	// (0,1]. A single rate covers both failure and slow-call fuses: slow
	// sends (>= SlowCallThreshold) count toward the same windowed rate, so
	// no separate slow-rate knob can drift from the failure policy.
	FailureRate float64
	// SlowCallThreshold marks one send slow at this latency and above.
	SlowCallThreshold time.Duration
	// Cooldown bounds an open breaker before one half-open probe is admitted.
	Cooldown time.Duration
}

const (
	defaultResilienceMaxConcurrent = 8
	defaultResilienceWindowSize    = 32
	defaultResilienceMinCalls      = 20
	defaultResilienceFailureRate   = 0.5
	defaultResilienceSlowAfter     = 60 * time.Second
	defaultResilienceCooldown      = 30 * time.Second
)

const (
	maxResilienceMaxConcurrent = 64
	maxResilienceWindowSize    = 128
	maxResilienceCooldown      = 5 * time.Minute
)

// DefaultWorkerResilienceConfig returns the conservative code-owned
// defaults: a tight per-model cap, a 20-call fuse window at 50% errors/slow
// (>=60s) sends, and a 30s open before one half-open probe.
func DefaultWorkerResilienceConfig() WorkerResilienceConfig {
	return WorkerResilienceConfig{
		MaxConcurrentPerModel: defaultResilienceMaxConcurrent,
		WindowSize:            defaultResilienceWindowSize,
		MinCalls:              defaultResilienceMinCalls,
		FailureRate:           defaultResilienceFailureRate,
		SlowCallThreshold:     defaultResilienceSlowAfter,
		Cooldown:              defaultResilienceCooldown,
	}
}

// normalize clamps the config fail-closed: every field keeps an explicit
// bound so no combination yields unlimited concurrency or a never-open fuse.
func (c WorkerResilienceConfig) normalize() WorkerResilienceConfig {
	if c.MaxConcurrentPerModel <= 0 {
		c.MaxConcurrentPerModel = defaultResilienceMaxConcurrent
	}
	if c.MaxConcurrentPerModel > maxResilienceMaxConcurrent {
		c.MaxConcurrentPerModel = maxResilienceMaxConcurrent
	}
	if c.WindowSize <= 0 {
		c.WindowSize = defaultResilienceWindowSize
	}
	if c.WindowSize > maxResilienceWindowSize {
		c.WindowSize = maxResilienceWindowSize
	}
	if c.MinCalls <= 0 {
		c.MinCalls = defaultResilienceMinCalls
	}
	if c.MinCalls > c.WindowSize {
		c.MinCalls = c.WindowSize
	}
	if c.FailureRate <= 0 || c.FailureRate > 1 {
		c.FailureRate = defaultResilienceFailureRate
	}
	if c.SlowCallThreshold <= 0 {
		c.SlowCallThreshold = defaultResilienceSlowAfter
	}
	if c.Cooldown <= 0 {
		c.Cooldown = defaultResilienceCooldown
	}
	if c.Cooldown > maxResilienceCooldown {
		c.Cooldown = maxResilienceCooldown
	}
	return c
}

// WorkerResilience is the process-shared per-model guard set. One instance
// is built in composition wiring and injected into every WorkerClient so
// breaker windows and concurrency slots span executions; per-client
// instances would reset on every task and never fuse. It is safe for
// concurrent use; a nil receiver fails closed in checkAndAcquire (no
// unlimited path).
type WorkerResilience struct {
	cfg     WorkerResilienceConfig
	metrics *redmetrics.Set

	mu     sync.Mutex
	states map[string]*workerModelState
}

// workerModelState owns one model's in-flight count and windowed breaker.
// An open breaker admits exactly one probe after its cooldown.
type workerModelState struct {
	inFlight   int
	outcomes   []workerCallOutcome
	next       int
	openUntil  time.Time
	probing    bool
	generation uint64
}

type workerCallOutcome struct {
	failed bool
	slow   bool
}

// NewWorkerResilience builds the shared guard set. A nil metrics set keeps
// observations local (unit paths); production wires the RED set.
func NewWorkerResilience(cfg WorkerResilienceConfig, metrics *redmetrics.Set) *WorkerResilience {
	return &WorkerResilience{cfg: cfg.normalize(), metrics: metrics, states: make(map[string]*workerModelState)}
}

// WorkerFuse is the narrow resilience seam consumed by WorkerClient.Do:
// admit one send or reject fail-closed before any network I/O. The
// returned handle releases the slot exactly once. *WorkerResilience is the
// production implementation; tests inject a fake.
type WorkerFuse interface {
	checkAndAcquire(model string) (func(time.Duration, bool), error)
}

var _ WorkerFuse = (*WorkerResilience)(nil)

// normalizeWorkerResilienceModel keeps the guard key closed to
// admin-controlled values: empty reads as unknown so unmapped sends still
// share one bounded slot set instead of escaping the guards.
func normalizeWorkerResilienceModel(model string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(model)); trimmed != "" {
		return trimmed
	}
	return "unknown"
}

// stateFor returns the model's state, creating its semaphore on first use.
// Caller holds r.mu. Models come from the admin mapping/catalog, so the map
// stays bounded by the catalog size.
func (r *WorkerResilience) stateFor(model string) *workerModelState {
	key := normalizeWorkerResilienceModel(model)
	state, ok := r.states[key]
	if !ok {
		state = &workerModelState{outcomes: make([]workerCallOutcome, 0, r.cfg.WindowSize)}
		r.states[key] = state
	}
	return state
}

// workerResilienceReject is the retryable 429 the guards return before any
// network I/O. Callers wrap it in UpstreamHTTPError so worker classifiers
// converge to timeout/retryable and the task requeues with backoff.
type workerResilienceReject struct {
	reason string
	model  string
}

func (e *workerResilienceReject) Error() string {
	return fmt.Sprintf("worker %s for model %q", e.reason, e.model)
}

// checkAndAcquire admits one send and returns its completion handle. The handle
// releases the concurrency slot exactly once and records only its own breaker
// generation; sends admitted before a fuse cannot decide a later probe's result.
func (r *WorkerResilience) checkAndAcquire(model string) (func(time.Duration, bool), error) {
	key := normalizeWorkerResilienceModel(model)
	if r == nil {
		return nil, &workerResilienceReject{reason: "resilience not configured", model: key}
	}
	r.mu.Lock()
	state := r.stateFor(key)
	probe := !state.openUntil.IsZero()
	if probe && (time.Now().Before(state.openUntil) || state.probing) {
		r.mu.Unlock()
		r.metrics.IncWorkerPerModelLimited(key, redmetrics.WorkerLimitedBreakerOpen)
		return nil, &workerResilienceReject{reason: "breaker open", model: key}
	}
	if state.inFlight >= r.cfg.MaxConcurrentPerModel {
		r.mu.Unlock()
		r.metrics.IncWorkerPerModelLimited(key, redmetrics.WorkerLimitedConcurrency)
		return nil, &workerResilienceReject{reason: "per-model concurrency limit reached", model: key}
	}
	state.inFlight++
	state.probing = probe
	generation := state.generation
	r.mu.Unlock()

	var once sync.Once
	return func(latency time.Duration, failed bool) {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			state.inFlight--
			if generation != state.generation {
				return
			}
			outcome := workerCallOutcome{failed: failed, slow: latency >= r.cfg.SlowCallThreshold}
			if probe {
				state.probing = false
				state.generation++
				if outcome.failed || outcome.slow {
					state.openUntil = time.Now().Add(r.cfg.Cooldown)
					r.metrics.SetWorkerBreakerOpen(key, true)
					return
				}
				state.openUntil = time.Time{}
				state.outcomes = state.outcomes[:0]
				state.next = 0
				r.metrics.SetWorkerBreakerOpen(key, false)
				return
			}
			if len(state.outcomes) < r.cfg.WindowSize {
				state.outcomes = append(state.outcomes, outcome)
			} else {
				state.outcomes[state.next] = outcome
				state.next = (state.next + 1) % r.cfg.WindowSize
			}
			calls := len(state.outcomes)
			if calls < r.cfg.MinCalls {
				return
			}
			failures, slows := 0, 0
			for _, outcome := range state.outcomes {
				if outcome.failed {
					failures++
				}
				if outcome.slow {
					slows++
				}
			}
			if float64(failures)/float64(calls) >= r.cfg.FailureRate ||
				float64(slows)/float64(calls) >= r.cfg.FailureRate {
				state.generation++
				state.openUntil = time.Now().Add(r.cfg.Cooldown)
				r.metrics.SetWorkerBreakerOpen(key, true)
			}
		})
	}, nil
}
