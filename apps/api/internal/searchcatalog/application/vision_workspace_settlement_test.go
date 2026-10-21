// Vision Search workspace settlement focused tests (#918).
//
// The settlement triple (submitter user_id, consumption workspace_id,
// payer billing_account_id) resolves through the stable #915 chain inside
// the reservation transaction; Hold/Capture/Release and background
// reconciliation converge on the STORED payer and never re-resolve from
// the live Owner or membership. Organization pools and explicit no-org
// links share that one chain. Idempotency binds account + resource scope:
// cross-workspace, cross-account, cross-operator, and cross-body key reuse
// conflicts instead of replaying.
//
// PostgreSQL integration tests SKIP without API_TEST_DATABASE_DSN (house
// convention); the hash-binding unit test always runs.

package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	billingpostgres "ideaboard/api/internal/billing/infrastructure/postgres"
	"ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	searchdomain "ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/searchcatalog/ports"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// staticVisionAccounts is the test-only settlement identity double: the
// deferred-reserve test and unit callers inject it instead of the
// production #915 chain. Production always uses
// visionBillingAccountResolver.
type staticVisionAccounts struct {
	workspaceInternalID int64
	billingAccountID    int64
	err                 error
}

func (s staticVisionAccounts) ResolveVisionSettlementInTx(_ context.Context, _ pgx.Tx, _ string, _ int64) (VisionSettlementIdentity, error) {
	if s.err != nil {
		return VisionSettlementIdentity{}, s.err
	}
	return VisionSettlementIdentity{WorkspaceInternalID: s.workspaceInternalID, BillingAccountID: s.billingAccountID}, nil
}

func (s staticVisionAccounts) ResolveWorkspaceInternalID(_ context.Context, _ pgx.Tx, _ string) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.workspaceInternalID, nil
}

func TestHashVisionRequestBindsScope(t *testing.T) {
	input := searchdomain.VisionInput{UserText: "  sunset   beach ", Images: []searchdomain.VisionImage{{Filename: "a.png", ContentType: "image/png", Data: []byte{1, 2, 3}}}}
	base := hashVisionRequest("ws_scope0001", 7, input)
	if len(base) != 64 {
		t.Fatalf("hashVisionRequest length = %d, want 64 hex", len(base))
	}
	if got := hashVisionRequest("ws_scope0001", 7, input); got != base {
		t.Fatal("hashVisionRequest unstable across identical scope")
	}
	// Same input, different workspace / operator: must not collide.
	if got := hashVisionRequest("ws_scope0002", 7, input); got == base {
		t.Fatal("hashVisionRequest collides across workspaces")
	}
	if got := hashVisionRequest("ws_scope0001", 8, input); got == base {
		t.Fatal("hashVisionRequest collides across operators")
	}
	other := input
	other.UserText = "mountain lake"
	if got := hashVisionRequest("ws_scope0001", 7, other); got == base {
		t.Fatal("hashVisionRequest collides across bodies")
	}
	// The input-only hash stays the scope-free building block.
	if hashVisionInput(input) == hashVisionInput(other) {
		t.Fatal("hashVisionInput collides across distinct inputs")
	}
}

type visionWorkspaceEnv struct {
	pool    *pgxpool.Pool
	credits *credits.Service
	catalog *billingapplication.BillableItemCatalog
}

func visionTestDefinition() billing.BillableItemDefinition {
	return billing.BillableItemDefinition{
		SKU:             visionSearchSKU,
		Namespace:       "ai-search",
		ProductKind:     "search-feature",
		DisplayName:     "vision",
		PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
	}
}

func openVisionWorkspaceEnv(t *testing.T, unitCost int64) *visionWorkspaceEnv {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	definition := visionTestDefinition()
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{definition})
	if err != nil {
		t.Fatal(err)
	}
	billingStore := billingpostgres.NewStore(pool)
	catalog := billingapplication.NewBillableItemCatalog(registry, billingStore)
	if _, err := billingStore.PublishPrice(ctx, billing.PublishPriceInput{Definition: definition, UnitCredits: unitCost, ActorID: 1}); err != nil {
		t.Fatalf("publish vision price: %v", err)
	}
	return &visionWorkspaceEnv{pool: pool, credits: creditService, catalog: catalog}
}

func timeNowUnixNano() int64 { return time.Now().UnixNano() }

var visionTestUIDCounter uint64

func visionTestStorageUID() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	n := atomic.AddUint64(&visionTestUIDCounter, 1) + uint64(timeNowUnixNano())
	buf := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		buf[i] = alphabet[n%36]
		n /= 36
	}
	return string(buf)
}

func (e *visionWorkspaceEnv) createUser(t *testing.T, name string) int64 {
	t.Helper()
	storageUID := visionTestStorageUID()
	var userID int64
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (display_name, storage_uid, role, status) VALUES ($1, $2, 'user', 1) RETURNING id`,
		name, storageUID,
	).Scan(&userID); err != nil {
		t.Fatalf("seed vision user: %v", err)
	}
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1) ON CONFLICT DO NOTHING`, storageUID); err != nil {
		t.Fatalf("seed vision user uid: %v", err)
	}
	return userID
}

func (e *visionWorkspaceEnv) createOrgWorkspace(t *testing.T, orgPublic, wsPublic string, members map[int64]string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO workspaces (public_id, organization_id, name)
		SELECT $1, internal_id, $2 FROM organizations WHERE public_id = $3`, wsPublic, "vision-"+wsPublic, orgPublic); err != nil {
		t.Fatalf("seed org workspace: %v", err)
	}
	var workspaceInternalID int64
	if err := e.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, wsPublic).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve org workspace: %v", err)
	}
	for userID, role := range members {
		if _, err := e.pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, workspaceInternalID, userID, role); err != nil {
			t.Fatalf("seed org membership: %v", err)
		}
	}
}

func (e *visionWorkspaceEnv) createPersonalWorkspace(t *testing.T, wsPublic string, owner int64, members map[int64]string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO workspaces (public_id, owner_user_id, name) VALUES ($1, $2, $3)`, wsPublic, owner, "vision-"+wsPublic); err != nil {
		t.Fatalf("seed personal workspace: %v", err)
	}
	var workspaceInternalID int64
	if err := e.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, wsPublic).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve personal workspace: %v", err)
	}
	all := map[int64]string{owner: "owner"}
	for userID, role := range members {
		all[userID] = role
	}
	for userID, role := range all {
		if _, err := e.pool.Exec(ctx, `INSERT INTO workspace_memberships (workspace_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, workspaceInternalID, userID, role); err != nil {
			t.Fatalf("seed personal membership: %v", err)
		}
	}
}

func (e *visionWorkspaceEnv) createOrg(t *testing.T, orgPublic string, members map[int64]string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `INSERT INTO organizations (public_id, name) VALUES ($1, $2)`, orgPublic, "vision-"+orgPublic); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	var orgInternalID int64
	if err := e.pool.QueryRow(ctx, `SELECT internal_id FROM organizations WHERE public_id = $1`, orgPublic).Scan(&orgInternalID); err != nil {
		t.Fatalf("resolve org: %v", err)
	}
	for userID, role := range members {
		if _, err := e.pool.Exec(ctx, `INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, orgInternalID, userID, role); err != nil {
			t.Fatalf("seed org membership: %v", err)
		}
	}
}

func (e *visionWorkspaceEnv) billingAccountFor(t *testing.T, wsPublic string) int64 {
	t.Helper()
	ctx := context.Background()
	var workspaceInternalID int64
	if err := e.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, wsPublic).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve workspace for billing: %v", err)
	}
	account, err := tenancypg.NewBillingAccountStore(e.pool).EnsureBillingAccountForWorkspace(ctx, workspaceInternalID, wsPublic)
	if err != nil {
		t.Fatalf("resolve billing account: %v", err)
	}
	return account.InternalID
}

func (e *visionWorkspaceEnv) fundAccount(t *testing.T, accountID int64, amount int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := e.credits.LifecycleInTx(tx).ProvisionAccountWallet(ctx, accountID); err != nil {
		t.Fatalf("provision vision wallet: %v", err)
	}
	grantAmount, err := creditsdomain.NewCreditAmount(amount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.credits.InTx(tx).Grant(ctx, creditsports.GrantCommand{BillingAccountID: accountID, Amount: grantAmount, Metadata: creditsdomain.CommandMetadata{
		IdempotencyKey: fmt.Sprintf("vision-seed-%d-%d", accountID, timeNowUnixNano()%1000000),
		Actor:          creditsdomain.Actor{Type: creditsdomain.ActorSystem, ID: "test"},
		Source:         "test", Reason: "vision settlement seed",
		BusinessReference: creditsdomain.BusinessReference{Type: "test", ID: "vision-seed"},
	}}); err != nil {
		t.Fatalf("grant vision funds: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (e *visionWorkspaceEnv) serviceWith(client *visionClientStub) *VisionBillingService {
	return NewVisionBillingService(e.pool, e.catalog, e.credits,
		newVisionSearchService(visionConfigStub{config: ports.VisionProviderConfig{Model: "test-model"}}, client))
}

func visionTestInput() searchdomain.VisionInput {
	return searchdomain.VisionInput{Images: []searchdomain.VisionImage{validVisionImage()}, UserText: "dunes at dusk"}
}

type visionOperationRow struct {
	userID    int64
	workspace int64
	account   int64
	status    string
	unit      int64
	holdID    string
	query     string
	errorCode string
}

func (e *visionWorkspaceEnv) operationRow(t *testing.T, operationID string) visionOperationRow {
	t.Helper()
	var row visionOperationRow
	if err := e.pool.QueryRow(context.Background(), `
		SELECT user_id, workspace_id, billing_account_id, status, unit_credits, hold_transaction_id, query, error_code
		FROM ai_search_operations WHERE operation_id = $1::uuid`,
		operationID).Scan(&row.userID, &row.workspace, &row.account, &row.status, &row.unit, &row.holdID, &row.query, &row.errorCode); err != nil {
		t.Fatalf("read vision operation: %v", err)
	}
	return row
}

func (e *visionWorkspaceEnv) ledgerCount(t *testing.T, commandType string, accountID int64) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM credit_transactions WHERE billing_account_id = $1 AND command_type = $2`, accountID, commandType).Scan(&count); err != nil {
		t.Fatalf("count vision ledger: %v", err)
	}
	return count
}

func (e *visionWorkspaceEnv) operationCount(t *testing.T, wsPublic, key string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM ai_search_operations o
		WHERE o.idempotency_key = $1 AND o.workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $2)`, key, wsPublic).Scan(&count); err != nil {
		t.Fatalf("count vision operations: %v", err)
	}
	return count
}

// TestVisionWorkspaceSettlementTriple proves the #918 happy path on the
// organization chain: two workspaces sharing the org pool bill one payer,
// the triple persists (submitter, workspace, account), the wallet refresh
// targets the payer, and the operator's personal account stays untouched.
func TestVisionWorkspaceSettlementTriple(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-triple-owner")
	member := env.createUser(t, "vision-triple-member")
	orgPublic := "org_visiontriple01"
	wsA, wsB := "ws_visiontripleA01", "ws_visiontripleB01"
	env.createOrg(t, orgPublic, map[int64]string{owner: "owner", member: "member"})
	env.createOrgWorkspace(t, orgPublic, wsA, map[int64]string{owner: "owner", member: "member"})
	env.createOrgWorkspace(t, orgPublic, wsB, map[int64]string{owner: "owner", member: "member"})

	accountA := env.billingAccountFor(t, wsA)
	accountB := env.billingAccountFor(t, wsB)
	if accountA != accountB {
		t.Fatalf("org workspaces bill %d vs %d, want one shared pool", accountA, accountB)
	}
	shared := accountA
	env.fundAccount(t, shared, 1000)

	var personal int64
	if err := env.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, member).Scan(&personal); err != nil {
		t.Fatalf("resolve personal account: %v", err)
	}
	if personal == shared {
		t.Fatal("test setup collides: personal account equals the org pool")
	}

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "  warm   cafe "}})
	first, err := service.Extract(ctx, wsA, member, "vision-triple-1", visionTestInput())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if first.Status != string(VisionOperationSucceeded) || first.Query != "warm cafe" || first.Replayed {
		t.Fatalf("first = %+v, want succeeded normalized replayed=false", first)
	}
	row := env.operationRow(t, first.OperationID)
	if row.userID != member || row.account != shared || row.status != "succeeded" || row.unit != 7 || row.holdID == "" {
		t.Fatalf("operation row = %+v, want submitter+workspace payer triple held", row)
	}
	var wsInternal int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, wsA).Scan(&wsInternal); err != nil {
		t.Fatal(err)
	}
	if row.workspace != wsInternal {
		t.Fatalf("operation workspace = %d, want %d", row.workspace, wsInternal)
	}

	// Same org, second workspace: the same payer converges without a new
	// wallet, and the balance refresh reads that payer.
	second, err := service.Extract(ctx, wsB, member, "vision-triple-2", visionTestInput())
	if err != nil {
		t.Fatalf("second Extract() error = %v", err)
	}
	if env.operationRow(t, second.OperationID).account != shared {
		t.Fatal("second workspace did not bill the shared pool")
	}
	if env.ledgerCount(t, "hold", shared) != 2 || env.ledgerCount(t, "capture", shared) != 2 {
		t.Fatalf("shared ledger holds=%d captures=%d, want 2/2",
			env.ledgerCount(t, "hold", shared), env.ledgerCount(t, "capture", shared))
	}
	if env.ledgerCount(t, "hold", personal) != 0 || env.ledgerCount(t, "capture", personal) != 0 {
		t.Fatal("personal wallet was charged for workspace consumption")
	}
	wallet, err := env.credits.GetWallet(ctx, shared)
	if err != nil {
		t.Fatalf("read shared wallet: %v", err)
	}
	if wallet.Available != 1000-14 || wallet.Reserved != 0 {
		t.Fatalf("shared wallet available=%d reserved=%d, want 986/0", wallet.Available, wallet.Reserved)
	}
}

// TestVisionWorkspaceExplicitLinkSharesOneChain proves the no-org chain:
// two personal workspaces with an explicit link bill the same account,
// exactly like the organization pool above.
func TestVisionWorkspaceExplicitLinkSharesOneChain(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 5)
	ctx := context.Background()
	owner := env.createUser(t, "vision-link-owner")
	member := env.createUser(t, "vision-link-member")
	wsA, wsB := "ws_visionlinkA01", "ws_visionlinkB01"
	env.createPersonalWorkspace(t, wsA, owner, map[int64]string{member: "member"})
	env.createPersonalWorkspace(t, wsB, owner, map[int64]string{member: "member"})

	accountA := env.billingAccountFor(t, wsA)
	var wsBInternal int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, wsB).Scan(&wsBInternal); err != nil {
		t.Fatal(err)
	}
	// Explicit studio link (permission checks belong to #915 tests; the
	// settlement must treat the linked account as the stable payer).
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_billing_links (workspace_id, billing_account_id) VALUES ($1, $2) ON CONFLICT (workspace_id) DO UPDATE SET billing_account_id = EXCLUDED.billing_account_id`, wsBInternal, accountA); err != nil {
		t.Fatalf("link workspaces: %v", err)
	}
	env.fundAccount(t, accountA, 500)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "linked dunes"}})
	got, err := service.Extract(ctx, wsB, member, "vision-link-1", visionTestInput())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if row := env.operationRow(t, got.OperationID); row.account != accountA || row.status != "succeeded" {
		t.Fatalf("linked operation = %+v, want payer %d succeeded", row, accountA)
	}
	if env.ledgerCount(t, "capture", accountA) != 1 {
		t.Fatal("linked workspace did not capture on the shared account")
	}
}

// TestVisionWorkspaceFailureReleasesOriginalAccount proves deterministic
// provider failures Release the same payer's Hold and persist the failure.
func TestVisionWorkspaceFailureReleasesOriginalAccount(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-fail-owner")
	member := env.createUser(t, "vision-fail-member")
	ws := "ws_visionfail01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)
	env.fundAccount(t, account, 1000)

	service := env.serviceWith(&visionClientStub{err: &searchdomain.VisionProviderError{Kind: searchdomain.VisionProviderFailureCredentials, Err: errors.New("bad key")}})
	_, err := service.Extract(ctx, ws, member, "vision-fail-1", visionTestInput())
	failure, ok := err.(*VisionOperationFailure)
	if !ok || failure == nil {
		t.Fatalf("Extract() error = %v, want *VisionOperationFailure", err)
	}
	if failure.HTTPStatus != 502 {
		t.Fatalf("failure status = %d, want 502", failure.HTTPStatus)
	}
	var status, errorCode string
	var storedAccount int64
	if err := env.pool.QueryRow(ctx, `SELECT status, error_code, billing_account_id FROM ai_search_operations WHERE user_id = $1 AND idempotency_key = $2`,
		member, "vision-fail-1").Scan(&status, &errorCode, &storedAccount); err != nil {
		t.Fatalf("read failed operation: %v", err)
	}
	if status != "failed" || storedAccount != account {
		t.Fatalf("failed operation status=%s account=%d, want failed/%d", status, storedAccount, account)
	}
	if env.ledgerCount(t, "hold", account) != 1 || env.ledgerCount(t, "release", account) != 1 || env.ledgerCount(t, "capture", account) != 0 {
		t.Fatal("failure did not hold+release exactly once on the original payer")
	}
	wallet, err := env.credits.GetWallet(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.Available != 1000 || wallet.Reserved != 0 {
		t.Fatalf("wallet after release available=%d reserved=%d, want 1000/0", wallet.Available, wallet.Reserved)
	}
}

// TestVisionWorkspaceIdempotencyBinding proves replay stays inside the
// scope that created it: same key+scope replays, any scope divergence
// (body, workspace, operator, payer) conflicts, and nobody replays
// another scope's operation.
func TestVisionWorkspaceIdempotencyBinding(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-idem-owner")
	member := env.createUser(t, "vision-idem-member")
	other := env.createUser(t, "vision-idem-other")
	wsA, wsB := "ws_visionidemA01", "ws_visionidemB01"
	env.createPersonalWorkspace(t, wsA, owner, map[int64]string{member: "member", other: "member"})
	env.createPersonalWorkspace(t, wsB, owner, map[int64]string{member: "member"})
	env.fundAccount(t, env.billingAccountFor(t, wsA), 1000)
	env.fundAccount(t, env.billingAccountFor(t, wsB), 1000)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "dunes"}})
	input := visionTestInput()
	first, err := service.Extract(ctx, wsA, member, "vision-idem-1", input)
	if err != nil {
		t.Fatalf("first Extract() error = %v", err)
	}
	holdsBefore := env.ledgerCount(t, "hold", env.billingAccountFor(t, wsA))

	// Same key + same scope: replay, no new ledger row.
	replay, err := service.Extract(ctx, wsA, member, "vision-idem-1", input)
	if err != nil {
		t.Fatalf("replay Extract() error = %v", err)
	}
	if !replay.Replayed || replay.OperationID != first.OperationID {
		t.Fatalf("replay = %+v, want same operation replayed", replay)
	}
	accountA := env.billingAccountFor(t, wsA)
	if env.ledgerCount(t, "hold", accountA) != holdsBefore {
		t.Fatal("replay wrote a second hold")
	}

	// Same key + different body: conflict.
	divergent := input
	divergent.UserText = "mountain lake"
	if _, err := service.Extract(ctx, wsA, member, "vision-idem-1", divergent); !errors.Is(err, ErrVisionIdempotencyConflict) {
		t.Fatalf("body-divergent error = %v, want idempotency conflict", err)
	}

	// Same key + same body, different workspace: conflict (never replay).
	if _, err := service.Extract(ctx, wsB, member, "vision-idem-1", input); !errors.Is(err, ErrVisionIdempotencyConflict) {
		t.Fatalf("cross-workspace error = %v, want idempotency conflict", err)
	}

	// Same key + same workspace, different operator: conflict (never
	// replay another member's operation).
	if _, err := service.Extract(ctx, wsA, other, "vision-idem-1", input); !errors.Is(err, ErrVisionIdempotencyConflict) {
		t.Fatalf("cross-operator error = %v, want idempotency conflict", err)
	}
}

// TestVisionWorkspaceConcurrency proves concurrent retries of one user
// action converge to a single operation and a single Hold.
func TestVisionWorkspaceConcurrency(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-conc-owner")
	member := env.createUser(t, "vision-conc-member")
	ws := "ws_visionconc01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)
	env.fundAccount(t, account, 1000)

	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "dunes"}})
	const callers = 8
	results := make([]VisionBillingResult, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.Extract(ctx, ws, member, "vision-conc-1", visionTestInput())
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d error = %v", i, err)
		}
		if results[i].OperationID != results[0].OperationID {
			t.Fatalf("caller %d operation %s diverges from %s", i, results[i].OperationID, results[0].OperationID)
		}
	}
	if env.operationCount(t, ws, "vision-conc-1") != 1 {
		t.Fatal("concurrent reserves created more than one operation")
	}
	if env.ledgerCount(t, "hold", account) != 1 {
		t.Fatalf("concurrent holds = %d, want exactly 1", env.ledgerCount(t, "hold", account))
	}
}

// TestVisionWorkspaceTimeoutRecoveryUsesOriginalAccount proves uncertain
// outcomes park for reconciliation and converge on the ORIGINAL payer even
// after the operator loses workspace standing and the billing management
// hands over: reconciliation never re-resolves the payer.
func TestVisionWorkspaceTimeoutRecoveryUsesOriginalAccount(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-rec-owner")
	member := env.createUser(t, "vision-rec-member")
	successor := env.createUser(t, "vision-rec-successor")
	ws := "ws_visionrec01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)
	env.fundAccount(t, account, 1000)

	uncertain := env.serviceWith(&visionClientStub{err: &searchdomain.VisionProviderError{Kind: searchdomain.VisionProviderFailureTimeout, Err: errors.New("gateway timeout")}})
	pending, err := uncertain.Extract(ctx, ws, member, "vision-rec-1", visionTestInput())
	var pendingErr *VisionBillingPending
	if !errors.As(err, &pendingErr) || pending.Status != string(VisionOperationReconciliationPending) {
		t.Fatalf("uncertain = %+v/%v, want reconciliation_pending", pending, err)
	}

	// Membership and management churn AFTER the Hold: the spender leaves
	// the workspace and billing ownership hands over. Settlement must not
	// follow either fact.
	var wsInternal int64
	if err := env.pool.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, ws).Scan(&wsInternal); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id = $1 AND user_id = $2`, wsInternal, member); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO billing_account_members (billing_account_id, user_id, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`, account, successor); err != nil {
		t.Fatal(err)
	}
	// Age the operation past the reconciliation horizon.
	if _, err := env.pool.Exec(ctx, `UPDATE ai_search_operations SET created_at = now() - interval '20 minutes' WHERE operation_id = $1::uuid`, pending.OperationID); err != nil {
		t.Fatal(err)
	}

	reconciler := NewVisionBillingService(env.pool, env.catalog, env.credits, &VisionSearchService{})
	if err := reconciler.ReconcilePending(ctx); err != nil {
		t.Fatalf("ReconcilePending() error = %v", err)
	}
	row := env.operationRow(t, pending.OperationID)
	if row.status != "failed" || row.errorCode != "vision_search_reconciliation_timeout" || row.account != account {
		t.Fatalf("reconciled = %+v, want failed/timeout on payer %d", row, account)
	}
	if env.ledgerCount(t, "hold", account) != 1 || env.ledgerCount(t, "release", account) != 1 || env.ledgerCount(t, "capture", account) != 0 {
		t.Fatal("reconciliation did not release the original hold exactly once")
	}
	wallet, err := env.credits.GetWallet(ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.Available != 1000 || wallet.Reserved != 0 {
		t.Fatalf("wallet after reconciliation available=%d reserved=%d, want 1000/0", wallet.Available, wallet.Reserved)
	}
}

// TestVisionWorkspacePermissionAndFundsGate proves search permission,
// insufficient-funds feedback, and payer isolation: outsiders see only
// workspace_not_found (never a billing fact), underfunded calls fail on
// the payer with no operation row and no personal-wallet fallback.
func TestVisionWorkspacePermissionAndFundsGate(t *testing.T) {
	env := openVisionWorkspaceEnv(t, 7)
	ctx := context.Background()
	owner := env.createUser(t, "vision-gate-owner")
	member := env.createUser(t, "vision-gate-member")
	outsider := env.createUser(t, "vision-gate-outsider")
	ws := "ws_visiongate01"
	env.createPersonalWorkspace(t, ws, owner, map[int64]string{member: "member"})
	account := env.billingAccountFor(t, ws)

	// Outsider: invisible workspace, no billing signal.
	service := env.serviceWith(&visionClientStub{extraction: searchdomain.VisionExtraction{Query: "dunes"}})
	if _, err := service.Extract(ctx, ws, outsider, "vision-gate-1", visionTestInput()); !errors.Is(err, ErrVisionWorkspaceNotFound) {
		t.Fatalf("outsider error = %v, want workspace_not_found", err)
	}
	if env.operationCount(t, ws, "vision-gate-1") != 0 {
		t.Fatal("outsider call persisted an operation")
	}

	// Underfunded payer: Hold fails with insufficient credits on the
	// payer; nothing persists and no other wallet moves.
	env.fundAccount(t, account, 3)
	if _, err := service.Extract(ctx, ws, member, "vision-gate-2", visionTestInput()); !errors.Is(err, creditsdomain.ErrInsufficientCredits) {
		t.Fatalf("underfunded error = %v, want insufficient credits", err)
	}
	if env.operationCount(t, ws, "vision-gate-2") != 0 {
		t.Fatal("underfunded call persisted an operation")
	}
	var personal int64
	if err := env.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, member).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	if env.ledgerCount(t, "hold", personal) != 0 {
		t.Fatal("underfunded call fell back to the personal wallet")
	}
}
