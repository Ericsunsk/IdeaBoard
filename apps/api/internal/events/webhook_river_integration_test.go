package events_test

// River-backed webhook durability proof (issue #833). The external
// test package avoids the import cycle (workerriver owns events).
// Requires API_TEST_DATABASE_DSN; testdb SKIPs gracefully otherwise.

import (
	"context"
	"encoding/json"
	"testing"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/riverpolicy"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type riverProvenSender struct {
	calls  int
	last   map[string]string
	status int
}

func (f *riverProvenSender) Send(_ context.Context, _ string, headers map[string]string, _ []byte) (int, error) {
	f.calls++
	f.last = headers
	if f.status != 0 {
		return f.status, nil
	}
	return 200, nil
}

// TestWebhookRiverDurability proves the Postgres durability contract
// with a real River client: Dispatch persists the job row (survives
// client restarts), by-args uniqueness collapses duplicate Dispatches
// to one row, and the worker converges the redelivered job after the
// restart.
func TestWebhookRiverDurability(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	if err := workerriver.MigrateRiver(t.Context(), pool); err != nil {
		t.Fatalf("MigrateRiver() error = %v", err)
	}
	producer, err := workerriver.NewRiverProducer(pool)
	if err != nil {
		t.Fatalf("NewRiverProducer() error = %v", err)
	}
	sender := &riverProvenSender{}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO workspaces (public_id, owner_user_id, name)
VALUES ('ws_1', 1, 'webhook durability test')
ON CONFLICT (public_id) DO NOTHING`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	store := events.NewWebhookStore(sender, producer)
	store.SetPersistence(events.NewPostgresWebhookPersistence(pool))
	sub, _, _, err := store.Create(context.Background(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := events.MintV1Envelope(events.EventV1TaskCompleted, "ws_1",
		events.V1Actor{Type: events.V1ActorUser, ID: "usr_actor"},
		events.V1Resource{Type: events.V1ResourceTask, ID: "task_1"},
		events.V1TaskCompletedData{TaskID: "task_1", OutputsURI: "/api/v1/workspaces/ws_1/tasks/task_1/outputs"})
	if err != nil {
		t.Fatal(err)
	}

	before := countWebhookDeliverJobs(t, db)
	store.Dispatch(context.Background(), envelope)
	store.Dispatch(context.Background(), envelope)
	if got := countWebhookDeliverJobs(t, db); got != before+1 {
		t.Fatalf("job rows=%d, want %d: duplicate Dispatch must collapse to one unique row", got, before+1)
	}

	// Restart recovery: a fresh client and store on the same database
	// still see the pending delivery, and the worker converges it.
	if _, err := workerriver.NewRiverProducer(pool); err != nil {
		t.Fatalf("NewRiverProducer() after restart error = %v", err)
	}
	restarted := events.NewWebhookStore(sender, nil)
	restarted.SetPersistence(events.NewPostgresWebhookPersistence(pool))
	if err := restarted.LoadFromPersistence(t.Context()); err != nil {
		t.Fatalf("load subscriptions after restart: %v", err)
	}
	worker := events.NewWebhookDeliverWorker(restarted)
	var argsJSON []byte
	var attempt, maxAttempts int
	if err := pool.QueryRow(t.Context(), `
SELECT args, attempt, max_attempts FROM river_job
 WHERE kind = $1 ORDER BY id DESC LIMIT 1`, events.WebhookDeliveryJobKind).Scan(&argsJSON, &attempt, &maxAttempts); err != nil {
		t.Fatalf("read redelivered job row: %v", err)
	}
	var args events.WebhookDeliveryArgs
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		t.Fatalf("decode redelivered args: %v", err)
	}
	if args.SubscriptionID != sub.ID || args.DeliveryID != envelope.ID {
		t.Fatalf("redelivered args=%+v, want sub %s delivery %s", args, sub.ID, envelope.ID)
	}
	if maxAttempts != riverpolicy.RiverMaxAttemptsWebhookDeliver {
		t.Fatalf("row max_attempts=%d, want the 8-attempt budget", maxAttempts)
	}
	if attempt < 1 {
		attempt = 1
	}
	if err := worker.Work(t.Context(), &river.Job[events.WebhookDeliveryArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: maxAttempts},
		Args:   args,
	}); err != nil {
		t.Fatalf("Work() after restart error = %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("calls=%d, want 1 delivery after restart", sender.calls)
	}
	if got := sender.last["svix-id"]; got != envelope.ID {
		t.Fatalf("svix-id=%q, want the envelope id (signature payload requires it)", got)
	}
	got, _ := restarted.Get("ws_1", sub.ID)
	if _, delivered := got.Delivered[envelope.ID]; !delivered {
		t.Fatal("redelivered job must mark Delivered")
	}
	var status string
	var completedAt any
	var attempts int
	if err := pool.QueryRow(t.Context(), `
SELECT d.status, d.completed_at, d.attempt_count
FROM event_deliveries d
JOIN event_subscriptions s ON s.internal_id = d.subscription_id
JOIN workspace_events e ON e.internal_id = d.event_id
WHERE s.public_id = $1 AND e.public_id = $2`, sub.ID, envelope.ID).Scan(&status, &completedAt, &attempts); err != nil {
		t.Fatalf("read durable delivery state: %v", err)
	}
	if status != "delivered" || completedAt == nil || attempts != 1 {
		t.Fatalf("durable delivery state=(status=%q, completed_at=%v, attempts=%d), want delivered/complete/1", status, completedAt, attempts)
	}
	var outcome string
	if err := pool.QueryRow(t.Context(), `
SELECT a.outcome
FROM event_delivery_attempts a
JOIN event_deliveries d ON d.internal_id = a.delivery_id
JOIN event_subscriptions s ON s.internal_id = d.subscription_id
JOIN workspace_events e ON e.internal_id = d.event_id
WHERE s.public_id = $1 AND e.public_id = $2 AND a.attempt_number = 1`, sub.ID, envelope.ID).Scan(&outcome); err != nil {
		t.Fatalf("read durable attempt: %v", err)
	}
	if outcome != "delivered" {
		t.Fatalf("durable attempt outcome=%q, want delivered", outcome)
	}
	fresh := events.NewWebhookStore(&riverProvenSender{}, nil)
	fresh.SetPersistence(events.NewPostgresWebhookPersistence(pool))
	if err := fresh.LoadFromPersistence(t.Context()); err != nil {
		t.Fatalf("load subscriptions after restart: %v", err)
	}
	freshSub, ok := fresh.Get("ws_1", sub.ID)
	if !ok {
		t.Fatal("fresh store must load durable subscription")
	}
	if _, delivered := freshSub.Delivered[envelope.ID]; !delivered {
		t.Fatal("fresh store must retain delivered dedup state")
	}
	if deleted, err := fresh.Delete(t.Context(), "ws_1", sub.ID); err != nil || !deleted {
		t.Fatalf("delete durable subscription: deleted=%v err=%v", deleted, err)
	}
	afterDelete := events.NewWebhookStore(&riverProvenSender{}, nil)
	afterDelete.SetPersistence(events.NewPostgresWebhookPersistence(pool))
	if err := afterDelete.LoadFromPersistence(t.Context()); err != nil {
		t.Fatalf("load subscriptions after deletion: %v", err)
	}
	if _, ok := afterDelete.Get("ws_1", sub.ID); ok {
		t.Fatal("deleted subscription must not reappear after restart")
	}
}

func TestWebhookDeadLetterSurvivesRestart(t *testing.T) {
	secretstest.EnableEncryption(t)
	_, pool := testdb.OpenWithPool(t)
	if err := workerriver.MigrateRiver(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	producer, err := workerriver.NewRiverProducer(pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ('ws_1', 1, 'webhook DLQ test')`); err != nil {
		t.Fatal(err)
	}
	sender := &riverProvenSender{status: 500}
	store := events.NewWebhookStore(sender, producer)
	store.SetPersistence(events.NewPostgresWebhookPersistence(pool))
	sub, _, _, err := store.Create(t.Context(), "ws_1", "https://hooks.example.com/x", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := events.MintV1Envelope(events.EventV1TaskCompleted, "ws_1",
		events.V1Actor{Type: events.V1ActorUser, ID: "usr_actor"},
		events.V1Resource{Type: events.V1ResourceTask, ID: "task_1"},
		events.V1TaskCompletedData{TaskID: "task_1", OutputsURI: "/api/v1/workspaces/ws_1/tasks/task_1/outputs"})
	if err != nil {
		t.Fatal(err)
	}
	store.Dispatch(t.Context(), envelope)
	worker := events.NewWebhookDeliverWorker(store)
	args := events.WebhookDeliveryArgs{SubscriptionID: sub.ID, DeliveryID: envelope.ID, Envelope: envelope}
	for attempt := 1; attempt <= riverpolicy.RiverMaxAttemptsWebhookDeliver; attempt++ {
		job := &river.Job[events.WebhookDeliveryArgs]{
			JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: riverpolicy.RiverMaxAttemptsWebhookDeliver},
			Args:   args,
		}
		if err := worker.Work(t.Context(), job); err == nil {
			t.Fatalf("attempt %d must fail for River retry/discard", attempt)
		}
	}
	var status string
	var attempts int
	if err := pool.QueryRow(t.Context(), `
SELECT d.status, d.attempt_count FROM event_deliveries d
JOIN event_subscriptions s ON s.internal_id=d.subscription_id
JOIN workspace_events e ON e.internal_id=d.event_id
WHERE s.public_id=$1 AND e.public_id=$2`, sub.ID, envelope.ID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "dead_letter" || attempts != riverpolicy.RiverMaxAttemptsWebhookDeliver {
		t.Fatalf("durable delivery status=%q attempts=%d", status, attempts)
	}
	var attemptRows int
	if err := pool.QueryRow(t.Context(), `
SELECT COUNT(*) FROM event_delivery_attempts a
JOIN event_deliveries d ON d.internal_id=a.delivery_id
JOIN event_subscriptions s ON s.internal_id=d.subscription_id
WHERE s.public_id=$1`, sub.ID).Scan(&attemptRows); err != nil {
		t.Fatal(err)
	}
	if attemptRows != riverpolicy.RiverMaxAttemptsWebhookDeliver {
		t.Fatalf("durable attempt rows=%d, want %d", attemptRows, riverpolicy.RiverMaxAttemptsWebhookDeliver)
	}
	restarted := events.NewWebhookStore(sender, nil)
	restarted.SetPersistence(events.NewPostgresWebhookPersistence(pool))
	if err := restarted.LoadFromPersistence(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, ok := restarted.Get("ws_1", sub.ID)
	if !ok {
		t.Fatal("dead-letter subscription missing after restart")
	}
	if got.Status != events.WebhookStatusDeadLetter || len(got.DeadLetter) != 1 {
		t.Fatalf("reloaded subscription status=%q dead_letters=%d", got.Status, len(got.DeadLetter))
	}
}

func countWebhookDeliverJobs(t testing.TB, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM river_job WHERE kind = $1`,
		events.WebhookDeliveryJobKind).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
