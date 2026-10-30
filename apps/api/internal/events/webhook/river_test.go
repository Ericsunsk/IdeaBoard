package webhook

// Webhook River scheduling tests (issue #833): worker execution paths,
// jittered retry policy, insert routing, and the River-backed
// durability proof (enqueue persistence, by-args dedup, restart
// recovery) against PostgreSQL with graceful SKIP when no test DSN is
// configured.

import (
	"context"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/riverpolicy"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestWebhookDeliveryInsertOpts(t *testing.T) {
	envelope := testEnvelope(t)
	args := WebhookDeliveryArgs{SubscriptionID: "sub_abc", DeliveryID: envelope.ID, Envelope: envelope}
	opts := WebhookDeliveryInsertOpts(args)
	if opts.Queue != riverpolicy.RiverQueueWebhookDeliver {
		t.Fatalf("queue=%q, want %q", opts.Queue, riverpolicy.RiverQueueWebhookDeliver)
	}
	if riverpolicy.RiverMaxAttemptsWebhookDeliver != webhookMaxAttempts {
		t.Fatalf("matrix budget=%d, want the store budget %d",
			riverpolicy.RiverMaxAttemptsWebhookDeliver, webhookMaxAttempts)
	}
	if opts.MaxAttempts != riverpolicy.RiverMaxAttemptsWebhookDeliver {
		t.Fatalf("maxAttempts=%d, want the 8-attempt budget", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("inserts must be unique by args so duplicate Dispatches collapse")
	}
	if len(opts.Tags) != 1 || opts.Tags[0] != "sub_abc" {
		t.Fatalf("tags=%v, want the subscription id", opts.Tags)
	}
	if got := (WebhookDeliveryArgs{}).Kind(); got != WebhookDeliveryJobKind {
		t.Fatalf("kind=%q, want %q", got, WebhookDeliveryJobKind)
	}
}

func TestWebhookWorkerDeliversAndMarksDedup(t *testing.T) {
	sender := &fakeWebhookSender{status: 200}
	store := NewWebhookStore(sender, &fakeWebhookEnqueuer{})
	worker := NewWebhookDeliverWorker(store)
	sub, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t)
	args := WebhookDeliveryArgs{SubscriptionID: sub.ID, DeliveryID: envelope.ID, Envelope: envelope}
	if err := worker.Work(context.Background(), webhookJob(args, 1, webhookMaxAttempts)); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("calls=%d, want 1", sender.calls)
	}
	got, ok := store.Get("ws_1", sub.ID)
	if !ok {
		t.Fatal("subscription must resolve")
	}
	if _, delivered := got.Delivered[envelope.ID]; !delivered {
		t.Fatal("delivery must be marked Delivered")
	}
	if len(got.Attempts) != 1 || got.Attempts[0].Outcome != "delivered" {
		t.Fatalf("attempts=%v, want one delivered entry", got.Attempts)
	}
	if got.Status != WebhookStatusActive {
		t.Fatalf("status=%q, want active", got.Status)
	}
}

func TestWebhookWorkerDropsWithoutRetry(t *testing.T) {
	sender := &fakeWebhookSender{status: 200}
	store := NewWebhookStore(sender, &fakeWebhookEnqueuer{})
	worker := NewWebhookDeliverWorker(store)
	envelope := testEnvelope(t)
	dropped := []WebhookDeliveryArgs{
		{SubscriptionID: "sub_doesnotexist00", DeliveryID: envelope.ID, Envelope: envelope},
	}
	sub, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Deleted subscription: enqueue held the id, the worker finds nothing.
	deleted := WebhookDeliveryArgs{SubscriptionID: sub.ID, DeliveryID: envelope.ID, Envelope: envelope}
	if deleted, err := store.Delete(context.Background(), "ws_1", sub.ID); err != nil || !deleted {
		t.Fatal("delete must succeed")
	}
	dropped = append(dropped, deleted)
	for _, args := range dropped {
		if err := worker.Work(context.Background(), webhookJob(args, 3, webhookMaxAttempts)); err != nil {
			t.Fatalf("drop must return nil, got %v", err)
		}
	}
	if sender.calls != 0 {
		t.Fatalf("dropped deliveries sent %d times, want 0", sender.calls)
	}
	if err := worker.Work(context.Background(), nil); err == nil {
		t.Fatal("nil job must fail closed")
	}
	if err := NewWebhookDeliverWorker(nil).Work(context.Background(),
		webhookJob(dropped[0], 1, webhookMaxAttempts)); err == nil {
		t.Fatal("nil store must fail closed")
	}
}

func TestWebhookWorkerSurfacesSendErrorForRetry(t *testing.T) {
	sender := &fakeWebhookSender{status: 500}
	store := NewWebhookStore(sender, &fakeWebhookEnqueuer{})
	worker := NewWebhookDeliverWorker(store)
	sub, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t)
	args := WebhookDeliveryArgs{SubscriptionID: sub.ID, DeliveryID: envelope.ID, Envelope: envelope}
	if err := worker.Work(context.Background(), webhookJob(args, 1, webhookMaxAttempts)); err == nil {
		t.Fatal("transient failure must return an error for River retry")
	}
	got, _ := store.Get("ws_1", sub.ID)
	if got.Status != WebhookStatusDegraded {
		t.Fatalf("status=%q, want degraded after a failed attempt", got.Status)
	}
}

func TestWebhookRetryDelayWithJitterBounds(t *testing.T) {
	base := webhookBackoffBase
	for _, jitter := range []float64{-0.1, 0, 0.1, -1, 1} {
		got := webhookJitteredDelay(base, jitter)
		want := base + time.Duration(jitter*float64(base))
		if want < 0 {
			want = 0
		}
		if want > webhookBackoffCap {
			want = webhookBackoffCap
		}
		if got != want {
			t.Fatalf("jittered(%v, %v)=%v, want %v", base, jitter, got, want)
		}
	}
	// The sampled spread stays within +/-10% of the base curve.
	for i := 0; i < 100; i++ {
		got := webhookJitteredDelay(base, webhookJitterSample())
		if got < base-time.Duration(0.1*float64(base)) || got > base+time.Duration(0.1*float64(base)) {
			t.Fatalf("sampled delay %v outside +/-10%% of %v", got, base)
		}
	}
	if webhookJitteredDelay(0, 0.1) != 0 {
		t.Fatal("zero base must stay zero")
	}
	if got := webhookJitteredDelay(webhookBackoffCap, 1); got != webhookBackoffCap {
		t.Fatalf("jitter must clamp to the 1h cap, got %v", got)
	}
}

func TestWebhookWorkerNextRetryKeepsCurveWithJitter(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, &fakeWebhookEnqueuer{})
	worker := NewWebhookDeliverWorker(store)
	for attempt := 1; attempt < webhookMaxAttempts; attempt++ {
		before := time.Now().UTC()
		next := worker.NextRetry(&river.Job[WebhookDeliveryArgs]{
			JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: webhookMaxAttempts},
		})
		base := WebhookRetryDelay(attempt + 1)
		delay := next.Sub(before)
		low := base - time.Duration(webhookJitterFraction*float64(base)) - time.Second
		high := base + time.Duration(webhookJitterFraction*float64(base)) + time.Second
		if delay < low || delay > high {
			t.Fatalf("attempt %d next retry in %v, want within [%v, %v] of base %v",
				attempt, delay, low, high, base)
		}
	}
}

func TestWebhookDispatchWithoutEnqueuerDrops(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, nil)
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	// Must not panic: best-effort post-commit fan-out degrades to a
	// warning, mirroring the nil-EventBus seam.
	store.Dispatch(context.Background(), testEnvelope(t))
}

func TestWebhookDispatchEnqueueFailureDrops(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, &fakeWebhookEnqueuer{err: errors.New("river down")})
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	store.Dispatch(context.Background(), testEnvelope(t))
}

type fakeReconcilePersistence struct {
	pending []WebhookDeliveryArgs
	err     error
	called  bool
}

func (f *fakeReconcilePersistence) CreateSubscription(context.Context, *WebhookSubscription) error {
	return nil
}
func (f *fakeReconcilePersistence) DeleteSubscription(context.Context, string, string) error {
	return nil
}
func (f *fakeReconcilePersistence) LoadSubscriptions(context.Context) ([]*WebhookSubscription, error) {
	return nil, nil
}
func (f *fakeReconcilePersistence) LoadSubscription(context.Context, string) (*WebhookSubscription, error) {
	return nil, nil
}
func (f *fakeReconcilePersistence) EnsureDelivery(context.Context, events.V1Envelope, string) error {
	return nil
}
func (f *fakeReconcilePersistence) PendingDeliveries(context.Context) ([]WebhookDeliveryArgs, error) {
	f.called = true
	return f.pending, f.err
}
func (f *fakeReconcilePersistence) RecordAttempt(context.Context, string, events.V1Envelope, WebhookAttempt, bool) error {
	return nil
}
func (f *fakeReconcilePersistence) MarkDelivered(context.Context, string, events.V1Envelope) error {
	return nil
}

func TestWebhookReconcileArgs(t *testing.T) {
	args := WebhookReconcileArgs{}
	if got := args.Kind(); got != WebhookReconcileJobKind {
		t.Fatalf("kind=%q, want %q", got, WebhookReconcileJobKind)
	}
	if WebhookReconcileJobKind != "webhook_reconcile" {
		t.Fatalf("job kind=%q, want webhook_reconcile", WebhookReconcileJobKind)
	}
}

func TestWebhookReconcileWorker(t *testing.T) {
	ctx := context.Background()

	// Fail closed on nil worker or nil store.
	if err := (*WebhookReconcileWorker)(nil).Work(ctx, nil); err == nil {
		t.Fatal("nil worker must fail closed")
	}
	if err := NewWebhookReconcileWorker(nil).Work(ctx, nil); err == nil {
		t.Fatal("worker with nil store must fail closed")
	}

	enqueuer := &fakeWebhookEnqueuer{}
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, enqueuer)
	envelope := testEnvelope(t)
	pending := []WebhookDeliveryArgs{
		{SubscriptionID: "sub_rec_1", DeliveryID: envelope.ID, Envelope: envelope},
	}
	persistence := &fakeReconcilePersistence{pending: pending}
	store.SetPersistence(persistence)

	worker := NewWebhookReconcileWorker(store)
	if timeout := worker.Timeout(nil); timeout != riverpolicy.RiverJobTimeoutTaskSettlementSweep {
		t.Fatalf("timeout=%v, want %v", timeout, riverpolicy.RiverJobTimeoutTaskSettlementSweep)
	}

	if err := worker.Work(ctx, &river.Job[WebhookReconcileArgs]{}); err != nil {
		t.Fatalf("Work() unexpected error: %v", err)
	}
	if !persistence.called {
		t.Fatal("store.ReconcilePending was not called")
	}
	if len(enqueuer.inserts) != 1 {
		t.Fatalf("enqueued %d jobs, want 1", len(enqueuer.inserts))
	}
	if enqueuer.inserts[0].SubscriptionID != "sub_rec_1" {
		t.Fatalf("inserted sub=%q, want sub_rec_1", enqueuer.inserts[0].SubscriptionID)
	}

	// Persistence error propagates.
	persistenceErr := errors.New("db error")
	persistence.err = persistenceErr
	if err := worker.Work(ctx, &river.Job[WebhookReconcileArgs]{}); !errors.Is(err, persistenceErr) {
		t.Fatalf("Work() error = %v, want %v", err, persistenceErr)
	}
}

func TestWebhookReconciliationPeriodicJobs(t *testing.T) {
	jobs := WebhookReconciliationPeriodicJobs()
	if len(jobs) != 1 {
		t.Fatalf("periodic jobs = %d, want 1", len(jobs))
	}
	if jobs[0] == nil {
		t.Fatal("periodic job must not be nil")
	}

	args, opts := webhookReconcileInsert()
	if args.Kind() != WebhookReconcileJobKind {
		t.Fatalf("kind = %q, want %q", args.Kind(), WebhookReconcileJobKind)
	}
	if opts.Queue != riverpolicy.RiverQueueTaskSettlementSweep {
		t.Fatalf("queue = %q, want %q", opts.Queue, riverpolicy.RiverQueueTaskSettlementSweep)
	}
	if opts.MaxAttempts != riverpolicy.RiverMaxAttemptsTaskSettlementSweep {
		t.Fatalf("max attempts = %d, want %d", opts.MaxAttempts, riverpolicy.RiverMaxAttemptsTaskSettlementSweep)
	}
	if opts.Priority != 3 {
		t.Fatalf("priority = %d, want 3", opts.Priority)
	}

	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	if got := (webhookReconcileSchedule{}).Next(start); !got.Equal(start.Add(2 * time.Minute)) {
		t.Fatalf("next = %v, want %v", got, start.Add(2*time.Minute))
	}
	if webhookReconcilePeriod != 2*time.Minute {
		t.Fatalf("reconcile period = %v, want 2m", webhookReconcilePeriod)
	}
}
