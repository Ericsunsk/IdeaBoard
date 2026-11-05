package workspacetasks

// Settlement aggregation for settle.go / settle_tx.go (atomic settlement,
// carrier settlement, staged tx events). Merged from settle_atomic_test.go,
// settle_carrier_test.go and settle_tx_event_test.go; the source files were
// orphans with no same-named implementation.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/generation/catalog"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/media/workspacemedia"
	taskpostgres "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/tasking/settlement"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// merged from settle_atomic_test.go.
// Package workspacetasks_test proves the single-transaction settlement
// boundary (#826, Phase 4b-02 of #809) over the canonical fixture database.
//
// What is proven here (all legs in ONE pgx transaction opened on the task
// store pool):
//   - Create: Quote snapshot + Credit Hold + task insert + enqueue commit
//     atomically; an enqueue failure rolls everything back (no task row,
//     no hold ledger row, no enqueue probe row).
//   - Complete: state flip + outputs persist + Capture (+ remainder
//     Release) commit atomically; a Capture failure leaves the task
//     unchanged with the hold intact and no capture artifacts.
//   - Fail/Cancel: state flip + Release commit atomically; a Release
//     failure leaves the task unchanged.
//   - A failed task write (unknown id) leaves no billing artifacts
//     (orphans query over credit_transactions).
//
// The enqueue leg joins the business tx through TxEnqueuer: the probe
// INSERT below rides the same tx (rolled back on failure). The production
// River join (workspaceTaskEnqueuer.EnqueueInTx -> client.InsertTx) is
// proven at the adapter level in composition (mock RiverTxClient asserts
// the ambient tx is forwarded); the fixture snapshot carries no river_job
// table, so no real River insert runs here.
//
// Tests SKIP gracefully without API_TEST_DATABASE_DSN (house convention).

// allowTestPoolResolver serves every service/resolution from one enabled
// entry. Settlement tests run against real PG for the tx legs; the routing
// decision itself needs no database.
func allowTestPoolResolver() *routing.RoutingResolver {
	seed := routing.SelectionSeed{9, 9, 9}
	return routing.NewRoutingResolver(allowAllAtomicPoolStore{},
		routing.WithSeedGenerator(func() (routing.SelectionSeed, error) { return seed, nil }))
}

type allowAllAtomicPoolStore struct{}

func (allowAllAtomicPoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	return routing.Pool{Service: service, Version: 1, Entries: []routing.ModelPoolEntry{
		{Provider: "openrouter", Model: "test-model", Enabled: true, SupportedResolutions: []routing.Resolution{
			routing.Resolution1K, routing.Resolution2K, routing.Resolution4K,
		}},
	}}, nil
}

func (allowAllAtomicPoolStore) ListPoolInTx(ctx context.Context, _ pgx.Tx, service string) (routing.Pool, error) {
	return allowAllAtomicPoolStore{}.ListPool(ctx, service)
}

const atomicTestChecksum = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func randomAtomicHex(t *testing.T, nbytes int) string {
	t.Helper()
	buf := make([]byte, nbytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("random hex failed: %v", err)
	}
	return hex.EncodeToString(buf)
}

// atomicQuoter adapts the real Billing catalog to the settlement seams:
// Quote for the preflight, QuoteInTx for the single-tx path.
type atomicQuoter struct {
	catalog *billingapplication.BillableItemCatalog
}

func (q *atomicQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (BillingSnapshot, error) {
	quote, err := q.catalog.Quote(ctx, nil, "ai-image:"+kind, billing.OutputResolution(resolution), quantity)
	if err != nil {
		return BillingSnapshot{}, err
	}
	return mapAtomicQuote(quote), nil
}

func (q *atomicQuoter) QuoteInTx(ctx context.Context, tx pgx.Tx, kind, resolution string, quantity int64) (BillingSnapshot, error) {
	quote, err := q.catalog.Quote(ctx, tx, "ai-image:"+kind, billing.OutputResolution(resolution), quantity)
	if err != nil {
		return BillingSnapshot{}, err
	}
	return mapAtomicQuote(quote), nil
}

func mapAtomicQuote(quote billing.Quote) BillingSnapshot {
	return BillingSnapshot{
		SKU: string(quote.SKU), DisplayName: quote.DisplayName,
		PriceVersionID: quote.PriceVersionID, PriceVersion: quote.PriceVersion,
		OutputResolution: string(quote.OutputResolution),
		UnitCredits:      quote.UnitCredits, Quantity: quote.Quantity, TotalCredits: quote.TotalCredits,
		QuotedAt: quote.QuotedAt, SettlementState: "quoted",
	}
}

func atomicMetadata(key, taskID string, userID int64) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             domain.Actor{Type: domain.ActorUser, ID: fmt.Sprint(userID)},
		Source:            "task_submission",
		Reason:            "AI 生图任务 Idea 预授权",
		BusinessReference: domain.BusinessReference{Type: "task", ID: taskID},
	}
}

// atomicHolder adapts the real Credits service to transactional settlement.
// failCapture/failRelease inject billing-side failures to prove the task
// flip rolls back with them.
type atomicHolder struct {
	credits     *credits.Service
	mu          sync.Mutex
	failHold    error
	failCapture error
	failRelease error
}

func (h *atomicHolder) setFailures(hold, capture, release error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failHold, h.failCapture, h.failRelease = hold, capture, release
}

func (h *atomicHolder) holdFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failHold
}

func (h *atomicHolder) captureFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failCapture
}

func (h *atomicHolder) releaseFailure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failRelease
}

func (h *atomicHolder) HoldInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, amount int64, key, taskID string) (string, error) {
	if err := h.holdFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.SettlementInTx(tx).Hold(ctx, ports.HoldCommand{BillingAccountID: billingAccountID, Amount: creditAmount, Metadata: atomicMetadata(key, taskID, actorUserID)})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

func (h *atomicHolder) CaptureInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := h.captureFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.SettlementInTx(tx).Capture(ctx, ports.CaptureCommand{
		BillingAccountID: billingAccountID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: atomicMetadata("task-capture:"+taskID, taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

func (h *atomicHolder) ReleaseInTx(ctx context.Context, tx pgx.Tx, billingAccountID, actorUserID int64, holdID string, amount int64, taskID string) (string, error) {
	if err := h.releaseFailure(); err != nil {
		return "", err
	}
	creditAmount, err := domain.NewCreditAmount(amount)
	if err != nil {
		return "", err
	}
	result, err := h.credits.SettlementInTx(tx).Release(ctx, ports.ReleaseCommand{
		BillingAccountID: billingAccountID, HoldTransactionID: holdID, Amount: creditAmount,
		Metadata: atomicMetadata("task-release:"+taskID, taskID, actorUserID),
	})
	if err != nil {
		return "", err
	}
	return result.TransactionID, nil
}

// atomicQueue proves the enqueue leg joins the business transaction: the
// probe INSERT rides the same pgx.Tx, so a rollback removes it. The
// production River join is covered in composition (InsertTx delegation).
type atomicQueue struct {
	mu       sync.Mutex
	pool     *pgxpool.Pool
	failNext error
	calls    int
}

func (q *atomicQueue) EnqueueInTx(ctx context.Context, tx pgx.Tx, task *WorkspaceTask) error {
	q.mu.Lock()
	fail := q.failNext
	q.failNext = nil
	q.calls++
	q.mu.Unlock()
	if fail != nil {
		return fail
	}
	if tx == nil {
		return errors.New("atomicQueue: settlement transaction is nil")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settle_enqueue_probe (task_id) VALUES ($1) ON CONFLICT (task_id) DO NOTHING`, strings.TrimSpace(task.ID)); err != nil {
		return fmt.Errorf("atomicQueue: probe enqueue: %w", err)
	}
	return nil
}

func (q *atomicQueue) failOnce(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failNext = err
}

type atomicEnv struct {
	pool        *pgxpool.Pool
	store       *taskpostgres.Store
	credits     *credits.Service
	catalog     *billingapplication.BillableItemCatalog
	holder      *atomicHolder
	queue       *atomicQueue
	service     *Service
	serviceDeps ServiceDeps
	ws          string
	userID      int64
	accountID   int64
}

func openAtomicEnv(t *testing.T, unitCost int64) *atomicEnv {
	t.Helper()
	return openAtomicEnvWithPrice(t, workflow.FeatureSlugRender, &unitCost)
}

// A nil price leaves a registered feature unpriced.
func openAtomicEnvWithPrice(t *testing.T, feature string, unitCost *int64) *atomicEnv {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)

	if _, err := pool.Exec(ctx, `CREATE TABLE settle_enqueue_probe (task_id TEXT PRIMARY KEY, enqueued_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("create enqueue probe table: %v", err)
	}

	storageUID := "T" + strings.ToUpper(randomAtomicHex(t, 3))[:5]
	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ('atomic-settle-user', $1, 'user', 1) RETURNING id`,
		storageUID,
	).Scan(&userID); err != nil {
		t.Fatalf("seed atomic settle user: %v", err)
	}
	ws := "ws_atomic" + randomAtomicHex(t, 6)
	if _, err := pool.Exec(ctx,
		`INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, 'atomic-settle-ws')`,
		ws, userID,
	); err != nil {
		t.Fatalf("seed atomic settle workspace: %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,$2,'owner' FROM workspaces WHERE public_id=$1`, ws, userID); err != nil {
		t.Fatal(err)
	}
	boardStore := boards.NewPostgresWorkspaceBoardStore(pool, workspacemedia.ValidateDocumentAssets)
	acceptance := &workspacemedia.TaskSubmissionAuthorizer{Boards: boardStore, TaskBoards: &boardtaskaccess.Authorizer{Store: boardStore}}

	creditService := credits.NewService(pool)
	// Seed funds on the task workspace's stable billing account (#916):
	// submission Holds spend the resolved account, never an owner wallet.
	var workspaceInternalID int64
	if err := pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, ws).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve atomic settle workspace: %v", err)
	}
	workspaceAccount, err := tenancypg.NewBillingAccountStore(pool).EnsureBillingAccountForWorkspace(ctx, workspaceInternalID, ws)
	if err != nil {
		t.Fatalf("resolve atomic settle billing account: %v", err)
	}
	accountID := workspaceAccount.InternalID
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := creditService.LifecycleInTx(tx).ProvisionAccountWallet(ctx, accountID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("provision atomic settle wallet: %v", err)
	}
	grantAmount, err := domain.NewCreditAmount(1000)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err := creditService.GrantsInTx(tx).Grant(ctx, ports.GrantCommand{BillingAccountID: accountID, Amount: grantAmount, Metadata: domain.CommandMetadata{
		IdempotencyKey: "atomic-settle-seed-" + randomAtomicHex(t, 4),
		Actor:          domain.Actor{Type: domain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "atomic settlement seed",
		BusinessReference: domain.BusinessReference{Type: "test", ID: "atomic-settle-seed"},
	}}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("grant atomic settle funds: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	billingStore := billingpostgres.NewStore(pool)
	definition := billing.BillableItemDefinition{
		SKU: billing.SKU("ai-image:" + feature), Namespace: "ai-image", ProductKind: "workflow-feature",
		DisplayName: "Render", PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
	}
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{definition})
	if err != nil {
		t.Fatal(err)
	}
	billingCatalog := billingapplication.NewBillableItemCatalog(registry, billingStore)
	if unitCost != nil {
		if _, err := billingStore.PublishPrice(ctx, billing.PublishPriceInput{Definition: definition, UnitCredits: *unitCost, ActorID: userID}); err != nil {
			t.Fatalf("publish atomic settle price: %v", err)
		}
	}

	store := taskpostgres.NewStore(pool)
	holder := &atomicHolder{credits: creditService}
	queue := &atomicQueue{pool: pool}
	snapshotSaver := func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		if tx == nil {
			return errors.New("routing snapshot saver requires the settlement transaction")
		}
		return catalog.SaveRoutingSnapshot(ctx, dbsqlc.New(tx), taskID, snap)
	}
	deps := ServiceDeps{
		Acceptance: acceptance, TaskRead: store, TaskLifecycle: store, SubmissionClaims: store, BillingAccounts: store, TaskBilling: store, Quoter: &atomicQuoter{catalog: billingCatalog}, Holder: holder, Queue: queue, Resolver: allowTestPoolResolver(), SnapshotSaver: snapshotSaver,
	}
	service := NewService(deps)
	return &atomicEnv{pool: pool, store: store, credits: creditService, catalog: billingCatalog, holder: holder, queue: queue, service: service, serviceDeps: deps, ws: ws, userID: userID, accountID: accountID}
}

func (e *atomicEnv) wallet(t *testing.T) (available, reserved int64) {
	t.Helper()
	details, err := e.credits.GetWallet(context.Background(), e.accountID)
	if err != nil {
		t.Fatalf("read atomic settle wallet: %v", err)
	}
	return details.Available, details.Reserved
}

func (e *atomicEnv) ledgerCount(t *testing.T, where string, args ...any) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM credit_transactions WHERE `+where, args...).Scan(&count); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return count
}

func (e *atomicEnv) probeCount(t *testing.T, taskID string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM settle_enqueue_probe WHERE task_id = $1`, taskID).Scan(&count); err != nil {
		t.Fatalf("count enqueue probe rows: %v", err)
	}
	return count
}

// createPaidTaskKeyed creates a task with a caller-chosen idempotency key and
// returns both, so tests can assert the durable key's effects.
func (e *atomicEnv) createPaidTaskKeyed(t *testing.T, quantity int64, key string) *WorkspaceTask {
	t.Helper()
	created, err := e.service.Create(context.Background(), CreateInput{
		WorkspaceID: e.ws, OwnerUserID: e.userID,
		Service: "render", Quantity: quantity, Resolution: "1k",
	}, key)
	if err != nil {
		t.Fatalf("atomic Create: %v", err)
	}
	_ = e.service.StagedEvents()
	return created
}

func (e *atomicEnv) createPaidTask(t *testing.T, quantity int64) *WorkspaceTask {
	t.Helper()
	return e.createPaidTaskKeyed(t, quantity, "atomic-key-"+randomAtomicHex(t, 4))
}

func (e *atomicEnv) claimTask(t *testing.T, taskID, token string) {
	t.Helper()
	if _, err := e.store.ClaimTask(context.Background(), e.ws, taskID, token); err != nil {
		t.Fatalf("atomic ClaimTask: %v", err)
	}
}

func atomicRemoteOutput() []OutputRef {
	return []OutputRef{
		{Kind: OutputKindRemote, URL: "https://cdn.example.com/atomic-o.png", ChecksumSHA256: atomicTestChecksum, ResultIndex: 0},
	}
}

// TestAtomicCreateSettlesHoldInsertEnqueue proves submission commits Quote
// snapshot + Hold + task insert + enqueue atomically: the task row, the
// hold ledger row, the reserved balance, and the enqueue probe all exist.
func TestAtomicCreateSettlesHoldInsertEnqueue(t *testing.T) {
	env := openAtomicEnv(t, 10)
	key := "atomic-key-" + randomAtomicHex(t, 4)
	created := env.createPaidTaskKeyed(t, 1, key)

	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatalf("GetTask after atomic Create: %v", err)
	}
	if stored.Status != StatusPending {
		t.Fatalf("status = %q, want pending", stored.Status)
	}
	if stored.BillingSnapshot.HoldTransactionID == "" || stored.BillingSnapshot.SettlementState != "held" {
		t.Fatalf("snapshot = %+v, want held with hold id", stored.BillingSnapshot)
	}
	// The hold key is derived from the DURABLE submit identity
	// (workspace + operation + key), never from the freshly minted task id:
	// a task-id-derived key differs on every retry and cannot fence anything.
	if env.ledgerCount(t, `idempotency_key = $1`, settlement.TaskHoldKey(env.ws, key)) != 1 {
		t.Fatal("hold ledger row missing after atomic Create")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10)", available, reserved)
	}
	if env.probeCount(t, created.ID) != 1 {
		t.Fatal("enqueue probe missing: enqueue did not join the settlement tx")
	}
}

// TestAtomicCreateRollsBackOnEnqueueFailure proves an enqueue failure rolls
// the whole submission back: no task row, no hold ledger row, no reserved
// balance, no probe row, no staged events.
func TestAtomicCreateRollsBackOnEnqueueFailure(t *testing.T) {
	env := openAtomicEnv(t, 10)
	env.queue.failOnce(errors.New("river unavailable"))

	_, err := env.service.Create(context.Background(), CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "atomic-key-"+randomAtomicHex(t, 4))
	if err == nil {
		t.Fatal("Create with failing enqueue must fail")
	}
	count, err := env.store.CountTasks(context.Background(), env.ws)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tasks = %d, want 0 (insert rolled back)", count)
	}
	if env.ledgerCount(t, `billing_account_id = $1 AND command_type = 'hold'`, env.accountID) != 0 {
		t.Fatal("hold ledger row leaked after rolled-back Create")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0) (hold rolled back)", available, reserved)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed settlement")
	}
}

// TestAtomicCompleteCaptures proves terminal Complete commits the state
// flip + outputs + Capture (+ remainder Release) atomically, with partial
// capture math (quantity 2, one output -> capture 10, release 10).
func TestAtomicCompleteCaptures(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 2)
	env.claimTask(t, created.ID, created.ExecutionToken)

	completed, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err != nil {
		t.Fatalf("atomic Complete: %v", err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("status = %q, want completed", completed.Status)
	}
	if len(completed.OutputRefs) != 1 {
		t.Fatalf("outputs = %+v, want 1 persisted ref", completed.OutputRefs)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-capture:"+created.ID) != 1 {
		t.Fatal("capture ledger row missing after atomic Complete")
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("remainder release ledger row missing after partial capture")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (990, 0) (captured 10, released 10)", available, reserved)
	}
	events := env.service.StagedEvents()
	if len(events) != 1 || events[0].Status != StatusCompleted {
		t.Fatalf("staged events = %+v, want one completed", events)
	}
}

// TestAtomicCompleteBillingFailureLeavesTaskUnchanged proves billing-fails
// -> task-unchanged: a Capture failure rolls the state flip and the output
// writes back, keeping the hold intact with no capture artifacts.
func TestAtomicCompleteBillingFailureLeavesTaskUnchanged(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	env.holder.setFailures(nil, errors.New("ledger unavailable"), nil)

	_, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput())
	if err == nil {
		t.Fatal("Complete with failing Capture must fail")
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusRunning {
		t.Fatalf("status = %q, want running (flip rolled back)", stored.Status)
	}
	if len(stored.OutputRefs) != 0 {
		t.Fatalf("outputs = %+v, want none (output writes rolled back)", stored.OutputRefs)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-capture:"+created.ID) != 0 {
		t.Fatal("capture ledger row leaked after rolled-back Complete")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10) (hold intact)", available, reserved)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on failed terminal settlement")
	}
}

// TestAtomicFailReleases proves Fail commits the state flip + Release
// atomically with the full hold returned to available.
func TestAtomicFailReleases(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	failed, err := env.service.Fail(context.Background(), env.ws, created.ID, "provider exploded")
	if err != nil {
		t.Fatalf("atomic Fail: %v", err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", failed.Status)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("release ledger row missing after atomic Fail")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0) (fully released)", available, reserved)
	}
}

// TestAtomicFailBillingFailureLeavesTaskUnchanged proves a Release failure
// rolls the Fail flip back: the task keeps running with the hold intact.
func TestAtomicFailBillingFailureLeavesTaskUnchanged(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	env.holder.setFailures(nil, nil, errors.New("ledger unavailable"))

	_, err := env.service.Fail(context.Background(), env.ws, created.ID, "provider exploded")
	if err == nil {
		t.Fatal("Fail with failing Release must fail")
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != StatusRunning {
		t.Fatalf("status = %q, want running (flip rolled back)", stored.Status)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want (990, 10) (hold intact)", available, reserved)
	}
}

// TestAtomicCancelReleases proves Cancel commits the state flip + Release
// atomically and replays idempotently without a second Release.
func TestAtomicCancelReleases(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)

	cancelled, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "no longer needed")
	if err != nil || already {
		t.Fatalf("Cancel = (%v, %v, %v), want fresh cancel", cancelled, already, err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("status = %q, want cancelled", cancelled.Status)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("release ledger row missing after atomic Cancel")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want (1000, 0)", available, reserved)
	}
	replay, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "again")
	if err != nil || !already || replay.Status != StatusCancelled {
		t.Fatalf("replay = (%v, %v, %v), want idempotent cancelled", replay, already, err)
	}
	if env.ledgerCount(t, `idempotency_key = $1`, "task-release:"+created.ID) != 1 {
		t.Fatal("cancel replay must not write a second Release")
	}
}

// TestAtomicTaskFailureWritesNoBillingArtifacts proves task-fails -> no
// billing artifacts: completing an unknown task writes no capture/release
// rows (orphans query over the task business reference).
func TestAtomicTaskFailureWritesNoBillingArtifacts(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ghostID := "task_ghost" + randomAtomicHex(t, 6)

	_, err := env.service.Complete(context.Background(), env.ws, ghostID, atomicRemoteOutput())
	if err == nil {
		t.Fatal("Complete of an unknown task must fail")
	}
	if count := env.ledgerCount(t, `business_reference_type = 'task' AND business_reference_id = $1`, ghostID); count != 0 {
		t.Fatalf("billing artifacts = %d for failed task %s, want 0", count, ghostID)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want untouched (1000, 0)", available, reserved)
	}
}

// TestAtomicRetryRequeuesInTx proves Retry consumes the budget entry and
// requeues atomically; a requeue failure keeps the running task untouched.
func TestAtomicRetryRequeuesInTx(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	retried, err := env.service.Retry(context.Background(), env.ws, created.ID, true)
	if err != nil {
		t.Fatalf("atomic Retry: %v", err)
	}
	if retried.Status != StatusPending || retried.RetryCount != 1 {
		t.Fatalf("retried = %+v, want pending retry_count=1", retried)
	}
	if env.probeCount(t, created.ID) != 1 {
		t.Fatal("requeue probe missing: requeue did not join the settlement tx")
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("wallet = (%d, %d), want hold untouched (990, 10)", available, reserved)
	}

	env.queue.failOnce(errors.New("river unavailable"))
	if _, err := env.service.Retry(context.Background(), env.ws, created.ID, true); err == nil {
		t.Fatal("Retry with failing requeue must fail")
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1 (budget bump rolled back)", stored.RetryCount)
	}
}

// TestAtomicFreeTerminalSettlementTriggerRejectsMoneyMovement proves the
// relaxed state machine still fails closed on value: a free snapshot can
// only converge with zero amounts.
func TestAtomicFreeTerminalSettlementTriggerRejectsMoneyMovement(t *testing.T) {
	env := openAtomicEnv(t, 0)
	created, err := env.service.Create(context.Background(), CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "atomic-free-trigger-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("free Create: %v", err)
	}
	_ = env.service.StagedEvents()
	ctx := context.Background()
	for _, statement := range []string{
		`UPDATE task_billing_settlements SET state = 'captured', actual_output_count = 1, captured_credits = 5, settled_at = now() WHERE task_id = $1`,
		`UPDATE task_billing_settlements SET state = 'released', actual_output_count = 0, released_credits = 5, settled_at = now() WHERE task_id = $1`,
		`UPDATE task_billing_settlements SET state = 'held', settled_at = NULL WHERE task_id = $1`,
	} {
		if _, err := env.pool.Exec(ctx, statement, created.ID); err == nil {
			t.Fatalf("statement unexpectedly succeeded: %.80s", statement)
		}
	}
}

// Submit-path routing snapshot tests over the fixture database.
// The resolver-wired service below shares openAtomicEnv's store, holder,
// queue, wallet, and billing price; only the pool, resolver, and snapshot
// saver are added, so the existing atomic tests continue to prove the
// settlement transaction independently.

// openRoutingAtomicEnv rebuilds the env service with the shared
// RoutingResolver over a seeded render pool plus the production snapshot
// saver (catalog.SaveRoutingSnapshot with the settlement tx's Queries).
// The billing catalog is rebuilt over the same pool; the ai-image:render
// 1k price published by openAtomicEnv persists in the database.
func openRoutingAtomicEnv(t *testing.T) *atomicEnv {
	t.Helper()
	env := openAtomicEnv(t, 10)
	ctx := context.Background()

	poolRepo := catalog.NewRepository(env.pool)
	if _, err := poolRepo.ReplacePool(ctx, "render", 0, []catalog.PoolEntryInput{
		{Provider: "atomic-test", Model: "model", Enabled: true, Resolutions: []string{"1k"}},
	}); err != nil {
		t.Fatalf("seed atomic routing pool: %v", err)
	}
	poolResolver := routing.NewRoutingResolver(poolRepo)
	poolSaver := func(ctx context.Context, tx pgx.Tx, taskID string, snap routing.RoutingSnapshot) error {
		if tx == nil {
			return errors.New("routing snapshot saver requires the settlement tx")
		}
		return catalog.SaveRoutingSnapshot(ctx, dbsqlc.New(tx), taskID, snap)
	}

	billingStore := billingpostgres.NewStore(env.pool)
	definition := billing.BillableItemDefinition{
		SKU: "ai-image:render", Namespace: "ai-image", ProductKind: "workflow-feature",
		DisplayName: "Render", PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
	}
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{definition})
	if err != nil {
		t.Fatal(err)
	}
	env.service = NewService(ServiceDeps{
		TaskRead: env.store, TaskLifecycle: env.store, SubmissionClaims: env.store, BillingAccounts: env.store, TaskBilling: env.store, Quoter: &atomicQuoter{catalog: billingapplication.NewBillableItemCatalog(registry, billingStore)},
		Holder: env.holder, Queue: env.queue,
		Resolver: poolResolver, SnapshotSaver: poolSaver, Acceptance: env.serviceDeps.Acceptance,
	})
	return env
}

func loadAtomicSnapshot(t *testing.T, env *atomicEnv, taskID string) routing.RoutingSnapshot {
	t.Helper()
	snap, err := catalog.LoadRoutingSnapshot(context.Background(), dbsqlc.New(env.pool), taskID)
	if err != nil {
		t.Fatalf("LoadRoutingSnapshot(%s): %v", taskID, err)
	}
	return snap
}

// TestAtomicRoutingSnapshotPinnedInTx proves submit resolves via the shared
// RoutingResolver and pins the snapshot atomically with Hold/task/enqueue:
// the stored fact replays without the live pool.
func TestAtomicRoutingSnapshotPinnedInTx(t *testing.T) {
	env := openRoutingAtomicEnv(t)
	created := env.createPaidTask(t, 1)

	snap := loadAtomicSnapshot(t, env, created.ID)
	if err := snap.Verify(); err != nil {
		t.Fatalf("pinned snapshot must replay without the live pool: %v", err)
	}
	if snap.Service != "render" || snap.RequestedResolution != routing.Resolution1K || snap.PoolVersion != 1 {
		t.Fatalf("pinned pinning = %+v, want render/1k/version 1", snap)
	}
	if len(snap.EligibleModels) != 1 || snap.EligibleModels[0] != "atomic-test/model" {
		t.Fatalf("eligible = %v, want [atomic-test/model]", snap.EligibleModels)
	}
	if snap.SelectedModel != "atomic-test/model" || snap.HasRetryCandidate() {
		t.Fatalf("selected = %q retry = %q, want the single candidate with no retry", snap.SelectedModel, snap.RetryCandidate)
	}
}

// TestAtomicRoutingUnavailableReservesNothing proves an empty eligible set
// converges to the canonical sentinel before Hold, task insert, or enqueue:
// no task row, no hold ledger row, no reserved balance, no enqueue probe.
func TestAtomicRoutingUnavailableReservesNothing(t *testing.T) {
	env := openRoutingAtomicEnv(t)
	ctx := context.Background()
	poolRepo := catalog.NewRepository(env.pool)
	version, err := poolRepo.GetVersion(ctx, "render")
	if err != nil {
		t.Fatalf("pool version: %v", err)
	}
	if _, err := poolRepo.ReplacePool(ctx, "render", version, nil); err != nil {
		t.Fatalf("clear atomic routing pool: %v", err)
	}

	_, err = env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "atomic-routing-unavail-"+randomAtomicHex(t, 4))
	if !errors.Is(err, routing.ErrServiceResolutionUnavailable) {
		t.Fatalf("Create err = %v, want ErrServiceResolutionUnavailable", err)
	}
	var unavailable *routing.ResolutionUnavailableError
	if !errors.As(err, &unavailable) || unavailable.Service != "render" || unavailable.Resolution != routing.Resolution1K {
		t.Fatalf("Create fields = %+v, want normalized render/1k", unavailable)
	}
	count, err := env.store.CountTasks(ctx, env.ws)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tasks = %d, want 0", count)
	}
	if env.ledgerCount(t, `billing_account_id = $1 AND command_type = 'hold'`, env.accountID) != 0 {
		t.Fatal("hold ledger row leaked after unavailable Create")
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("wallet = (%d, %d), want untouched (1000, 0)", available, reserved)
	}
	var probes int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM settle_enqueue_probe`).Scan(&probes); err != nil {
		t.Fatalf("count enqueue probes: %v", err)
	}
	if probes != 0 {
		t.Fatalf("enqueue probes = %d, want 0", probes)
	}
	if len(env.service.StagedEvents()) != 0 {
		t.Fatal("events must not stage on unavailable Create")
	}
}

// TestAtomicRoutingSnapshotSurvivesPoolEdits proves later Admin pool edits
// never invalidate an accepted task nor change its candidate set: the
// stored snapshot still replays byte-identical without the live pool.
func TestAtomicRoutingSnapshotSurvivesPoolEdits(t *testing.T) {
	env := openRoutingAtomicEnv(t)
	ctx := context.Background()
	created := env.createPaidTask(t, 1)
	before := loadAtomicSnapshot(t, env, created.ID)

	poolRepo := catalog.NewRepository(env.pool)
	version, err := poolRepo.GetVersion(ctx, "render")
	if err != nil {
		t.Fatalf("pool version: %v", err)
	}
	if _, err := poolRepo.ReplacePool(ctx, "render", version, []catalog.PoolEntryInput{
		{Provider: "atomic-test", Model: "other", Enabled: true, Resolutions: []string{"2k", "4k"}},
	}); err != nil {
		t.Fatalf("edit atomic routing pool: %v", err)
	}

	after := loadAtomicSnapshot(t, env, created.ID)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("stored snapshot changed by the pool edit:\n%+v\n%+v", before, after)
	}
	if err := after.Verify(); err != nil {
		t.Fatalf("Verify() after pool edit = %v; replay must not need the live pool", err)
	}
}

// merged from settle_carrier_test.go.
// Package workspacetasks_test proves the settlement joins the ambient
// guard transaction (control audit atomicity, step 3).
//
// What is proven here, against the isolated fixture database:
//   - Cancel on a carrier context settles inside the carrier: the
//     terminal write is a real INSERT on the carrier transaction,
//     readable from the still-open carrier and invisible on the pool,
//     then durable after the carrier commits. The store's own
//     BeginSettlementScope is never called (no nested transaction).
//   - A carrier rollback leaves no settlement artifact behind (the
//     step-2 gap: the settlement commit used to survive on its own).
//   - With no carrier the settlement-transactor path runs unchanged
//     (settle_atomic_test.go is that proof; it is not duplicated here).

// carrierSettleStore is a tx-observing settlement fake: every seam the
// cancel path needs is present, but the terminal flip performs a REAL
// INSERT on the transaction it is handed, so the test observes exactly
// which transaction the settlement joined.
type carrierSettleStore struct {
	t                *testing.T
	probe            string
	beginCalled      bool
	cancelCalls      int
	lastTxNil        bool
	cancelledTaskIDs []string
}

func (f *carrierSettleStore) BeginSettlementScope(context.Context) (*dbutil.Scope, error) {
	f.beginCalled = true
	return nil, fmt.Errorf("carrier settle store: settlement must join the carrier, never begin its own transaction")
}

func (f *carrierSettleStore) GetTask(_ context.Context, workspaceID, taskID string) (*WorkspaceTask, error) {
	return &WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, Status: StatusRunning}, nil
}

func (f *carrierSettleStore) ListTasks(_ context.Context, _ string, _, _ int) ([]*WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) CountTasks(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (f *carrierSettleStore) CreateTask(_ context.Context, task *WorkspaceTask) (*WorkspaceTask, error) {
	return task, nil
}

func (f *carrierSettleStore) CancelTask(_ context.Context, _, _ string, _ int64, _ string) (*WorkspaceTask, bool, error) {
	f.t.Fatal("non-transactional CancelTask must not run when the tx seams are wired")
	return nil, false, nil
}

func (f *carrierSettleStore) TransitionForRetry(_ context.Context, _, _, _ string) (*WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) CompleteTask(_ context.Context, _, _ string, _ []OutputRef) (*WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) FailTask(_ context.Context, _, _, _ string) (*WorkspaceTask, error) {
	return nil, nil
}

func (f *carrierSettleStore) DeleteTask(_ context.Context, _, _ string) error { return nil }

func (f *carrierSettleStore) CancelTaskInTx(ctx context.Context, tx pgx.Tx, workspaceID, taskID string, _ int64, _ string) (*WorkspaceTask, bool, error) {
	f.cancelCalls++
	if tx == nil {
		f.lastTxNil = true
		return nil, false, fmt.Errorf("carrier settle store: terminal flip requires a transaction")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+f.probe+` (id, note) VALUES ($1, 'cancelled')`, taskID); err != nil {
		return nil, false, err
	}
	f.cancelledTaskIDs = append(f.cancelledTaskIDs, taskID)
	// An empty billing snapshot keeps the holder out of this proof: the
	// terminal task write alone decides which transaction settled.
	return &WorkspaceTask{ID: taskID, WorkspaceID: workspaceID, OwnerUserID: 7, Status: StatusCancelled}, false, nil
}

func (f *carrierSettleStore) CompleteTaskInTx(_ context.Context, _ pgx.Tx, _, _ string, _ []OutputRef) (*WorkspaceTask, error) {
	return nil, fmt.Errorf("carrier settle store: CompleteTaskInTx must not run in the cancel-carrier proof")
}

func (f *carrierSettleStore) FailTaskInTx(_ context.Context, _ pgx.Tx, _, _, _ string) (*WorkspaceTask, error) {
	return nil, fmt.Errorf("carrier settle store: FailTaskInTx must not run in the cancel-carrier proof")
}

func (f *carrierSettleStore) TransitionForRetryInTx(_ context.Context, _ pgx.Tx, _, _, _ string) (*WorkspaceTask, error) {
	return nil, fmt.Errorf("carrier settle store: TransitionForRetryInTx must not run in the cancel-carrier proof")
}

func (f *carrierSettleStore) CreateTaskInTx(_ context.Context, _ pgx.Tx, task *WorkspaceTask) (*WorkspaceTask, error) {
	return task, nil
}

func (f *carrierSettleStore) GetTaskInTx(_ context.Context, _ pgx.Tx, workspaceID, taskID string) (*WorkspaceTask, error) {
	return &WorkspaceTask{ID: taskID, WorkspaceID: workspaceID}, nil
}

// carrierSettleHolder satisfies the holder seams without touching the
// ledger: the fabricated task carries no hold, so the holder is never
// called in this proof.
type carrierSettleHolder struct{}

func (carrierSettleHolder) HoldInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, _ int64, _, _ string) (string, error) {
	return "", nil
}

func (carrierSettleHolder) CaptureInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, _ string, _ int64, _ string) (string, error) {
	return "", nil
}

func (carrierSettleHolder) ReleaseInTx(_ context.Context, _ pgx.Tx, _ int64, _ int64, _ string, _ int64, _ string) (string, error) {
	return "", nil
}

func TestCancelJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	table := fmt.Sprintf("wt_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	store := &carrierSettleStore{t: t, probe: table}
	settler := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Holder: carrierSettleHolder{},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	const taskID = "task_carrier_1"
	if _, _, err := settler.Cancel(carrier, "ws-carrier", taskID, 7, "operator cancel"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Cancel() on carrier error = %v", err)
	}
	if store.beginCalled {
		_ = tx.Rollback(ctx)
		t.Fatal("settlement opened its own transaction despite the staged carrier (nested tx)")
	}
	if store.cancelCalls != 1 || store.lastTxNil {
		_ = tx.Rollback(ctx)
		t.Fatalf("terminal flip calls = %d nil-tx = %v, want 1/false", store.cancelCalls, store.lastTxNil)
	}
	// Same-transaction visibility: the terminal write must be readable
	// from INSIDE the still-open carrier ...
	var inside int
	if err := tx.QueryRow(carrier, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&inside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count probe inside carrier: %v", err)
	}
	if inside != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("settlement rows visible inside carrier = %d, want 1", inside)
	}
	// ... and invisible outside it before the carrier commits.
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("count probe on pool: %v", err)
	}
	if outside != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("settlement rows visible on pool before commit = %d, want 0", outside)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		t.Fatalf("count probe after commit: %v", err)
	}
	if outside != 1 {
		t.Fatalf("settlement rows after carrier commit = %d, want 1", outside)
	}
	// The terminal event still stages under a borrowed transaction (the
	// caller flushes it after the commit it can see).
	if staged := settler.StagedEvents(); len(staged) != 1 {
		t.Fatalf("staged events = %d, want 1", len(staged))
	}
}

func TestCancelRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	table := fmt.Sprintf("wt_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	store := &carrierSettleStore{t: t, probe: table}
	settler := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Holder: carrierSettleHolder{},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	const taskID = "task_carrier_2"
	if _, _, err := settler.Cancel(carrier, "ws-carrier", taskID, 7, "operator cancel"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Cancel() on carrier error = %v", err)
	}
	// The business fails after settlement: the guard rolls the whole
	// carrier back, settlement write included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		t.Fatalf("count probe after rollback: %v", err)
	}
	if outside != 0 {
		t.Fatalf("settlement rows after carrier rollback = %d, want 0", outside)
	}
	_ = settler
}

// TestCancelDefersStageOnRegistryCarrierUntilPostCommit proves the #889
// deferral at the settlement seam: on a registry-carrying ambient
// transaction Cancel settles the write but stages NOTHING publishable
// yet; the staged cancelled event appears only after the post-commit
// hooks run (the guard runs them right after its commit). A caller that
// drains before the commit therefore cannot announce the transition.
func TestCancelDefersStageOnRegistryCarrierUntilPostCommit(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	table := fmt.Sprintf("wt_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	store := &carrierSettleStore{t: t, probe: table}
	settler := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Holder: carrierSettleHolder{},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientPostCommit(dbutil.ContextWithAmbientTx(ctx, tx))
	const taskID = "task_carrier_3"
	if _, _, err := settler.Cancel(carrier, "ws-carrier", taskID, 7, "operator cancel"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Cancel() on registry carrier error = %v", err)
	}
	// Nothing publishable before the carrier commits: the guard has not
	// run the hooks yet, so the drain must be empty even though the
	// terminal write sits in the still-open transaction.
	if staged := settler.StagedEvents(); len(staged) != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("staged events before carrier commit = %d, want 0 (defer until post-commit)", len(staged))
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	// The guard commits, then runs the hooks: only now is the cancelled
	// transition staged for publication.
	dbutil.RunAmbientPostCommit(carrier)
	staged := settler.StagedEvents()
	if len(staged) != 1 {
		t.Fatalf("staged events after post-commit hooks = %d, want 1", len(staged))
	}
	if staged[0].Status != StatusCancelled || staged[0].TaskID != taskID {
		t.Fatalf("staged event = %+v, want cancelled for %s", staged[0], taskID)
	}
}

// TestCancelDiscardsDeferredStageWhenHooksNeverRun proves the other half
// of the deferral: when the carrier rolls back (audit/commit failure at
// the guard), the hooks never run and no transition is ever staged, so
// no announcement can escape for the rolled-back flip.
func TestCancelDiscardsDeferredStageWhenHooksNeverRun(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	table := fmt.Sprintf("wt_carrier_probe_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE TABLE `+table+` (id text PRIMARY KEY, note text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table)
	})

	store := &carrierSettleStore{t: t, probe: table}
	settler := NewService(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Holder: carrierSettleHolder{},
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientPostCommit(dbutil.ContextWithAmbientTx(ctx, tx))
	const taskID = "task_carrier_4"
	if _, _, err := settler.Cancel(carrier, "ws-carrier", taskID, 7, "operator cancel"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Cancel() on registry carrier error = %v", err)
	}
	// The guard fails (audit/commit) and rolls back without running the
	// hooks: the deferred stage is dropped with the context.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if staged := settler.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged events after carrier rollback = %d, want 0 (deferred stage discarded)", len(staged))
	}
	var outside int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, taskID).Scan(&outside); err != nil {
		t.Fatalf("count probe after rollback: %v", err)
	}
	if outside != 0 {
		t.Fatalf("settlement rows after carrier rollback = %d, want 0", outside)
	}
}

// merged from settle_tx_event_test.go.
// Post-commit staging of workspace task status events (#807 dead-channel
// fix).
//
// The settlement contract is: STAGE inside the call, PUBLISH after the
// transaction committed. Staging exists precisely so a publish can never
// announce a state a rollback still has the power to erase. These tests
// pin the other half of that contract over the REAL PostgreSQL settlement
// legs, using a probe transaction over the real store:
//
//   - a staged event means a transaction that committed successfully (the
//     probe records commit-ok, and an independent connection already reads
//     the new status);
//   - a settlement that rolled back, or whose COMMIT ITSELF failed, stages
//     nothing, so no event can escape for a fact that never landed.
//
// The commit-failure leg is the decisive one: it is the only observable
// that separates "staged after the commit" from "staged before the
// commit", because a stage placed ahead of the commit would leave a staged
// event behind on exactly that path.
//
// They run over the same fixture database as settle_atomic_test.go and skip
// without API_TEST_DATABASE_DSN (house convention).

// assertStagedEventIsCommitted reads the task row over a connection
// checked out of the pool — deliberately NOT the settlement transaction —
// so it can only observe what the commit made durable. A staged event whose
// row is still pre-transition would mean staging ran inside the
// transaction.
func assertStagedEventIsCommitted(t *testing.T, env *atomicEnv, taskID string, want Status) {
	t.Helper()
	conn, err := env.pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire independent connection: %v", err)
	}
	defer conn.Release()
	var status Status
	if err := conn.QueryRow(context.Background(), `SELECT status FROM tasks WHERE public_id = $1`, taskID).Scan(&status); err != nil {
		t.Fatalf("read committed status for %s: %v", taskID, err)
	}
	if status != want {
		t.Fatalf("staged event observed status %q on an independent connection, want %q (a staged event must follow the commit)", status, want)
	}
}

func assertSingleStagedStatus(t *testing.T, service *Service, env *atomicEnv, taskID string, want Status) {
	t.Helper()
	staged := service.StagedEvents()
	if len(staged) != 1 {
		t.Fatalf("staged %d events, want exactly 1: %+v", len(staged), staged)
	}
	if staged[0].Status != want {
		t.Fatalf("staged status = %q, want %q", staged[0].Status, want)
	}
	if staged[0].TaskID != taskID {
		t.Fatalf("staged task_id = %q, want %q", staged[0].TaskID, taskID)
	}
	if staged[0].WorkspaceID != env.ws {
		t.Fatalf("staged workspace_id = %q, want %q", staged[0].WorkspaceID, env.ws)
	}
	if staged[0].OccurredAt.IsZero() {
		t.Fatal("staged event carried a zero occurred_at")
	}
}

// TestSettlementStagesCompletedOnlyAfterCommit proves a successful
// terminal Complete stages exactly one completed event, and that the row
// is already durable when the event exists.
func TestSettlementStagesCompletedOnlyAfterCommit(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events before the terminal settlement, want 0: %+v", len(staged), staged)
	}
	if _, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, StatusCompleted)
	assertStagedEventIsCommitted(t, env, created.ID, StatusCompleted)
}

// TestSettlementStagesFailedOnlyAfterCommit proves the failure leg carries
// its own single event, and that a failed Release (which rolls the state
// flip back) stages nothing.
func TestSettlementStagesFailedOnlyAfterCommit(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	if _, err := env.service.Fail(context.Background(), env.ws, created.ID, "provider exploded"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, StatusFailed)
	assertStagedEventIsCommitted(t, env, created.ID, StatusFailed)

	// Rollback leg: a Release failure on a second task leaves it running
	// and must not stage anything.
	other := env.createPaidTask(t, 1)
	env.claimTask(t, other.ID, other.ExecutionToken)
	env.holder.setFailures(nil, nil, errors.New("ledger unavailable"))
	if _, err := env.service.Fail(context.Background(), env.ws, other.ID, "provider exploded"); err == nil {
		t.Fatal("Fail with a failing Release must fail")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events after a rolled-back Fail, want 0: %+v", len(staged), staged)
	}
	assertStagedEventIsCommitted(t, env, other.ID, StatusRunning)
}

// TestRetryInTxStagesPendingAfterCommit proves the requeue stages its
// running -> pending event exactly once, that the row is already pending
// when the event exists, and that a failed requeue (which rolls the budget
// bump back) stages nothing.
func TestRetryInTxStagesPendingAfterCommit(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	if _, err := env.service.Retry(context.Background(), env.ws, created.ID, true); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, StatusPending)
	assertStagedEventIsCommitted(t, env, created.ID, StatusPending)

	// Rollback leg: the requeue INSERT rides the settlement transaction,
	// so a River failure rolls the budget bump back and must stage nothing.
	env.queue.failOnce(errors.New("river unavailable"))
	if _, err := env.service.Retry(context.Background(), env.ws, created.ID, true); err == nil {
		t.Fatal("Retry with a failing requeue must fail")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events after a rolled-back Retry, want 0: %+v", len(staged), staged)
	}
	stored, err := env.store.GetTask(context.Background(), env.ws, created.ID)
	if err != nil {
		t.Fatalf("GetTask after the rolled-back Retry: %v", err)
	}
	if stored.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1 (the second budget bump rolled back)", stored.RetryCount)
	}
}

// TestRolledBackSettlementLeavesNoEventForTheNextDrain proves the two legs
// do not bleed into each other: after a rolled-back Complete, the next
// committed Complete stages exactly one event (its own). A leak here would
// make the worker's drain publish a transition twice.
func TestRolledBackSettlementLeavesNoEventForTheNextDrain(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)

	env.holder.setFailures(nil, errors.New("ledger unavailable"), nil)
	if _, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err == nil {
		t.Fatal("Complete with a failing Capture must fail")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events after the rolled-back Complete, want 0: %+v", len(staged), staged)
	}
	assertStagedEventIsCommitted(t, env, created.ID, StatusRunning)

	env.holder.setFailures(nil, nil, nil)
	if _, err := env.service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete after the ledger recovered: %v", err)
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, StatusCompleted)
}

// TestCancelReplayStagesNoSecondEvent proves exactly-once for the cancel
// leg the admin control adapter drains: a fresh flip stages one cancelled
// event, and a terminal replay stages none (otherwise a repeated operator
// cancel would re-announce the same transition).
func TestCancelReplayStagesNoSecondEvent(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)

	if _, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "operator cancel"); err != nil {
		t.Fatalf("Cancel: %v", err)
	} else if already {
		t.Fatal("first cancel must be a fresh flip")
	}
	assertSingleStagedStatus(t, env.service, env, created.ID, StatusCancelled)
	assertStagedEventIsCommitted(t, env, created.ID, StatusCancelled)

	if _, already, err := env.service.Cancel(context.Background(), env.ws, created.ID, env.userID, "operator cancel"); err != nil {
		t.Fatalf("replayed Cancel: %v", err)
	} else if !already {
		t.Fatal("second cancel must replay as already")
	}
	if staged := env.service.StagedEvents(); len(staged) != 0 {
		t.Fatalf("staged %d events on a terminal replay, want 0: %+v", len(staged), staged)
	}
}

// probeSettlementStore wraps the real postgres store so the settlement
// transaction it hands out is observable. Nothing about the settlement
// behaviour changes: it is the same store, the same SQL, the same
// transaction — only Commit/Rollback are recorded (and Commit can be made
// to fail on demand). The concrete store is embedded (not the interface)
// so every ...InTx seam is promoted unchanged.
type probeSettlementStore struct {
	*taskpostgres.Store

	mu sync.Mutex
	// pendingFailCommit arms the NEXT transaction to fail its commit, so
	// the injection can be set up before the settlement opens a tx.
	pendingFailCommit error
	steps             []string
	tx                *probeSettlementTx
}

func (s *probeSettlementStore) BeginSettlementScope(ctx context.Context) (*dbutil.Scope, error) {
	scope, err := s.Store.BeginSettlementScope(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.steps = append(s.steps, "begin")
	s.tx = &probeSettlementTx{Tx: scope.Tx(), store: s, failCommit: s.pendingFailCommit}
	s.pendingFailCommit = nil
	s.mu.Unlock()
	return dbutil.ScopeForTest(ctx, s.tx, true), nil
}

func (s *probeSettlementStore) record(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, step)
}

func (s *probeSettlementStore) failNextCommit(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		s.tx.failCommit = err
		return
	}
	s.pendingFailCommit = err
}

func (s *probeSettlementStore) sequence() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.steps...)
}

type probeSettlementTx struct {
	pgx.Tx
	store      *probeSettlementStore
	failCommit error
}

func (t *probeSettlementTx) Commit(ctx context.Context) error {
	if t.failCommit != nil {
		t.store.record("commit-fail")
		// The real commit never happens; the caller's deferred rollback
		// then discards every write in the transaction.
		return t.failCommit
	}
	if err := t.Tx.Commit(ctx); err != nil {
		t.store.record("commit-err")
		return err
	}
	t.store.record("commit-ok")
	return nil
}

func (t *probeSettlementTx) Rollback(ctx context.Context) error {
	t.store.record("rollback")
	return t.Tx.Rollback(ctx)
}

// newProbeService builds a second settlement Service over the SAME store,
// holder, quoter, and queue, differing only in that its transactions are
// observable. The staged-event buffer is per-Service, so the assertions
// below read only this Service's buffer.
func newProbeService(t *testing.T, env *atomicEnv) (*Service, *probeSettlementStore) {
	t.Helper()
	probe := &probeSettlementStore{Store: env.store}
	deps := env.serviceDeps
	deps.TaskLifecycle = probe
	return NewService(deps), probe
}

func assertProbeSequence(t *testing.T, probe *probeSettlementStore, want ...string) {
	t.Helper()
	got := probe.sequence()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("transaction steps = %v, want %v", got, want)
	}
}

// TestSettlementStagesNothingWhenTheCommitItselfFails is the decisive
// ordering leg. A staged event is the trigger the caller turns into a
// publish, so a stage that happened BEFORE the commit would announce a
// transition the failed commit erased. Forcing the commit to fail proves
// the stage is not on that side: nothing is staged, the row is untouched,
// and the deferred rollback runs.
func TestSettlementStagesNothingWhenTheCommitItselfFails(t *testing.T) {
	cases := []struct {
		name   string
		settle func(*Service, context.Context, string, string) error
	}{
		{
			name: "complete",
			settle: func(s *Service, ctx context.Context, ws, id string) error {
				_, err := s.Complete(ctx, ws, id, atomicRemoteOutput())
				return err
			},
		},
		{
			name: "fail",
			settle: func(s *Service, ctx context.Context, ws, id string) error {
				_, err := s.Fail(ctx, ws, id, "provider exploded")
				return err
			},
		},
		{
			name: "cancel",
			settle: func(s *Service, ctx context.Context, ws, id string) error {
				_, _, err := s.Cancel(ctx, ws, id, 1, "operator cancel")
				return err
			},
		},
		{
			name: "retry",
			settle: func(s *Service, ctx context.Context, ws, id string) error {
				_, err := s.Retry(ctx, ws, id, true)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := openAtomicEnv(t, 10)
			created := env.createPaidTask(t, 1)
			env.claimTask(t, created.ID, created.ExecutionToken)
			service, probe := newProbeService(t, env)
			probe.failNextCommit(errors.New("commit rejected"))

			if err := tc.settle(service, context.Background(), env.ws, created.ID); err == nil {
				t.Fatal("settlement must fail when the commit fails")
			}
			if staged := service.StagedEvents(); len(staged) != 0 {
				t.Fatalf("staged %d events for a failed commit, want 0 (a staged event must follow the commit, never precede it): %+v", len(staged), staged)
			}
			assertProbeSequence(t, probe, "begin", "commit-fail", "rollback")
			// The row never moved (the claim is the only committed fact), so
			// nothing downstream can have observed a transition that does
			// not exist.
			assertStagedEventIsCommitted(t, env, created.ID, StatusRunning)
		})
	}
}

// TestSettlementStagesOnlyAfterACommittedTransaction proves the positive
// half with the same probe: a staged event is always preceded by a
// successful commit on the settlement's own transaction.
func TestSettlementStagesOnlyAfterACommittedTransaction(t *testing.T) {
	env := openAtomicEnv(t, 10)
	created := env.createPaidTask(t, 1)
	env.claimTask(t, created.ID, created.ExecutionToken)
	service, probe := newProbeService(t, env)

	if _, err := service.Complete(context.Background(), env.ws, created.ID, atomicRemoteOutput()); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	assertProbeSequence(t, probe, "begin", "commit-ok")
	assertSingleStagedStatus(t, service, env, created.ID, StatusCompleted)
	assertStagedEventIsCommitted(t, env, created.ID, StatusCompleted)
}

// A submission carries the quote's timestamp through the quota check and
// the durable Task fact, even if the check runs across a calendar boundary.
type reservationTimeQuoter struct {
	*atomicQuoter
	at time.Time
}

func (q reservationTimeQuoter) QuoteInTx(ctx context.Context, tx pgx.Tx, service, resolution string, quantity int64) (BillingSnapshot, error) {
	snapshot, err := q.atomicQuoter.QuoteInTx(ctx, tx, service, resolution, quantity)
	snapshot.QuotedAt = q.at
	return snapshot, err
}

type reservationTimeChecks struct {
	quotaAt time.Time
}

func (c *reservationTimeChecks) CheckQuotaInTx(_ context.Context, _ pgx.Tx, _, _ int64, _ int64, reservedAt time.Time) error {
	c.quotaAt = reservedAt
	return nil
}

func TestSubmitPersistsTheReservationPeriodCheckedByQuota(t *testing.T) {
	// Organization scope: the quota check runs in the settlement (Personal
	// scopes skip it per #973/#974), so the reservation timestamp the
	// check observes must equal the persisted quoted_at.
	env := openQuotaOrgEnv(t)
	reservedAt := time.Date(2025, 1, 31, 23, 59, 59, 999000000, time.UTC)
	checks := &reservationTimeChecks{}
	deps := env.serviceDeps
	deps.Quoter = reservationTimeQuoter{atomicQuoter: deps.Quoter.(*atomicQuoter), at: reservedAt}
	deps.Quotas = checks
	env.service = NewService(deps)
	task := env.createPaidTask(t, 1)
	var persisted time.Time
	if err := env.pool.QueryRow(context.Background(), `SELECT quoted_at FROM task_billing_snapshots WHERE task_id=$1`, task.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if !checks.quotaAt.Equal(reservedAt) || !persisted.Equal(reservedAt) {
		t.Fatalf("reservation=%s quota=%s persisted=%s", reservedAt, checks.quotaAt, persisted)
	}
}
