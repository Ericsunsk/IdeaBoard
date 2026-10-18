package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/events"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type blockingWebhookPersistence struct {
	WebhookPersistence
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (p *blockingWebhookPersistence) CreateSubscription(context.Context, *WebhookSubscription) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		close(p.started)
		<-p.release
	}
	return nil
}

type fakeWebhookSender struct {
	status int
	err    error
	calls  int
	last   map[string]string
}

func (f *fakeWebhookSender) Send(_ context.Context, _ string, headers map[string]string, _ []byte) (int, error) {
	f.calls++
	f.last = headers
	if f.err != nil {
		return 0, f.err
	}
	return f.status, nil
}

// fakeWebhookEnqueuer captures River inserts for dispatch assertions.
// Tests drive retries by calling the worker directly with fabricated
// River jobs (the River-backed integration test covers real insert +
// redelivery against PostgreSQL).
type fakeWebhookEnqueuer struct {
	inserts []WebhookDeliveryArgs
	opts    []*river.InsertOpts
	err     error
}

func (f *fakeWebhookEnqueuer) Insert(_ context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	delivery, ok := args.(WebhookDeliveryArgs)
	if !ok {
		return nil, fmt.Errorf("fake enqueuer: unexpected args %T", args)
	}
	f.inserts = append(f.inserts, delivery)
	f.opts = append(f.opts, opts)
	return &rivertype.JobInsertResult{}, nil
}

// webhookJob fabricates the River job the worker would receive for a
// delivery attempt: River numbers attempts 1-based and caps them at
// the job MaxAttempts.
func webhookJob(args WebhookDeliveryArgs, attempt, maxAttempts int) *river.Job[WebhookDeliveryArgs] {
	return &river.Job[WebhookDeliveryArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: maxAttempts},
		Args:   args,
	}
}

func testEnvelope(t *testing.T) events.V1Envelope {
	t.Helper()
	envelope, err := events.MintV1Envelope(events.EventV1BillingSettled, "ws_1",
		events.V1Actor{Type: events.V1ActorUser, ID: "usr_actor"},
		events.V1Resource{Type: events.V1ResourceCheckoutSession, ID: "checkout_1"},
		events.V1BillingSettledData{ReferenceType: "checkout_session", ReferenceID: "checkout_1", SettlementState: "captured"})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestWebhookCreateMintsSecretOnce(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, nil)
	sub, minted, first, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(minted) == 0 || !first {
		t.Fatal("empty secret must be minted and returned once")
	}
	if !IsWebhookSubscriptionID(sub.ID) {
		t.Fatalf("subscription id %q must carry sub_ prefix", sub.ID)
	}
	views := store.List("ws_1")
	if len(views) != 1 || !views[0].HasSecret {
		t.Fatalf("list views=%v, want one masked view with has_secret", views)
	}
	// Provided secrets are never returned in any view.
	provided := FormatWhsecSecret([]byte("provided-secret-1234"))
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/y",
		[]string{events.EventV1BillingSettled}, []byte(provided)); err != nil {
		t.Fatal(err)
	}
	for _, view := range store.List("ws_1") {
		raw, _ := json.Marshal(view)
		if strings.Contains(string(raw), "provided-secret-1234") {
			t.Fatal("secret plaintext must never appear in reads")
		}
	}
}

func TestWebhookCreateReservesSubscriptionCapacityAcrossConcurrentPersistence(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, nil)
	for index := 0; index < webhookMaxSubscriptions-1; index++ {
		if _, _, _, err := store.Create(context.Background(), "ws_1", fmt.Sprintf("https://hooks.example.com/%d", index), nil, nil); err != nil {
			t.Fatalf("seed subscription %d: %v", index, err)
		}
	}
	persistence := &blockingWebhookPersistence{started: make(chan struct{}), release: make(chan struct{})}
	store.SetPersistence(persistence)
	var firstErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _, firstErr = store.Create(context.Background(), "ws_1", "https://hooks.example.com/first", nil, nil)
	}()
	select {
	case <-persistence.started:
	case <-time.After(time.Second):
		t.Fatal("first persistence call did not start")
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/second", nil, nil); err == nil {
		t.Fatal("concurrent create must observe the reserved subscription slot")
	}
	close(persistence.release)
	wg.Wait()
	if firstErr != nil {
		t.Fatalf("first create: %v", firstErr)
	}
}

func TestWebhookCreateRejectsBadURLAndFilter(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, nil)
	if _, _, _, err := store.Create(context.Background(), "ws_1", "http://hooks.example.com/x", nil, nil); err == nil {
		t.Fatal("plain http to public hosts must fail closed")
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x",
		[]string{"task.exploded"}, nil); err == nil {
		t.Fatal("unknown event-type filter must fail closed")
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, []byte("short")); err == nil {
		t.Fatal("short secrets must fail closed")
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "http://localhost:9/hook", nil, nil); err != nil {
		t.Fatalf("loopback http must be allowed for dev/tests: %v", err)
	}
}

func TestWebhookSignatureVerifyAndReject(t *testing.T) {
	secret := []byte(FormatWhsecSecret([]byte("test-secret-1234567890")))
	body := []byte(`{"id":"evt_1"}`)
	now := time.Now().UTC()
	headers := SignWebhook(secret, body, "evt_1", now)
	if err := VerifyWebhookSignature(secret, body, headers, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if got := headers["svix-id"]; got != "evt_1" {
		t.Fatalf("svix-id = %q, want evt_1", got)
	}
	if !strings.HasPrefix(headers["svix-signature"], "v1,") {
		t.Fatalf("svix-signature = %q, want v1,<base64>", headers["svix-signature"])
	}
	if err := VerifyWebhookSignature([]byte("wrong-secret-12345678"), body, headers, now); err == nil {
		t.Fatal("wrong secret must be rejected")
	}
	tampered := append([]byte(nil), body...)
	tampered = append(tampered, ' ')
	if err := VerifyWebhookSignature(secret, tampered, headers, now); err == nil {
		t.Fatal("tampered body must be rejected")
	}
	stale := SignWebhook(secret, body, "evt_1", now.Add(-time.Hour))
	if err := VerifyWebhookSignature(secret, body, stale, now); err == nil {
		t.Fatal("stale timestamp must be rejected")
	}
	future := SignWebhook(secret, body, "evt_1", now.Add(time.Hour))
	if err := VerifyWebhookSignature(secret, body, future, now); err == nil {
		t.Fatal("future timestamp must be rejected")
	}
	if err := VerifyWebhookSignature(secret, body, map[string]string{}, now); err == nil {
		t.Fatal("missing headers must be rejected")
	}
	// The delivery id is part of the signed payload: swapping it must
	// break verification even when the signature header is untouched.
	swapped := map[string]string{}
	for key, value := range headers {
		swapped[key] = value
	}
	swapped["svix-id"] = "evt_other0000000000"
	if err := VerifyWebhookSignature(secret, body, swapped, now); err == nil {
		t.Fatal("swapped delivery id must be rejected")
	}
}

// TestWebhookSvixOfficialVector pins the Svix known-answer vector from
// the svix-webhooks repo Go tests (svix-id msg_p5jXN8AQM9LWM0D4loKWxJek,
// timestamp 1614265330, payload {"test": 2432232314}): the signature
// encoding is base64, not hex.
func TestWebhookSvixOfficialVector(t *testing.T) {
	secret := []byte("whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")
	body := []byte(`{"test": 2432232314}`)
	at := time.Unix(1614265330, 0).UTC()
	headers := SignWebhook(secret, body, "msg_p5jXN8AQM9LWM0D4loKWxJek", at)
	if got := headers["svix-signature"]; got != "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=" {
		t.Fatalf("svix-signature = %q, want the official vector", got)
	}
	now := time.Unix(1614265330, 0).UTC()
	if err := VerifyWebhookSignature(secret, body, headers, now); err != nil {
		t.Fatalf("official vector rejected: %v", err)
	}
	// The whsec_ prefix selects base64 decoding: the same base64 text
	// WITHOUT the prefix is treated as literal key bytes (the legacy
	// raw-secret rule), so it must not verify the same delivery.
	if err := VerifyWebhookSignature([]byte("MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"), body, map[string]string{
		"svix-id":        "msg_p5jXN8AQM9LWM0D4loKWxJek",
		"svix-timestamp": "1614265330",
		"svix-signature": "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=",
	}, now); err == nil {
		t.Fatal("bare base64 key is not the HMAC key and must be rejected")
	}
}

// TestWebhookSvixRotation covers multi-signature rotation entries (any v1
// match wins; v2 tags are skipped).
func TestWebhookSvixRotation(t *testing.T) {
	secret := []byte("whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")
	body := []byte(`{"test": 2432232314}`)
	now := time.Unix(1614265330, 0).UTC()
	valid := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	rotated := map[string]string{
		"svix-id":        "msg_p5jXN8AQM9LWM0D4loKWxJek",
		"svix-timestamp": "1614265330",
		"svix-signature": "v1,Ceo5qEr07ixe2NLpvHk3FH9bwy/WavXrAFQ/9tdO6mc= v2,Ceo5qEr07ixe2NLpvHk3FH9bwy/WavXrAFQ/9tdO6mc= " + valid,
	}
	if err := VerifyWebhookSignature(secret, body, rotated, now); err != nil {
		t.Fatalf("rotation entries rejected: %v", err)
	}
	allBad := map[string]string{
		"svix-id":        "msg_p5jXN8AQM9LWM0D4loKWxJek",
		"svix-timestamp": "1614265330",
		"svix-signature": "v1,Ceo5qEr07ixe2NLpvHk3FH9bwy/WavXrAFQ/9tdO6mc= v2,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=",
	}
	if err := VerifyWebhookSignature(secret, body, allBad, now); err == nil {
		t.Fatal("all-bad rotation entries must be rejected")
	}
	emptyEntry := map[string]string{
		"svix-id":        "msg_p5jXN8AQM9LWM0D4loKWxJek",
		"svix-timestamp": "1614265330",
		"svix-signature": "v1,",
	}
	if err := VerifyWebhookSignature(secret, body, emptyEntry, now); err == nil {
		t.Fatal("empty v1 entry must be rejected")
	}
}

// TestWebhookTimestampToleranceBoundary pins the ±5 minute replay
// window: the boundary itself verifies, one second past it rejects.
func TestWebhookTimestampToleranceBoundary(t *testing.T) {
	secret := []byte(FormatWhsecSecret([]byte("test-secret-1234567890")))
	body := []byte(`{"id":"evt_1"}`)
	// Fixed whole-second clock: unix-second truncation would otherwise
	// push an exact-boundary stamp just past tolerance.
	now := time.Unix(1614265330, 0).UTC()
	edge := SignWebhook(secret, body, "evt_1", now.Add(-webhookTimestampTolerance))
	if err := VerifyWebhookSignature(secret, body, edge, now); err != nil {
		t.Fatalf("boundary timestamp rejected: %v", err)
	}
	past := SignWebhook(secret, body, "evt_1", now.Add(-webhookTimestampTolerance-time.Second))
	if err := VerifyWebhookSignature(secret, body, past, now); err == nil {
		t.Fatal("timestamp past tolerance must be rejected")
	}
	futurePast := SignWebhook(secret, body, "evt_1", now.Add(webhookTimestampTolerance+time.Second))
	if err := VerifyWebhookSignature(secret, body, futurePast, now); err == nil {
		t.Fatal("future timestamp past tolerance must be rejected")
	}
}

// TestWebhookWhsecSecretLifecycle covers the whsec_ secret form:
// minted secrets render as whsec_ once, provided whsec_ secrets are
// accepted, malformed whsec_ fails closed, and raw secrets are rejected.
func TestWebhookWhsecSecretLifecycle(t *testing.T) {
	store := NewWebhookStore(&fakeWebhookSender{status: 200}, nil)
	_, minted, first, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("minted secret must be reported once")
	}
	rendered := FormatWhsecSecret(minted)
	if !strings.HasPrefix(rendered, "whsec_") {
		t.Fatalf("minted secret renders as %q, want whsec_ form", rendered)
	}
	decoded, ok := ParseWhsecSecret([]byte(rendered))
	if !ok || len(decoded) != len(minted) {
		t.Fatal("whsec_ form must decode back to the stored key")
	}
	body := []byte(`{"id":"evt_1"}`)
	now := time.Now().UTC()
	signed := SignWebhook(minted, body, "evt_1", now)
	if err := VerifyWebhookSignature([]byte(rendered), body, signed, now); err != nil {
		t.Fatalf("whsec_ transport form must verify the same delivery: %v", err)
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/y",
		nil, []byte(rendered)); err != nil {
		t.Fatalf("provided whsec_ secret must be accepted: %v", err)
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/z",
		nil, []byte("whsec_not-valid-base64!!!")); err == nil {
		t.Fatal("malformed whsec_ secret must fail closed")
	}
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/raw", nil,
		[]byte("legacy-raw-secret-1234567890")); err == nil {
		t.Fatal("raw secrets must be rejected after the #832 hard cut")
	}
}

func TestWebhookRetryThenDeadLetter(t *testing.T) {
	sender := &fakeWebhookSender{err: fmt.Errorf("connection refused")}
	enqueuer := &fakeWebhookEnqueuer{}
	store := NewWebhookStore(sender, enqueuer)
	worker := NewWebhookDeliverWorker(store)
	sub, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t)
	store.Dispatch(context.Background(), envelope)
	if len(enqueuer.inserts) != 1 {
		t.Fatalf("dispatch enqueued %d jobs, want 1", len(enqueuer.inserts))
	}
	delivery := enqueuer.inserts[0]
	if delivery.SubscriptionID != sub.ID || delivery.DeliveryID != envelope.ID {
		t.Fatalf("enqueued args=%+v, want sub %s delivery %s", delivery, sub.ID, envelope.ID)
	}
	// River drives attempts 1..8 through the worker: transient failures
	// return an error (retry), the final one parks the DLQ entry and
	// still returns an error so River marks the job discarded (the
	// durable Postgres DLQ fact).
	for attempt := 1; attempt <= webhookMaxAttempts; attempt++ {
		err := worker.Work(context.Background(), webhookJob(delivery, attempt, webhookMaxAttempts))
		if err == nil {
			t.Fatalf("attempt %d must return an error for River retry/discard", attempt)
		}
	}
	if sender.calls != webhookMaxAttempts {
		t.Fatalf("calls=%d, want %d attempts before DLQ", sender.calls, webhookMaxAttempts)
	}
	got, ok := store.Get("ws_1", sub.ID)
	if !ok || got.Status != WebhookStatusDeadLetter {
		t.Fatalf("subscription status=%v ok=%v, want dead_letter", got, ok)
	}
	if len(got.DeadLetter) != 1 || got.DeadLetter[0].DeliveryID != envelope.ID {
		t.Fatalf("dead letter=%v, want the envelope parked", got.DeadLetter)
	}
	if len(got.Attempts) != webhookMaxAttempts {
		t.Fatalf("attempt log=%d, want %d entries", len(got.Attempts), webhookMaxAttempts)
	}
	// Redelivery after budget exhaustion parks nothing new and sends
	// nothing (the live entry is park-once; River discards the job).
	if err := worker.Work(context.Background(), webhookJob(delivery, webhookMaxAttempts, webhookMaxAttempts)); err != nil {
		t.Fatalf("post-DLQ redelivery must drop without error, got %v", err)
	}
	if sender.calls != webhookMaxAttempts {
		t.Fatalf("DLQ must stop sends, calls=%d", sender.calls)
	}
	parked, ok := store.Get("ws_1", sub.ID)
	if !ok || len(parked.DeadLetter) != 1 {
		t.Fatal("DLQ entry must be parked exactly once")
	}
}

func TestWebhookRetryDelayBackoff(t *testing.T) {
	if WebhookRetryDelay(1) != 0 {
		t.Fatal("attempt 1 needs no delay")
	}
	if WebhookRetryDelay(2) != webhookBackoffBase {
		t.Fatalf("attempt 2 delay=%v, want base %v", WebhookRetryDelay(2), webhookBackoffBase)
	}
	if WebhookRetryDelay(3) != 2*webhookBackoffBase {
		t.Fatalf("attempt 3 delay=%v, want 2x base", WebhookRetryDelay(3))
	}
	if WebhookRetryDelay(100) != webhookBackoffCap {
		t.Fatalf("delay must cap at %v, got %v", webhookBackoffCap, WebhookRetryDelay(100))
	}
}

func TestWebhookDuplicateDeliveryDedups(t *testing.T) {
	sender := &fakeWebhookSender{status: 200}
	enqueuer := &fakeWebhookEnqueuer{}
	store := NewWebhookStore(sender, enqueuer)
	worker := NewWebhookDeliverWorker(store)
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t)
	// A duplicate Dispatch before execution still enqueues twice here;
	// River's by-args uniqueness collapses the pair to one row (covered
	// by the River-backed test). The in-memory guard covers everything
	// after delivery: execute once, then redispatch.
	store.Dispatch(context.Background(), envelope)
	if len(enqueuer.inserts) != 1 {
		t.Fatalf("dispatch enqueued %d jobs, want 1", len(enqueuer.inserts))
	}
	if err := worker.Work(context.Background(), webhookJob(enqueuer.inserts[0], 1, webhookMaxAttempts)); err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("delivered %d times, want 1", sender.calls)
	}
	store.Dispatch(context.Background(), envelope)
	if len(enqueuer.inserts) != 1 {
		t.Fatalf("post-delivery redispatch enqueued, inserts=%d", len(enqueuer.inserts))
	}
	// A redelivered job for an already-delivered envelope drops without
	// sending (restart redelivery after success is a no-op).
	duplicate := enqueuer.inserts[0]
	if err := worker.Work(context.Background(), webhookJob(duplicate, 2, webhookMaxAttempts)); err != nil {
		t.Fatalf("duplicate execution must drop without error, got %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("duplicate envelope delivered %d times, want 1", sender.calls)
	}
}

func TestWebhookDelete(t *testing.T) {
	sender := &fakeWebhookSender{status: 200}
	enqueuer := &fakeWebhookEnqueuer{}
	store := NewWebhookStore(sender, enqueuer)
	filtered, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x",
		[]string{events.EventV1BillingSettled}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := store.Delete(context.Background(), "ws_1", filtered.ID); err != nil || !deleted {
		t.Fatal("delete must succeed")
	}
	if _, ok := store.Get("ws_1", filtered.ID); ok {
		t.Fatal("deleted subscription must be gone")
	}
	if deleted, err := store.Delete(context.Background(), "ws_1", filtered.ID); err != nil || deleted {
		t.Fatal("second delete must report missing")
	}
	if _, ok := store.Get("ws_2", filtered.ID); ok {
		t.Fatal("cross-workspace lookup must miss")
	}
}
