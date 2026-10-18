package webhook

// Webhook delivery on River (issue #833).
//
// Scheduling: Dispatch enqueues one webhook_deliver job per
// (subscription, envelope) delivery; River's native attempts drive the
// 8-attempt budget (riverpolicy.RiverMaxAttemptsWebhookDeliver) with a
// worker-level NextRetry that keeps the 30s-base doubling curve capped
// at 1h and adds +/-10% jitter (the custom ring had no jitter, so
// simultaneous failures retried in lockstep). The job args carry the
// full envelope so a restarted worker resumes without any DB lookup;
// the signing secret is never in args (the worker reads it from the
// store at execution time).
//
// Durability: River persists jobs and the events tables persist
// subscriptions, deliveries, attempt history, and dead-letter state.
// The process-local registry is rebuilt from PostgreSQL at startup.
//
// Deduplication: inserts are unique by args (subscription + delivery
// id), so a duplicate Dispatch while a job is pending/completed
// collapses to one row; the worker additionally drops deliveries
// already marked Delivered. NATS stays the event transport; River is
// the delivery queue. Enqueue happens post-commit and never fails
// the domain commit it follows.

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/riverpolicy"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// WebhookDeliveryJobKind is the River job kind for one webhook delivery.
const WebhookDeliveryJobKind = "webhook_deliver"

// webhookJitterFraction bounds the retry spread: +/-10% of the base
// delay, matching River's own default jitter width.
const webhookJitterFraction = 0.1

// WebhookDeliveryArgs is one River job: deliver envelope to one
// subscription. DeliveryID is the evt_ envelope id, stable across
// retries and also the Svix svix-id header value.
type WebhookDeliveryArgs struct {
	SubscriptionID string            `json:"subscription_id"`
	DeliveryID     string            `json:"delivery_id"`
	Envelope       events.V1Envelope `json:"envelope"`
}

// Kind implements river.JobArgs.
func (WebhookDeliveryArgs) Kind() string { return WebhookDeliveryJobKind }

// WebhookEnqueuer inserts delivery jobs. Production passes the shared
// River client (which satisfies this via Insert); tests inject fakes.
// The interface stays narrow so the events package never owns River
// execution (see the credits/payments architecture guards).
type WebhookEnqueuer interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// WebhookDeliveryInsertOpts routes one delivery onto the dedicated
// webhook queue with the 8-attempt budget. Unique by args collapses
// duplicate Dispatches of the same (subscription, delivery) pair.
func WebhookDeliveryInsertOpts(args WebhookDeliveryArgs) *river.InsertOpts {
	return &river.InsertOpts{
		Queue:       riverpolicy.RiverQueueWebhookDeliver,
		MaxAttempts: riverpolicy.RiverMaxAttemptsWebhookDeliver,
		Tags:        []string{args.SubscriptionID},
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// WebhookDeliverWorker executes delivery jobs against the store.
type WebhookDeliverWorker struct {
	river.WorkerDefaults[WebhookDeliveryArgs]
	store *WebhookStore
}

// NewWebhookDeliverWorker builds the worker over the shared store.
func NewWebhookDeliverWorker(store *WebhookStore) *WebhookDeliverWorker {
	return &WebhookDeliverWorker{store: store}
}

// Timeout pins the per-attempt budget: one signed POST under the
// webhookSendTimeout ceiling plus store bookkeeping.
func (*WebhookDeliverWorker) Timeout(*river.Job[WebhookDeliveryArgs]) time.Duration {
	return riverpolicy.RiverJobTimeoutWebhookDeliver
}

// NextRetry keeps the webhookBackoffBase doubling curve (webhookBackoffCap)
// and adds jitter so simultaneous failures spread instead of retrying in
// lockstep.
// job.Attempt is the just-failed 1-based attempt, so the delay before
// the next one is the curve value at Attempt+1.
func (w *WebhookDeliverWorker) NextRetry(job *river.Job[WebhookDeliveryArgs]) time.Time {
	attempt := 1
	if job != nil && job.Attempt > 0 {
		attempt = job.Attempt
	}
	base := WebhookRetryDelay(attempt + 1)
	if base <= 0 {
		return time.Now().UTC()
	}
	return time.Now().UTC().Add(webhookJitteredDelay(base, webhookJitterSample()))
}

// Work performs one delivery attempt. Nil means delivered or dropped
// (unknown/deleted subscription, dead-lettered subscription, or
// duplicate); a returned error asks River to retry (or discard once
// the budget is exhausted, after parking the live DLQ entry).
func (w *WebhookDeliverWorker) Work(ctx context.Context, job *river.Job[WebhookDeliveryArgs]) error {
	if w == nil || w.store == nil || job == nil {
		return fmt.Errorf("events: webhook deliver worker unavailable")
	}
	attempt := job.Attempt
	if attempt < 1 {
		attempt = 1
	}
	maxAttempts := webhookMaxAttempts
	if job.MaxAttempts > 0 {
		maxAttempts = job.MaxAttempts
	}
	return w.store.executeDeliveryAttempt(ctx, job.Args.SubscriptionID, job.Args.Envelope, attempt, maxAttempts)
}

// webhookJitterSample draws the signed jitter fraction in
// [-webhookJitterFraction, +webhookJitterFraction].
func webhookJitterSample() float64 {
	var sample [8]byte
	_, _ = rand.Read(sample[:])
	unit := float64(binary.LittleEndian.Uint64(sample[:])>>11) / (1 << 53)
	return unit*2*webhookJitterFraction - webhookJitterFraction
}

// webhookJitteredDelay applies a signed jitter fraction to base and
// clamps to [0, webhookBackoffCap]. The fraction argument keeps the
// curve unit-testable without sampling the RNG.
func webhookJitteredDelay(base time.Duration, jitter float64) time.Duration {
	if base <= 0 {
		return 0
	}
	delay := base + time.Duration(jitter*float64(base))
	if delay < 0 {
		return 0
	}
	if delay > webhookBackoffCap {
		return webhookBackoffCap
	}
	return delay
}
