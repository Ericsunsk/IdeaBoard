package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

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

type manualScheduler struct {
	pending []func()
}

func (m *manualScheduler) schedule(_ time.Duration, fn func()) {
	m.pending = append(m.pending, fn)
}

func (m *manualScheduler) runAll() {
	for len(m.pending) > 0 {
		next := m.pending
		m.pending = nil
		for _, fn := range next {
			fn()
		}
	}
}

func testEnvelope(t *testing.T) V1Envelope {
	t.Helper()
	envelope, err := MintV1Envelope(EventV1TaskCompleted, "ws_1",
		V1Actor{Type: V1ActorUser, ID: "usr_actor"},
		V1Resource{Type: V1ResourceTask, ID: "task_1"},
		V1TaskCompletedData{TaskID: "task_1", OutputsURI: "/api/v1/workspaces/ws_1/tasks/task_1/outputs"})
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
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/y",
		[]string{EventV1TaskCompleted}, []byte("provided-secret-1234")); err != nil {
		t.Fatal(err)
	}
	for _, view := range store.List("ws_1") {
		raw, _ := json.Marshal(view)
		if strings.Contains(string(raw), "provided-secret-1234") {
			t.Fatal("secret plaintext must never appear in reads")
		}
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
	secret := []byte("test-secret-1234567890")
	body := []byte(`{"id":"evt_1"}`)
	now := time.Now().UTC()
	headers := SignWebhook(secret, body, now)
	if err := VerifyWebhookSignature(secret, body, headers, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifyWebhookSignature([]byte("wrong-secret-12345678"), body, headers, now); err == nil {
		t.Fatal("wrong secret must be rejected")
	}
	tampered := append([]byte(nil), body...)
	tampered = append(tampered, ' ')
	if err := VerifyWebhookSignature(secret, tampered, headers, now); err == nil {
		t.Fatal("tampered body must be rejected")
	}
	stale := SignWebhook(secret, body, now.Add(-time.Hour))
	if err := VerifyWebhookSignature(secret, body, stale, now); err == nil {
		t.Fatal("stale timestamp must be rejected")
	}
	future := SignWebhook(secret, body, now.Add(time.Hour))
	if err := VerifyWebhookSignature(secret, body, future, now); err == nil {
		t.Fatal("future timestamp must be rejected")
	}
	if err := VerifyWebhookSignature(secret, body, map[string]string{}, now); err == nil {
		t.Fatal("missing headers must be rejected")
	}
}

func TestWebhookRetryThenDeadLetter(t *testing.T) {
	sender := &fakeWebhookSender{err: fmt.Errorf("connection refused")}
	scheduler := &manualScheduler{}
	store := NewWebhookStore(sender, scheduler.schedule)
	sub, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t)
	store.Dispatch(context.Background(), envelope)
	if sender.calls != 1 {
		t.Fatalf("first attempt must run inline, calls=%d", sender.calls)
	}
	for i := 0; i < webhookMaxAttempts-1; i++ {
		scheduler.runAll()
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
	// No further retries after DLQ.
	scheduler.runAll()
	if sender.calls != webhookMaxAttempts {
		t.Fatalf("DLQ must stop retries, calls=%d", sender.calls)
	}
}

func TestWebhookRetryDelayBackoff(t *testing.T) {
	if WebhookRetryDelay(1) != 0 {
		t.Fatal("attempt 1 needs no delay")
	}
	if WebhookRetryDelay(2) != 30*time.Second {
		t.Fatalf("attempt 2 delay=%v, want 30s", WebhookRetryDelay(2))
	}
	if WebhookRetryDelay(3) != time.Minute {
		t.Fatalf("attempt 3 delay=%v, want 60s", WebhookRetryDelay(3))
	}
	if WebhookRetryDelay(100) != time.Hour {
		t.Fatalf("delay must cap at 1h, got %v", WebhookRetryDelay(100))
	}
}

func TestWebhookDuplicateDeliveryDedups(t *testing.T) {
	sender := &fakeWebhookSender{status: 200}
	store := NewWebhookStore(sender, nil)
	if _, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil); err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t)
	store.Dispatch(context.Background(), envelope)
	store.Dispatch(context.Background(), envelope)
	if sender.calls != 1 {
		t.Fatalf("duplicate envelope delivered %d times, want 1", sender.calls)
	}
}

func TestWebhookFilterAndDelete(t *testing.T) {
	sender := &fakeWebhookSender{status: 200}
	store := NewWebhookStore(sender, nil)
	filtered, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x",
		[]string{EventV1TaskCreated}, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := testEnvelope(t) // task.completed: filtered out
	store.Dispatch(context.Background(), envelope)
	if sender.calls != 0 {
		t.Fatal("filtered-out type must not dispatch")
	}
	if !store.Delete("ws_1", filtered.ID) {
		t.Fatal("delete must succeed")
	}
	if _, ok := store.Get("ws_1", filtered.ID); ok {
		t.Fatal("deleted subscription must be gone")
	}
	if store.Delete("ws_1", filtered.ID) {
		t.Fatal("second delete must report missing")
	}
	if _, ok := store.Get("ws_2", filtered.ID); ok {
		t.Fatal("cross-workspace lookup must miss")
	}
}
