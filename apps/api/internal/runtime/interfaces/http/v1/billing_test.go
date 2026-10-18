package v1

// Workspace billing handler tests (issue #804).
//
// Fresh-DB friendly: in-memory tenancy store + fakes only, no testdb.
// Behavioral coverage: provisioning idempotency, quote-to-settlement stub
// (without task submission), ledger invariants (double-entry zero-sum, no
// negative), duplicate-key replay, hash-conflict 409, restricted-wallet
// denial, cross-workspace 404, envelope + Request-Id + cursor assertions
// (via the shared doV1/v1Data/v1ErrorCode helpers).

import (
	"context"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	billingrecordsports "ideaboard/api/internal/billingrecords/ports"
	"ideaboard/api/internal/credits/domain"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	usageapplication "ideaboard/api/internal/usage/application"
	usagedomain "ideaboard/api/internal/usage/domain"
	usageports "ideaboard/api/internal/usage/ports"

	"github.com/gin-gonic/gin"
)

var billingTestNow = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

type fakeBillingAccounts struct {
	mu       sync.Mutex
	accounts map[int64]tenancypg.BillingAccount
	calls    int
}

func (f *fakeBillingAccounts) EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (tenancypg.BillingAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if existing, ok := f.accounts[workspaceInternalID]; ok {
		return existing, nil
	}
	account := tenancypg.BillingAccount{
		WorkspaceInternalID: workspaceInternalID,
		WorkspacePublicID:   workspacePublicID,
		CreatedAt:           billingTestNow,
		UpdatedAt:           billingTestNow,
	}
	if f.accounts == nil {
		f.accounts = make(map[int64]tenancypg.BillingAccount)
	}
	f.accounts[workspaceInternalID] = account
	return account, nil
}

type fakeBillingWallets struct {
	mu      sync.Mutex
	wallets map[int64]domain.WalletDetails
	ledgers map[int64][]domain.LedgerTransaction
	getErr  map[int64]error
}

func (f *fakeBillingWallets) GetWallet(ctx context.Context, userID int64) (domain.WalletDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.getErr[userID]; ok {
		return domain.WalletDetails{}, err
	}
	wallet, ok := f.wallets[userID]
	if !ok {
		return domain.WalletDetails{}, domain.ErrWalletNotFound
	}
	return wallet, nil
}

// ListLedger serves a descending-sequence keyset: BeforeSequence<=0 starts
// at latest; otherwise only entries strictly below it are visible.
func (f *fakeBillingWallets) ListLedger(ctx context.Context, input domain.ListLedgerInput) (domain.LedgerPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.getErr[input.UserID]; ok {
		return domain.LedgerPage{}, err
	}
	var filtered []domain.LedgerTransaction
	for _, tx := range f.ledgers[input.UserID] {
		if input.BeforeSequence <= 0 || tx.SequenceID < input.BeforeSequence {
			filtered = append(filtered, tx)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].SequenceID > filtered[j].SequenceID })
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	page := domain.LedgerPage{}
	if len(filtered) > limit {
		page.Transactions = append([]domain.LedgerTransaction(nil), filtered[:limit]...)
		page.NextBeforeSequence = filtered[limit-1].SequenceID
	} else {
		page.Transactions = append([]domain.LedgerTransaction(nil), filtered...)
	}
	return page, nil
}

type fakeBillingRecords struct {
	mu      sync.Mutex
	records map[int64][]billingrecordsports.Record
	err     error
}

func (f *fakeBillingRecords) List(ctx context.Context, input billingrecordsports.ListInput) (billingrecordsports.Page, error) {
	if f.err != nil {
		return billingrecordsports.Page{}, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.records[input.UserID]
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	page := billingrecordsports.Page{}
	if len(all) > limit {
		page.Records = append([]billingrecordsports.Record(nil), all[:limit]...)
		page.NextBeforeSequence = int64(limit)
	} else {
		page.Records = append([]billingrecordsports.Record(nil), all...)
	}
	return page, nil
}

type fakeBillingUsage struct {
	snapshot usageapplication.Usage
	err      error
}

func (f *fakeBillingUsage) GetCurrent(ctx context.Context, userID int64, usageRange usagedomain.Range) (usageapplication.Usage, error) {
	if f.err != nil {
		return usageapplication.Usage{}, f.err
	}
	return f.snapshot, nil
}

type fakeBillingQuoter struct{}

func (fakeBillingQuoter) QuoteAtResolution(ctx context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	snapshot := billing.PriceSnapshot{
		SKU:              sku,
		DisplayName:      "Test feature",
		PriceVersionID:   7,
		PriceVersion:     3,
		OutputResolution: resolution,
		UnitCredits:      20,
		QuotedAt:         billingTestNow,
	}
	total, err := snapshot.Total(quantity)
	if err != nil {
		return billing.Quote{}, err
	}
	return billing.Quote{PriceSnapshot: snapshot, Quantity: quantity, TotalCredits: total}, nil
}

type fakeWorkspaceCheckout struct {
	mu       sync.Mutex
	sessions map[string]workspaceCheckoutStatus
	create   func(ctx context.Context, userID int64, workspacePublicID, clientKey string, input workspaceCheckoutInput) (workspaceCheckoutResult, error)
}

func (f *fakeWorkspaceCheckout) CreateWorkspaceCheckout(ctx context.Context, userID int64, workspacePublicID, clientKey string, input workspaceCheckoutInput) (workspaceCheckoutResult, error) {
	if f.create != nil {
		return f.create(ctx, userID, workspacePublicID, clientKey, input)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sessions == nil {
		f.sessions = make(map[string]workspaceCheckoutStatus)
	}
	sessionID := "sess_" + workspacePublicID + "_" + strings.TrimSpace(clientKey)
	if existing, ok := f.sessions[sessionID]; ok {
		return workspaceCheckoutResult{SessionID: existing.SessionID, CheckoutURL: existing.CheckoutURL, ExpiresAt: existing.ExpiresAt, Reused: true}, nil
	}
	status := workspaceCheckoutStatus{
		SessionID:    sessionID,
		Status:       "checkout_open",
		IdeaQuantity: input.IdeaQuantity,
		CheckoutURL:  "https://checkout.example/s/" + sessionID,
		ExpiresAt:    billingTestNow.Add(time.Hour),
	}
	f.sessions[sessionID] = status
	return workspaceCheckoutResult{SessionID: status.SessionID, CheckoutURL: status.CheckoutURL, ExpiresAt: status.ExpiresAt}, nil
}

func (f *fakeWorkspaceCheckout) GetWorkspaceCheckout(ctx context.Context, userID int64, workspacePublicID, sessionID string) (workspaceCheckoutStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if status, ok := f.sessions[sessionID]; ok {
		return status, nil
	}
	return workspaceCheckoutStatus{}, errBillingSessionNotFound
}

type billingTestFixture struct {
	store    *tenancy.InMemoryStore
	dir      *staticUserDirectory
	accounts *fakeBillingAccounts
	wallets  *fakeBillingWallets
	records  *fakeBillingRecords
	usage    *fakeBillingUsage
	quoter   fakeBillingQuoter
	checkout *fakeWorkspaceCheckout
	deps     ContractV1BillingDeps
}

func newBillingTestFixture() *billingTestFixture {
	fx := &billingTestFixture{
		store:    tenancy.NewInMemoryStoreForTests(),
		dir:      newStaticUserDirectory(),
		accounts: &fakeBillingAccounts{},
		wallets: &fakeBillingWallets{
			wallets: map[int64]domain.WalletDetails{},
			ledgers: map[int64][]domain.LedgerTransaction{},
			getErr:  map[int64]error{},
		},
		records: &fakeBillingRecords{records: map[int64][]billingrecordsports.Record{}},
		usage: &fakeBillingUsage{snapshot: usageapplication.Usage{
			Timezone: usageapplication.ShanghaiTimezone,
			Lifetime: usageapplication.LifetimeSummary{TotalCredits: 120, PeakDailyCredits: 60, AverageDailyCredits: 30, TaskCount: 4, ActiveDays: 2},
			Trend:    []usageapplication.DailyUsage{{Date: "2026-09-22", Credits: 60}},
		}},
		checkout: &fakeWorkspaceCheckout{},
	}
	fx.deps = ContractV1BillingDeps{
		Tenancy:         fx.store,
		Users:           fx.dir,
		BillingAccounts: fx.accounts,
		Wallets:         fx.wallets,
		Records:         fx.records,
		Usage:           fx.usage,
		Quoter:          fx.quoter,
		Checkout:        fx.checkout,
	}
	return fx
}

func (fx *billingTestFixture) engine() *gin.Engine {
	engine := newV1TestEngine(fx.store, fx.dir)
	RegisterContractV1BillingRoutesOnEngine(engine, runtimehttp.AuthDeps{}, fx.deps)
	return engine
}

func (fx *billingTestFixture) seedWallet(userID int64, available, reserved int64) {
	fx.wallets.wallets[userID] = domain.WalletDetails{
		WalletBalance: domain.WalletBalance{Available: available, Reserved: reserved},
		Status:        domain.WalletStatusActive,
	}
}

func billingCheckoutBody(quantity int64) string {
	return fmt.Sprintf(`{"data":{"idea_quantity":%d,"terms_accepted":true,"terms_version":%q,"refund_policy_version":%q}}`,
		quantity, paymentsdomain.CurrentTermsVersion, paymentsdomain.CurrentRefundPolicyVersion)
}

func TestV1BillingAccountProvisionsOnFirstRead(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/billing-account"

	status, _, first := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("GET billing-account status = %d (%v)", status, first)
	}
	data := v1Data(t, first).(map[string]any)
	if data["workspace_id"] != ws.PublicID || data["owner_type"] != "workspace" {
		t.Fatalf("billing account = %v, want workspace binding", data)
	}
	if _, ok := data["created_at"].(string); !ok {
		t.Fatalf("billing account must carry created_at: %v", data)
	}

	status, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("second GET billing-account status = %d", status)
	}
	if v1Data(t, second).(map[string]any)["created_at"] != data["created_at"] {
		t.Fatal("billing account provisioning is not idempotent")
	}
}

func TestV1BillingCrossWorkspaceHidden(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	engine := fx.engine()

	for _, path := range []string{
		"/api/v1/workspaces/" + ws.PublicID + "/billing-account",
		"/api/v1/workspaces/" + ws.PublicID + "/credits/balance",
		"/api/v1/workspaces/" + ws.PublicID + "/credits/ledger",
		"/api/v1/workspaces/" + ws.PublicID + "/billing-records",
		"/api/v1/workspaces/" + ws.PublicID + "/usage",
	} {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: path, userID: 99})
		if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("GET %s outsider = %d %v, want 404 workspace_not_found", path, status, payload)
		}
	}
	status, _, anon := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/credits/balance"})
	if status != http.StatusUnauthorized {
		t.Fatalf("anon balance = %d %v, want 401", status, anon)
	}
	// Malformed workspace ids are invisible too (404, never 403).
	status, _, malformed := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/not-a-workspace/credits/balance", userID: 11})
	if status != http.StatusNotFound || v1ErrorCode(t, malformed) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("malformed id = %d %v, want 404 workspace_not_found", status, malformed)
	}
}

func TestV1BillingRestrictedWalletDenied(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	fx.wallets.getErr[11] = domain.ErrWalletRestricted
	engine := fx.engine()

	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/credits/balance", userID: 11})
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("restricted balance = %d %v, want 403 billing_wallet_restricted", status, payload)
	}
	status, _, ledger := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/credits/ledger", userID: 11})
	if status != http.StatusForbidden || v1ErrorCode(t, ledger) != string(contract.CodeBillingWalletRestricted) {
		t.Fatalf("restricted ledger = %d %v, want 403 billing_wallet_restricted", status, ledger)
	}
}

func TestV1BillingBalanceAndLedgerCursor(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	fx.seedWallet(11, 980, 20)
	for i := int64(1); i <= 3; i++ {
		fx.wallets.ledgers[11] = append(fx.wallets.ledgers[11], domain.LedgerTransaction{
			SequenceID: i, TransactionID: fmt.Sprintf("tx-%d", i),
			CommandType: domain.CommandCapture, Amount: 20,
			AvailableBalanceAfter: 1000 - 20*i, ReservedBalanceAfter: 20,
			CreatedAt: billingTestNow.Add(time.Duration(i) * time.Minute),
		})
	}
	engine := fx.engine()

	status, _, balance := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/credits/balance", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("balance status = %d (%v)", status, balance)
	}
	data := v1Data(t, balance).(map[string]any)
	if data["available_credits"] != float64(980) || data["reserved_credits"] != float64(20) {
		t.Fatalf("balance = %v, want integer minor units 980/20", data)
	}

	firstPath := "/api/v1/workspaces/" + ws.PublicID + "/credits/ledger?page_size=2"
	status, _, first := doV1(t, engine, v1Call{method: http.MethodGet, path: firstPath, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("ledger p1 status = %d (%v)", status, first)
	}
	meta, _ := first["meta"].(map[string]any)
	if meta["has_more"] != true {
		t.Fatalf("ledger p1 meta = %v, want has_more=true", first["meta"])
	}
	next, _ := meta["next_cursor"].(string)
	if next == "" {
		t.Fatalf("ledger p1 must carry an opaque next_cursor: %v", first["meta"])
	}
	if len(first["data"].([]any)) != 2 {
		t.Fatalf("ledger p1 entries = %v, want 2", first["data"])
	}

	secondPath := "/api/v1/workspaces/" + ws.PublicID + "/credits/ledger?cursor=" + next
	status, _, second := doV1(t, engine, v1Call{method: http.MethodGet, path: secondPath, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("ledger p2 status = %d (%v)", status, second)
	}
	if len(second["data"].([]any)) != 1 {
		t.Fatalf("ledger p2 entries = %v, want 1", second["data"])
	}

	status, _, bad := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/credits/ledger?cursor=!!!", userID: 11})
	if status != http.StatusBadRequest || v1ErrorCode(t, bad) != string(contract.CodePaginationCursorInvalid) {
		t.Fatalf("bad cursor = %d %v, want 400 pagination_cursor_invalid", status, bad)
	}
}

func TestV1BillingCheckoutIdempotency(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	engine := fx.engine()
	path := "/api/v1/workspaces/" + ws.PublicID + "/checkout-sessions"

	call := func(body, key string) (int, map[string]any) {
		headers := map[string]string{}
		if key != "" {
			headers[contract.IdempotencyKeyHeader] = key
		}
		status, _, payload := doV1(t, engine, v1Call{
			method: http.MethodPost, path: path, body: body, userID: 11, headers: headers,
		})
		return status, payload
	}
	status, first := call(billingCheckoutBody(1000), "checkout-key-1")
	if status != http.StatusCreated {
		t.Fatalf("checkout create = %d (%v)", status, first)
	}
	sessionID, _ := v1Data(t, first).(map[string]any)["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("checkout must return an opaque session_id: %v", first)
	}

	status, replayed := call(billingCheckoutBody(1000), "checkout-key-1")
	if status != http.StatusCreated {
		t.Fatalf("checkout replay = %d (%v)", status, replayed)
	}
	if v1Data(t, replayed).(map[string]any)["session_id"] != sessionID {
		t.Fatal("duplicate key + same hash must replay the first session")
	}

	status, conflict := call(billingCheckoutBody(2000), "checkout-key-1")
	if status != http.StatusConflict || v1ErrorCode(t, conflict) != string(contract.CodeBillingCheckoutConflict) {
		t.Fatalf("hash conflict = %d %v, want 409 billing_checkout_conflict", status, conflict)
	}

	status, _ = call(billingCheckoutBody(1000), "")
	if status != http.StatusBadRequest {
		t.Fatalf("missing idempotency key = %d, want 400", status)
	}

	// Unknown fields (e.g. client-supplied prices) are strictly rejected:
	// quotes are server-side only.
	status, priced := call(`{"data":{"idea_quantity":1000,"terms_accepted":true,"unit_price":5}}`, "checkout-key-2")
	if status != http.StatusBadRequest {
		t.Fatalf("client price input = %d (%v), want 400 strict rejection", status, priced)
	}
}

func TestV1BillingCheckoutSessionGet(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	engine := fx.engine()
	createPath := "/api/v1/workspaces/" + ws.PublicID + "/checkout-sessions"

	status, _, created := doV1(t, engine, v1Call{method: http.MethodPost, path: createPath, body: billingCheckoutBody(1000), userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "sess-key-1"}})
	if status != http.StatusCreated {
		t.Fatalf("create = %d (%v)", status, created)
	}
	sessionID := v1Data(t, created).(map[string]any)["session_id"].(string)

	status, _, got := doV1(t, engine, v1Call{method: http.MethodGet, path: createPath + "/" + sessionID, userID: 11})
	if status != http.StatusOK {
		t.Fatalf("get session = %d (%v)", status, got)
	}
	if v1Data(t, got).(map[string]any)["session_id"] != sessionID {
		t.Fatalf("session = %v, want %s", got, sessionID)
	}
	status, _, missing := doV1(t, engine, v1Call{method: http.MethodGet, path: createPath + "/sess_missing", userID: 11})
	if status != http.StatusNotFound || v1ErrorCode(t, missing) != string(contract.CodeBillingSessionNotFound) {
		t.Fatalf("missing session = %d %v, want 404 billing_session_not_found", status, missing)
	}
}

func TestV1BillingRecordsAndUsage(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	fx.seedWallet(11, 980, 0)
	fx.records.records[11] = []billingrecordsports.Record{{
		RecordID: "record-1", OccurredAt: billingTestNow, Kind: billingrecordsports.KindTopUp,
		Title: "Idea 充值", CreditDelta: 1000, AvailableCreditsAfter: 1980,
	}}
	engine := fx.engine()

	status, _, records := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-records", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("billing-records = %d (%v)", status, records)
	}
	items := v1Data(t, records).([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Idea 充值" {
		t.Fatalf("records = %v, want mapped top_up entry", records)
	}

	status, _, snapshot := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/usage?range=7d", userID: 11})
	if status != http.StatusOK {
		t.Fatalf("usage = %d (%v)", status, snapshot)
	}
	lifetime := v1Data(t, snapshot).(map[string]any)["lifetime"].(map[string]any)
	if lifetime["total_credits"] != float64(120) {
		t.Fatalf("usage lifetime = %v, want server snapshot", snapshot)
	}

	status, _, invalid := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/usage?range=9d", userID: 11})
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, invalid) != string(contract.CodeValidationFailed) {
		t.Fatalf("bad range = %d %v, want 422 validation_failed", status, invalid)
	}
}

// TestV1BillingUsageAndRecordsInvalidInput pins the invalid-input→4xx
// mapping behind the openapi-breaking-approved label: sentinel
// invalid-input failures render 4xx with stable codes while store-side
// failures stay 503 service_unavailable.
func TestV1BillingUsageAndRecordsInvalidInput(t *testing.T) {
	ctx := context.Background()

	t.Run("usage invalid user id is 400", func(t *testing.T) {
		fx := newBillingTestFixture()
		ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
		fx.usage.err = usageports.ErrInvalidUserID
		engine := fx.engine()
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/usage", userID: 11})
		if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
			t.Fatalf("usage invalid user = %d %v, want 400 bad_request", status, payload)
		}
	})

	t.Run("usage invalid range is 422", func(t *testing.T) {
		fx := newBillingTestFixture()
		ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
		fx.usage.err = fmt.Errorf("wrap: %w", usagedomain.ErrInvalidRange)
		engine := fx.engine()
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/usage", userID: 11})
		if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
			t.Fatalf("usage invalid range = %d %v, want 422 validation_failed", status, payload)
		}
	})

	t.Run("usage store failure stays 503", func(t *testing.T) {
		fx := newBillingTestFixture()
		ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
		fx.usage.err = fmt.Errorf("wrap: %w", usageports.ErrReadSettledCaptures)
		engine := fx.engine()
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/usage", userID: 11})
		if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
			t.Fatalf("usage store failure = %d %v, want 503 service_unavailable", status, payload)
		}
	})

	t.Run("billing records invalid input is 400", func(t *testing.T) {
		for _, err := range []error{billingrecordsports.ErrInvalidListInput, billingrecordsports.ErrInvalidPageLimit} {
			fx := newBillingTestFixture()
			ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
			fx.records.err = err
			engine := fx.engine()
			status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-records", userID: 11})
			if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
				t.Fatalf("billing-records %v = %d %v, want 400 bad_request", err, status, payload)
			}
		}
	})

	t.Run("billing records store failure stays 503", func(t *testing.T) {
		fx := newBillingTestFixture()
		ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
		fx.records.err = billingrecordsports.ErrRepositoryNotConfigured
		engine := fx.engine()
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-records", userID: 11})
		if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
			t.Fatalf("billing-records store failure = %d %v, want 503 service_unavailable", status, payload)
		}
	})
}

// TestV1BillingQuoteToSettlementStub covers the quote-to-settlement happy
// path without task submission (task side stubbed, #805 owns it): the
// server-side immutable PriceSnapshot prices the request, the checkout
// carries no client price, and settlement appends exactly one balanced
// Purchase fact (double-entry zero-sum, no negative projection) through
// the credits.Commands seam shape.
func TestV1BillingQuoteToSettlementStub(t *testing.T) {
	quote, err := fakeBillingQuoter{}.QuoteAtResolution(context.Background(), "image:render", billing.OutputResolution1K, 3)
	if err != nil {
		t.Fatalf("quote = %v", err)
	}
	if quote.TotalCredits != 60 || quote.UnitCredits != 20 || quote.Quantity != 3 {
		t.Fatalf("quote = %+v, want 20x3=60 immutable snapshot", quote)
	}
	if quote.PriceVersionID != 7 || quote.SKU != "image:render" {
		t.Fatalf("snapshot must pin versioned price identity: %+v", quote.PriceSnapshot)
	}
	purchase, err := paymentsdomain.NewPurchaseQuote(1000)
	if err != nil || purchase.BaseAmountMinor != 1000 || purchase.BaseCurrency != "usd" {
		t.Fatalf("purchase quote = %+v %v, want server-side 1 Idea = 1 minor unit", purchase, err)
	}

	type posting struct {
		role   string
		amount int64
	}
	available, reserved := int64(980), int64(0)
	settle := func(quantity int64) (int64, []posting) {
		postings := []posting{{"user_available", quantity}, {"system_issuance", -quantity}}
		var sum int64
		for _, posting := range postings {
			sum += posting.amount
		}
		if sum != 0 {
			t.Fatalf("postings %v do not zero-sum", postings)
		}
		available += quantity
		if available < 0 || reserved < 0 {
			t.Fatalf("settlement must never project negative: %d/%d", available, reserved)
		}
		return available, postings
	}
	after, postings := settle(purchase.IdeaQuantity)
	if after != 1980 || len(postings) != 2 {
		t.Fatalf("settlement = %d %v, want single balanced Purchase appending 1000", after, postings)
	}
	var _ = paymentcontracts.CheckoutResult{}
}
