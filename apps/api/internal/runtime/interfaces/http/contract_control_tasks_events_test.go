package httpapi

// Control task status announcements (admin cancel dead-channel fix).
//
// The defect these pin: an operator cancel settled the task but its
// staged status event was never drained, so `task.cancelled` reached
// neither the task status channel nor the workspace envelope. Both
// delivery channels are asserted with COUNTS, not just presence: the
// failure mode being guarded is a silent no-op, and "at least once" would
// happily pass a double-send.
//
// The port is deliberately a recording fake declared HERE, in the Control
// Plane's own package, with no dependency on the event layer. That is the
// point of the neutral port: the delivery mechanism is a composition-time
// choice, so "did the operator cancel announce exactly once" is provable
// without a bus, a hub, or a webhook endpoint.
//
// These are PostgreSQL integration tests over the real settlement legs
// (cancel flips the row and releases the hold inside ONE transaction).
// They skip without API_TEST_DATABASE_DSN like the other postgres
// integration tests.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"ideaboard/api/internal/billing"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	creditsports "ideaboard/api/internal/credits/ports"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// recordingControlTaskEvents is the fake ControlTaskEventPort. It records
// both channels separately because they are two distinct deliveries, and
// it can be made to fail on either so an announcement error can be proven
// not to break the settlement.
type recordingControlTaskEvents struct {
	mu          sync.Mutex
	statuses    []ControlTaskStatusEvent
	envelopes   []ControlTaskStatusEvent
	statusErr   error
	envelopeErr error
}

func (r *recordingControlTaskEvents) PublishTaskStatus(_ context.Context, event ControlTaskStatusEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, event)
	return r.statusErr
}

func (r *recordingControlTaskEvents) EmitWorkspaceTaskEvent(_ context.Context, event ControlTaskStatusEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.envelopes = append(r.envelopes, event)
	return r.envelopeErr
}

func (r *recordingControlTaskEvents) snapshot() ([]ControlTaskStatusEvent, []ControlTaskStatusEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ControlTaskStatusEvent(nil), r.statuses...),
		append([]ControlTaskStatusEvent(nil), r.envelopes...)
}

// controlTxHolder adapts the real Credits service to both settlement
// seams and can fail ReleaseInTx on demand to prove a rolled-back cancel
// announces nothing.
type controlTxHolder struct {
	credits     *credits.Service
	mu          sync.Mutex
	failRelease error
}

var (
	_ workspacetasks.Holder   = (*controlTxHolder)(nil)
	_ workspacetasks.TxHolder = (*controlTxHolder)(nil)
)

func (h *controlTxHolder) releaseFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failRelease
}

// Hold/Capture/Release must never run: the production store exposes the
// PostgreSQL settlement seams, so cancel settles inside one transaction.
func (h *controlTxHolder) Hold(context.Context, int64, int64, string, string) (string, error) {
	return "", errors.New("controlTxHolder.Hold must not run on the single-tx path")
}

func (h *controlTxHolder) Capture(context.Context, string, int64, string) error {
	return errors.New("controlTxHolder.Capture must not run on the single-tx path")
}

func (h *controlTxHolder) Release(context.Context, string, int64, string) error {
	return errors.New("controlTxHolder.Release must not run on the single-tx path")
}

func (h *controlTxHolder) HoldInTx(context.Context, pgx.Tx, int64, int64, string, string) (string, error) {
	return "", errors.New("controlTxHolder.HoldInTx must not run on cancel")
}

func (h *controlTxHolder) CaptureInTx(context.Context, pgx.Tx, int64, string, int64, string) (string, error) {
	return "", errors.New("controlTxHolder.CaptureInTx must not run on cancel")
}

func (h *controlTxHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, userID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := h.releaseFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.InTx(tx).Release(ctx, creditsports.ReleaseCommand{
		UserID: userID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "control-cancel-release:" + taskID,
			Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
			Source:            "task_submission",
			Reason:            "control admin cancel release",
			BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
		},
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

// controlOperatorDirectory resolves exactly one operator, so the envelope
// actor assertion is deterministic and an unknown operator degrades the
// way production does.
type controlOperatorDirectory struct {
	userID   int64
	publicID uuid.UUID
}

var _ ControlCreditDirectory = controlOperatorDirectory{}

func (d controlOperatorDirectory) ResolvePublicID(_ context.Context, publicID string) (credithttp.UserIdentity, error) {
	if publicID != tenancy.FormatUserPublicID(d.publicID) {
		return credithttp.UserIdentity{}, errors.New("unknown operator")
	}
	return credithttp.UserIdentity{ID: d.userID, PublicID: d.publicID}, nil
}

func (d controlOperatorDirectory) ResolveID(_ context.Context, id int64) (credithttp.UserIdentity, error) {
	if id != d.userID {
		return credithttp.UserIdentity{}, errors.New("unknown operator")
	}
	return credithttp.UserIdentity{ID: d.userID, PublicID: d.publicID}, nil
}

// controlCancelEnv is one isolated fixture database: a workspace, a
// wallet-funded owner, and the seeds an operator cancel needs.
type controlCancelEnv struct {
	pool      *pgxpool.Pool
	workspace string
	userID    int64
	billingID int64
	// priceVersionID/priceVersion identify the published render price the
	// seeded billing snapshot references (composite FK).
	priceVersionID int64
	priceVersion   int64
}

func openControlCancelEnv(t *testing.T) *controlCancelEnv {
	t.Helper()
	pool := openControlTaskTestDB(t)
	workspace, userID, _, billingID := seedControlTaskWorkspace(t, pool)
	env := &controlCancelEnv{pool: pool, workspace: workspace, userID: userID, billingID: billingID}
	env.publishFixturePrice(t)
	env.priceVersionID, env.priceVersion = env.readPriceVersion(t)
	return env
}

// creditServiceFor provisions a wallet and grants credits so the fixture can
// take a real hold. The hold itself is taken per task by holdForTask, because
// the billing-snapshot trigger requires the hold's business reference to name
// that exact task.
func (e *controlCancelEnv) creditServiceFor(t *testing.T) *credits.Service {
	t.Helper()
	ctx := context.Background()
	service := credits.NewService(e.pool)
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin wallet tx: %v", err)
	}
	if err := service.LifecycleInTx(tx).ProvisionWallet(ctx, e.userID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision wallet: %v", err)
	}
	grant, err := domain.NewCreditAmount(1000)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant amount: %v", err)
	}
	if _, err := service.InTx(tx).Grant(ctx, creditsports.GrantCommand{
		UserID: e.userID, Amount: grant,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "control-cancel-grant-" + randomControlHex(t, 6),
			Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"},
			Source:         "test", Reason: "control cancel fixture",
			BusinessReference: domain.BusinessReference{Type: "test", ID: "control-cancel-grant"},
		},
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant credits: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit wallet tx: %v", err)
	}
	return service
}

// holdForTask reserves credits whose business reference names taskID,
// returning the hold transaction id. The task_billing_snapshots trigger
// cross-checks the hold against the snapshot, so the reference must match.
func (e *controlCancelEnv) holdForTask(t *testing.T, service *credits.Service, taskID string) string {
	t.Helper()
	ctx := context.Background()
	hold, err := domain.NewCreditAmount(100)
	if err != nil {
		t.Fatalf("hold amount: %v", err)
	}
	result, err := service.Hold(ctx, creditsports.HoldCommand{
		UserID: e.userID, Amount: hold,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: "control-cancel-hold-" + randomControlHex(t, 6),
			Actor:          domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(e.userID)},
			Source:         "task_submission", Reason: "control cancel fixture",
			BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
		},
	})
	if err != nil {
		t.Fatalf("hold credits: %v", err)
	}
	return result.TransactionID
}

// seedPaidTask inserts a pending task carrying a real billing snapshot
// with a hold, so cancel must release credits inside its transaction.
func (e *controlCancelEnv) seedPaidTask(t *testing.T, service *credits.Service, taskID string) {
	t.Helper()
	holdID := e.holdForTask(t, service, taskID)
	envelope := fmt.Sprintf(
		`{"params":{},"billing_snapshot":{"sku":"ai-image:render","display_name":"Render",`+
			`"unit_credits":100,"quantity":1,"total_credits":100,"quoted_at":"2026-01-01T00:00:00Z",`+
			`"hold_transaction_id":%q,"settlement_state":"held"}}`,
		holdID,
	)
	// routing_snapshot is jsonb, so it must be NULL rather than '' (an empty
	// string is not valid JSON). workflow_input goes in as $4::text::jsonb:
	// with a bare $4::jsonb pgx picks its JSON codec and JSON-encodes the
	// Go string a second time, which the server rejects.
	if _, err := e.pool.Exec(context.Background(), `
INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status, priority, retry_count, lease_token, error_message, routing_snapshot)
VALUES ($1, (SELECT internal_id FROM workspaces WHERE public_id = $5), $2, $3, 'render', $4::text::jsonb, 'pending', 2, 0, '', '', NULL)`,
		taskID, e.userID, e.billingID, envelope, e.workspace); err != nil {
		t.Fatalf("seed paid control task %s: %v", taskID, err)
	}
	// The billing snapshot/settlement rows the settlement transaction UPDATEs.
	// Without the settlement row the Release leg rolls back with "not found",
	// which would make every leg here fail for the wrong reason.
	if _, err := e.pool.Exec(context.Background(), `
INSERT INTO task_billing_snapshots (task_id, user_id, service, sku, display_name, price_version_id, price_version, output_resolution, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id)
VALUES ($1, $2, 'render', 'ai-image:render', 'Render', $4, $5, '1k', 100, 1, 100, now(), $3)`,
		taskID, e.userID, holdID, e.priceVersionID, e.priceVersion); err != nil {
		t.Fatalf("seed paid control billing snapshot %s: %v", taskID, err)
	}
	if _, err := e.pool.Exec(context.Background(), `
INSERT INTO task_billing_settlements (task_id, state) VALUES ($1, 'held')`, taskID); err != nil {
		t.Fatalf("seed paid control billing settlement %s: %v", taskID, err)
	}
}

func (e *controlCancelEnv) taskStatus(t *testing.T, taskID string) string {
	t.Helper()
	var status string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT status FROM tasks WHERE public_id = $1`, taskID).Scan(&status); err != nil {
		t.Fatalf("read status for %s: %v", taskID, err)
	}
	return status
}

func (e *controlCancelEnv) adapter(holder workspacetasks.Holder, users ControlCreditDirectory, events ControlTaskEventPort) *workspaceControlTaskAdapter {
	return NewWorkspaceControlTasks(e.pool, taskpostgres.NewStore(e.pool), holder, users, events)
}

// publishFixturePrice publishes the render price the task billing snapshot
// references. task_billing_snapshots has a composite FK onto
// billing_price_versions, so the snapshot cannot be seeded without the real
// published price behind it.
func (e *controlCancelEnv) publishFixturePrice(t *testing.T) {
	t.Helper()
	store := billingpostgres.NewStore(e.pool)
	if _, err := store.PublishPrice(context.Background(), billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: "ai-image:render", Namespace: "ai-image", ProductKind: "workflow-feature",
			DisplayName: "Render", PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
		},
		UnitCredits: 100, ActorID: e.userID,
	}); err != nil {
		t.Fatalf("publish fixture price: %v", err)
	}
}

func (e *controlCancelEnv) readPriceVersion(t *testing.T) (int64, int64) {
	t.Helper()
	var id, version int64
	if err := e.pool.QueryRow(context.Background(),
		`SELECT price_version_id, version FROM billing_price_versions WHERE sku = 'ai-image:render' AND unit_credits = 100`,
	).Scan(&id, &version); err != nil {
		t.Fatalf("read fixture price version: %v", err)
	}
	return id, version
}

// TestControlAdminCancelAnnouncesTaskCancelledExactlyOnce is the primary
// regression test for the dead channel: an operator cancel must announce
// task.cancelled EXACTLY once on the task status channel and EXACTLY once
// as a workspace event, carrying the workspace, task, operator reason,
// and operator identity.
func TestControlAdminCancelAnnouncesTaskCancelledExactlyOnce(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_ctl_cancel1"
	env.seedPaidTask(t, creditService, taskID)

	operatorPublicID := uuid.New()
	events := &recordingControlTaskEvents{}
	adapter := env.adapter(
		&controlTxHolder{credits: creditService},
		controlOperatorDirectory{userID: env.userID, publicID: operatorPublicID},
		events,
	)

	if err := adapter.CancelAdminTask(context.Background(), taskID, env.userID, "operator cancelled for abuse"); err != nil {
		t.Fatalf("CancelAdminTask: %v", err)
	}
	if got := env.taskStatus(t, taskID); got != "cancelled" {
		t.Fatalf("task status = %q, want cancelled (the settlement must have committed)", got)
	}

	statuses, envelopes := events.snapshot()
	if len(statuses) != 1 {
		t.Fatalf("task status announcements = %d, want exactly 1: %+v", len(statuses), statuses)
	}
	if len(envelopes) != 1 {
		t.Fatalf("workspace envelopes = %d, want exactly 1: %+v", len(envelopes), envelopes)
	}
	published := statuses[0]
	if published.Status != string(workspacetasks.StatusCancelled) {
		t.Fatalf("status announcement = %q, want cancelled", published.Status)
	}
	if published.TaskID != taskID || published.WorkspaceID != env.workspace {
		t.Fatalf("status announcement scope = %s/%s, want %s/%s", published.WorkspaceID, published.TaskID, env.workspace, taskID)
	}
	if published.Reason != "operator cancelled for abuse" {
		t.Fatalf("status announcement reason = %q, want the operator reason", published.Reason)
	}
	if published.ActorUserID != tenancy.FormatUserPublicID(operatorPublicID) {
		t.Fatalf("status announcement actor = %q, want the operator usr_ id", published.ActorUserID)
	}
	if published.OccurredAt.IsZero() {
		t.Fatal("status announcement carried a zero occurred_at; it must be stamped after the commit")
	}
	if envelopes[0].TaskID != taskID || envelopes[0].Status != string(workspacetasks.StatusCancelled) {
		t.Fatalf("workspace envelope = %+v, want task.cancelled for %s", envelopes[0], taskID)
	}
}

// TestControlAdminCancelReplaysAnnounceNothingOnce proves exactly-once
// from the other side: a terminal re-cancel is an idempotent replay that
// stages nothing, so the counters must NOT move. A drain that fired per
// call would double-announce here.
func TestControlAdminCancelReplaysAnnounceNothingOnce(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_ctl_replay1"
	env.seedPaidTask(t, creditService, taskID)

	events := &recordingControlTaskEvents{}
	adapter := env.adapter(&controlTxHolder{credits: creditService}, nil, events)

	if err := adapter.CancelAdminTask(context.Background(), taskID, env.userID, "first"); err != nil {
		t.Fatalf("first CancelAdminTask: %v", err)
	}
	statuses, envelopes := events.snapshot()
	if len(statuses) != 1 || len(envelopes) != 1 {
		t.Fatalf("after first cancel: statuses=%d envelopes=%d, want 1/1", len(statuses), len(envelopes))
	}
	// The second cancel hits an already-terminal row: no fresh flip, no staging.
	if err := adapter.CancelAdminTask(context.Background(), taskID, env.userID, "replay"); err != nil {
		t.Fatalf("replayed CancelAdminTask: %v", err)
	}
	statuses, envelopes = events.snapshot()
	if len(statuses) != 1 || len(envelopes) != 1 {
		t.Fatalf("after replay: statuses=%d envelopes=%d, want still 1/1 (a replay announces nothing)", len(statuses), len(envelopes))
	}
}

// TestControlAdminCancelRollbackAnnouncesNothing is the post-commit
// contract. The Release fails inside the settlement transaction, so the
// state flip rolls back: the task stays pending and BOTH counters must
// stay at zero. A flush placed before the commit, or one that published
// on the error path, would announce a cancellation that never happened.
func TestControlAdminCancelRollbackAnnouncesNothing(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_ctl_rollback1"
	env.seedPaidTask(t, creditService, taskID)

	holder := &controlTxHolder{credits: creditService}
	holder.failRelease = errors.New("ledger unavailable")
	events := &recordingControlTaskEvents{}
	adapter := env.adapter(holder, nil, events)

	if err := adapter.CancelAdminTask(context.Background(), taskID, env.userID, "should roll back"); err == nil {
		t.Fatal("cancel with a failing Release must fail")
	}
	if got := env.taskStatus(t, taskID); got != "pending" {
		t.Fatalf("task status = %q, want pending (the failed settlement must roll the flip back)", got)
	}
	statuses, envelopes := events.snapshot()
	if len(statuses) != 0 || len(envelopes) != 0 {
		t.Fatalf("rolled-back cancel announced statuses=%d envelopes=%d, want 0/0: %+v %+v",
			len(statuses), len(envelopes), statuses, envelopes)
	}
}

// TestControlAdminCancelSettlesWithNilEventPort proves the port is
// optional in the safe direction: with no port wired the settlement still
// commits and reports success. Announcements must never be able to fail a
// cancel, so the seam degrades to silence, never to an error.
func TestControlAdminCancelSettlesWithNilEventPort(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_ctl_nilport1"
	env.seedPaidTask(t, creditService, taskID)

	adapter := env.adapter(&controlTxHolder{credits: creditService}, nil, nil)
	if err := adapter.CancelAdminTask(context.Background(), taskID, env.userID, "no port wired"); err != nil {
		t.Fatalf("cancel with a nil event port: %v", err)
	}
	if got := env.taskStatus(t, taskID); got != "cancelled" {
		t.Fatalf("task status = %q, want cancelled (a missing port must not fail the settlement)", got)
	}
}

// TestControlAdminCancelSurvivesEventPortFailure proves announcement
// errors are warn-and-continue: a failing port still leaves the
// committed cancellation reported as success, because the domain fact
// already landed and failing the response would misreport it.
func TestControlAdminCancelSurvivesEventPortFailure(t *testing.T) {
	env := openControlCancelEnv(t)
	creditService := env.creditServiceFor(t)
	const taskID = "task_ctl_portfail1"
	env.seedPaidTask(t, creditService, taskID)

	events := &recordingControlTaskEvents{
		statusErr:   errors.New("status channel down"),
		envelopeErr: errors.New("webhook dispatch down"),
	}
	adapter := env.adapter(&controlTxHolder{credits: creditService}, nil, events)
	if err := adapter.CancelAdminTask(context.Background(), taskID, env.userID, "port is down"); err != nil {
		t.Fatalf("a failing event port must not fail the cancel: %v", err)
	}
	if got := env.taskStatus(t, taskID); got != "cancelled" {
		t.Fatalf("task status = %q, want cancelled", got)
	}
	statuses, envelopes := events.snapshot()
	if len(statuses) != 1 || len(envelopes) != 1 {
		t.Fatalf("a failing port must still be ATTEMPTED once per channel: statuses=%d envelopes=%d", len(statuses), len(envelopes))
	}
}
