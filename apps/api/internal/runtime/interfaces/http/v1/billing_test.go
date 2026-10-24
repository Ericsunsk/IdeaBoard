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

// fakeBillingAccounts resolves stable billing accounts by creation scope
// (ADR-0038): personal workspaces of one holder share the holder's
// Personal survivor, organization workspaces share the org pool. There is
// no per-workspace mint and no link indirection: the scope key derives
// from the fixture tenancy store on every Ensure, mirroring the
// production resolution order (org pool first, else holder survivor;
// ownerless personal scopes fail closed).
type fakeBillingAccounts struct {
	mu    sync.Mutex
	store *tenancy.InMemoryStore
	// accounts is keyed by stable billing-account internal ID (the
	// anchoring workspace's internal ID); byScope maps a scope key to
	// that account ID so every workspace in one scope resolves one row.
	accounts  map[int64]tenancy.BillingAccount
	byScope   map[string]int64
	byPublic  map[string]int64
	roles     map[int64]map[int64]string
	deny      map[int64]bool
	calls     int
	invites   map[string]*tenancy.BillingTransferInvite
	inviteSeq int64
}

// billingScopeOf derives the ADR-0038 funds scope for a fixture workspace:
// "org:<organization public id>" for organization workspaces (shared org
// pool, OwnerType organization), "personal:<holder user id>" for personal
// ones (holder survivor, OwnerType workspace). Unknown workspaces and
// ownerless personal scopes fail closed like production.
func (f *fakeBillingAccounts) billingScopeOf(ctx context.Context, workspacePublicID string) (scope, ownerType string, shared bool, err error) {
	if f.store == nil {
		return "", "", false, tenancy.ErrBillingWorkspaceInvalid
	}
	ws, err := f.store.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return "", "", false, tenancy.ErrBillingWorkspaceInvalid
	}
	if ws.OrganizationID != nil {
		return "org:" + *ws.OrganizationID, tenancypg.BillingOwnerOrganization, true, nil
	}
	if ws.OwnerUserID == nil || *ws.OwnerUserID <= 0 {
		return "", "", false, tenancy.ErrBillingConflict
	}
	return fmt.Sprintf("personal:%d", *ws.OwnerUserID), tenancypg.BillingOwnerWorkspace, false, nil
}

func (f *fakeBillingAccounts) EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (tenancy.BillingAccount, error) {
	scope, ownerType, shared, err := f.billingScopeOf(ctx, workspacePublicID)
	if err != nil {
		return tenancy.BillingAccount{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.byScope == nil {
		f.byScope = make(map[string]int64)
	}
	if accountID, ok := f.byScope[scope]; ok {
		if existing, ok := f.accounts[accountID]; ok {
			// Stable funds identity, resolving workspace: the account
			// names the payer while handlers name the workspace from
			// tenancy, so only the resolving location is refreshed.
			existing.WorkspaceInternalID = workspaceInternalID
			existing.WorkspacePublicID = workspacePublicID
			return existing, nil
		}
	}
	// First touch in this scope anchors the survivor: the stable
	// identity other workspaces in the scope resolve afterwards.
	account := tenancy.BillingAccount{
		InternalID:          workspaceInternalID,
		PublicID:            "ba_test_" + workspacePublicID,
		OwnerType:           ownerType,
		Shared:              shared,
		WorkspaceInternalID: workspaceInternalID,
		WorkspacePublicID:   workspacePublicID,
		CreatedAt:           billingTestNow,
		UpdatedAt:           billingTestNow,
	}
	if f.accounts == nil {
		f.accounts = make(map[int64]tenancy.BillingAccount)
	}
	if f.byPublic == nil {
		f.byPublic = make(map[string]int64)
	}
	f.accounts[account.InternalID] = account
	f.byScope[scope] = account.InternalID
	f.byPublic[account.PublicID] = account.InternalID
	return account, nil
}

func (f *fakeBillingAccounts) GetBillingAccountByPublicID(ctx context.Context, publicID string) (tenancy.BillingAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byPublic != nil {
		if internalID, ok := f.byPublic[publicID]; ok {
			if account, ok := f.accounts[internalID]; ok {
				return account, nil
			}
		}
	}
	for _, account := range f.accounts {
		if account.PublicID == publicID {
			return account, nil
		}
	}
	return tenancy.BillingAccount{}, tenancy.ErrBillingNotFound
}

func (f *fakeBillingAccounts) GetBillingAccountByInternalID(ctx context.Context, billingAccountInternalID int64) (tenancy.BillingAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if account, ok := f.accounts[billingAccountInternalID]; ok {
		return account, nil
	}
	return tenancy.BillingAccount{}, tenancy.ErrBillingNotFound
}

func (f *fakeBillingAccounts) GetBillingAccountRole(ctx context.Context, billingAccountInternalID, userID int64) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deny != nil && f.deny[userID] {
		return "", false, nil
	}
	if f.roles != nil {
		if byUser, ok := f.roles[billingAccountInternalID]; ok {
			if role, ok := byUser[userID]; ok {
				return role, true, nil
			}
			return "", false, nil
		}
	}
	return tenancy.BillingRoleOwner, true, nil
}

func (f *fakeBillingAccounts) ListBillingAccountMembers(ctx context.Context, billingAccountInternalID, actorUserID int64) ([]tenancy.BillingAccountMember, error) {
	role, ok, err := f.GetBillingAccountRole(ctx, billingAccountInternalID, actorUserID)
	if err != nil {
		return nil, err
	}
	if !ok || !tenancy.IsBillingManager(role) {
		return nil, tenancy.ErrBillingForbidden
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tenancy.BillingAccountMember
	if byUser, ok := f.roles[billingAccountInternalID]; ok {
		for userID, r := range byUser {
			out = append(out, tenancy.BillingAccountMember{BillingAccountID: billingAccountInternalID, UserID: userID, Role: r})
		}
	} else {
		out = append(out, tenancy.BillingAccountMember{BillingAccountID: billingAccountInternalID, UserID: actorUserID, Role: tenancy.BillingRoleOwner})
	}
	return out, nil
}

func (f *fakeBillingAccounts) AddBillingAccountMember(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64, role string) (*tenancy.BillingAccountMember, error) {
	actorRole, ok, err := f.GetBillingAccountRole(ctx, billingAccountInternalID, actorUserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, tenancy.ErrBillingForbidden
	}
	if err := tenancy.CheckBillingMemberAdd(actorRole, role); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.roles == nil {
		f.roles = make(map[int64]map[int64]string)
	}
	if f.roles[billingAccountInternalID] == nil {
		f.roles[billingAccountInternalID] = make(map[int64]string)
		// The permissive default owner materializes on first write so
		// later role lookups for the granting owner keep succeeding.
		f.roles[billingAccountInternalID][actorUserID] = actorRole
	}
	f.roles[billingAccountInternalID][targetUserID] = role
	return &tenancy.BillingAccountMember{BillingAccountID: billingAccountInternalID, UserID: targetUserID, Role: role}, nil
}

func (f *fakeBillingAccounts) UpdateBillingAccountMemberRole(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64, newRole string) (*tenancy.BillingAccountMember, error) {
	actorRole, _, err := f.GetBillingAccountRole(ctx, billingAccountInternalID, actorUserID)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	byUser := f.roles[billingAccountInternalID]
	current := ""
	if byUser != nil {
		current = byUser[targetUserID]
	}
	owners := 0
	for _, r := range byUser {
		if r == tenancy.BillingRoleOwner {
			owners++
		}
	}
	if owners == 0 {
		owners = 1
		if current == "" {
			current = tenancy.BillingRoleAdmin
		}
	}
	if err := tenancy.CheckBillingMemberMutation(actorRole, actorUserID, targetUserID, current, newRole, owners, false); err != nil {
		return nil, err
	}
	if f.roles == nil {
		f.roles = make(map[int64]map[int64]string)
	}
	if f.roles[billingAccountInternalID] == nil {
		f.roles[billingAccountInternalID] = make(map[int64]string)
	}
	f.roles[billingAccountInternalID][targetUserID] = newRole
	return &tenancy.BillingAccountMember{BillingAccountID: billingAccountInternalID, UserID: targetUserID, Role: newRole}, nil
}

func (f *fakeBillingAccounts) billingRoleLocked(billingAccountInternalID, userID int64) (string, bool) {
	if f.deny != nil && f.deny[userID] {
		return "", false
	}
	if byUser, ok := f.roles[billingAccountInternalID]; ok {
		if role, ok := byUser[userID]; ok {
			return role, true
		}
		return "", false
	}
	return tenancy.BillingRoleOwner, true
}

func (f *fakeBillingAccounts) InitiateBillingTransferInvite(ctx context.Context, billingAccountInternalID, fromUserID, toUserID int64, idempotencyKey string) (*tenancy.BillingTransferInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if fromUserID == toUserID || fromUserID <= 0 || toUserID <= 0 || billingAccountInternalID <= 0 {
		return nil, tenancy.ErrBillingWorkspaceInvalid
	}
	role, ok := f.billingRoleLocked(billingAccountInternalID, fromUserID)
	if !ok || !tenancy.IsBillingOwner(role) {
		return nil, tenancy.ErrBillingForbidden
	}
	if f.invites == nil {
		f.invites = make(map[string]*tenancy.BillingTransferInvite)
	}
	for _, existing := range f.invites {
		if existing.BillingAccountID != billingAccountInternalID {
			continue
		}
		if existing.Status == tenancy.BillingTransferPending {
			return nil, tenancy.ErrBillingConflict
		}
	}
	f.inviteSeq++
	publicID := "bti_test_" + string(rune('a'+f.inviteSeq%26)) + string(rune('0'+f.inviteSeq%10)) + "00000001"
	invited := &tenancy.BillingTransferInvite{
		InternalID: f.inviteSeq, PublicID: publicID,
		BillingAccountID: billingAccountInternalID,
		FromUserID:       fromUserID, ToUserID: toUserID,
		Status: tenancy.BillingTransferPending,
	}
	f.invites[publicID] = invited
	return invited, nil
}

func (f *fakeBillingAccounts) GetBillingTransferInvite(ctx context.Context, publicID string) (*tenancy.BillingTransferInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if invited, ok := f.invites[publicID]; ok {
		return invited, nil
	}
	return nil, tenancy.ErrBillingNotFound
}

func (f *fakeBillingAccounts) ListBillingTransferInvitesForAccount(ctx context.Context, billingAccountInternalID, actorUserID, beforeID int64, limit int) ([]*tenancy.BillingTransferInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	role, ok := f.billingRoleLocked(billingAccountInternalID, actorUserID)
	manager := ok && tenancy.IsBillingManager(role)
	var out []*tenancy.BillingTransferInvite
	for _, invited := range f.invites {
		if invited.BillingAccountID == billingAccountInternalID && (manager || invited.ToUserID == actorUserID) {
			out = append(out, invited)
		}
	}
	return out, nil
}

func (f *fakeBillingAccounts) AcceptBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, recipientUserID int64) (*tenancy.BillingTransferInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	invited, ok := f.invites[invitePublicID]
	if !ok || invited.BillingAccountID != billingAccountInternalID {
		return nil, tenancy.ErrBillingNotFound
	}
	if invited.Status != tenancy.BillingTransferPending {
		return nil, tenancy.ErrBillingConflict
	}
	if invited.ToUserID != recipientUserID {
		return nil, tenancy.ErrBillingForbidden
	}
	fromRole, ok := f.billingRoleLocked(invited.BillingAccountID, invited.FromUserID)
	if !ok || !tenancy.IsBillingOwner(fromRole) {
		return nil, tenancy.ErrBillingConflict
	}
	if f.roles == nil {
		f.roles = make(map[int64]map[int64]string)
	}
	if f.roles[invited.BillingAccountID] == nil {
		f.roles[invited.BillingAccountID] = make(map[int64]string)
	}
	f.roles[invited.BillingAccountID][invited.ToUserID] = tenancy.BillingRoleOwner
	f.roles[invited.BillingAccountID][invited.FromUserID] = tenancy.BillingRoleAdmin
	invited.Status = tenancy.BillingTransferAccepted
	return invited, nil
}

func (f *fakeBillingAccounts) RejectBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, recipientUserID int64) (*tenancy.BillingTransferInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	invited, ok := f.invites[invitePublicID]
	if !ok || invited.BillingAccountID != billingAccountInternalID {
		return nil, tenancy.ErrBillingNotFound
	}
	if invited.Status != tenancy.BillingTransferPending {
		return nil, tenancy.ErrBillingConflict
	}
	if invited.ToUserID != recipientUserID {
		return nil, tenancy.ErrBillingForbidden
	}
	invited.Status = tenancy.BillingTransferRejected
	return invited, nil
}

func (f *fakeBillingAccounts) CancelBillingTransferInvite(ctx context.Context, billingAccountInternalID int64, invitePublicID string, initiatorUserID int64) (*tenancy.BillingTransferInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	invited, ok := f.invites[invitePublicID]
	if !ok || invited.BillingAccountID != billingAccountInternalID {
		return nil, tenancy.ErrBillingNotFound
	}
	if invited.Status != tenancy.BillingTransferPending {
		return nil, tenancy.ErrBillingConflict
	}
	if invited.FromUserID != initiatorUserID {
		return nil, tenancy.ErrBillingForbidden
	}
	invited.Status = tenancy.BillingTransferCancelled
	return invited, nil
}

func (f *fakeBillingAccounts) RemoveBillingAccountMember(ctx context.Context, billingAccountInternalID, actorUserID, targetUserID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actorRole, _ := f.billingRoleLocked(billingAccountInternalID, actorUserID)
	targetRole, ok := f.billingRoleLocked(billingAccountInternalID, targetUserID)
	if !ok {
		return tenancy.ErrBillingNotFound
	}
	owners := 0
	for _, r := range f.roles[billingAccountInternalID] {
		if r == tenancy.BillingRoleOwner {
			owners++
		}
	}
	if owners == 0 {
		owners = 1
	}
	if err := tenancy.CheckBillingMemberMutation(actorRole, actorUserID, targetUserID, targetRole, "", owners, true); err != nil {
		return err
	}
	delete(f.roles[billingAccountInternalID], targetUserID)
	return nil
}

type fakeBillingWallets struct {
	mu      sync.Mutex
	wallets map[int64]domain.WalletDetails
	ledgers map[int64][]domain.LedgerTransaction
	getErr  map[int64]error
}

func (f *fakeBillingWallets) GetWallet(ctx context.Context, billingAccountID int64) (domain.WalletDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.getErr[billingAccountID]; ok {
		return domain.WalletDetails{}, err
	}
	wallet, ok := f.wallets[billingAccountID]
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
	if err, ok := f.getErr[input.BillingAccountID]; ok {
		return domain.LedgerPage{}, err
	}
	var filtered []domain.LedgerTransaction
	for _, tx := range f.ledgers[input.BillingAccountID] {
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
	mu        sync.Mutex
	records   map[int64][]billingrecordsports.Record
	lastInput billingrecordsports.ListInput
	err       error
}

func (f *fakeBillingRecords) List(ctx context.Context, input billingrecordsports.ListInput) (billingrecordsports.Page, error) {
	if f.err != nil {
		return billingrecordsports.Page{}, f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastInput = input
	all := f.records[input.BillingAccountID]
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
	snapshot      usageapplication.Usage
	lastFilter    domain.UsageFilter
	lastAccountID int64
	err           error
}

func (f *fakeBillingUsage) GetCurrent(ctx context.Context, userID int64, filter domain.UsageFilter, usageRange usagedomain.Range) (usageapplication.Usage, error) {
	f.lastFilter = filter
	f.lastAccountID = userID
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
	lastBill int64
	create   func(ctx context.Context, billingAccountID, userID int64, workspacePublicID, clientKey string, input workspaceCheckoutInput) (workspaceCheckoutResult, error)
}

func (f *fakeWorkspaceCheckout) CreateWorkspaceCheckout(ctx context.Context, billingAccountID, userID int64, workspacePublicID, clientKey string, input workspaceCheckoutInput) (workspaceCheckoutResult, error) {
	if f.create != nil {
		return f.create(ctx, billingAccountID, userID, workspacePublicID, clientKey, input)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastBill = billingAccountID
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

func (f *fakeWorkspaceCheckout) GetWorkspaceCheckout(ctx context.Context, billingAccountID int64, sessionID string) (workspaceCheckoutStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastBill = billingAccountID
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
	store := tenancy.NewInMemoryStoreForTests()
	fx := &billingTestFixture{
		store:    store,
		dir:      newStaticUserDirectory(),
		accounts: &fakeBillingAccounts{store: store},
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
		Snapshot:        func(ctx context.Context, read func(context.Context) error) error { return read(ctx) },
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

func (fx *billingTestFixture) seedWallet(billingAccountID int64, available, reserved int64) {
	fx.wallets.wallets[billingAccountID] = domain.WalletDetails{
		WalletBalance: domain.WalletBalance{BillingAccountID: billingAccountID, Available: available, Reserved: reserved},
		Status:        domain.WalletStatusActive,
	}
}

// mustAccount resolves the stable funds identity for a fixture workspace:
// reads seed the account wallet, never a user wallet (#916).
func (fx *billingTestFixture) mustAccount(t *testing.T, ctx context.Context, ws *tenancy.Workspace) int64 {
	t.Helper()
	account, err := fx.accounts.EnsureBillingAccountForWorkspace(ctx, ws.InternalID, ws.PublicID)
	if err != nil {
		t.Fatalf("resolve fixture billing account: %v", err)
	}
	return account.InternalID
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
	if data["workspace_id"] != ws.PublicID || data["owner_type"] != "workspace" || data["can_manage_member_quotas"] != true {
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

func TestV1BillingQuotaCapabilityUsesWorkspaceScope(t *testing.T) {
	ctx := t.Context()
	fx := newBillingTestFixture()
	org, err := fx.store.CreateOrganization(ctx, 11, "capability-org")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []struct {
		userID int64
		role   string
	}{{12, tenancy.RoleAdmin}, {13, tenancy.RoleMember}} {
		if _, err := fx.store.AddOrganizationMember(ctx, org.PublicID, m.userID, m.role); err != nil {
			t.Fatal(err)
		}
	}
	orgWorkspace, err := fx.store.CreateOrganizationWorkspace(ctx, org.PublicID, 11, "organization")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.AddWorkspaceMember(ctx, orgWorkspace.PublicID, 13, tenancy.RoleMember); err != nil {
		t.Fatal(err)
	}
	personalWorkspace := v1WorkspaceFixture(t, ctx, fx.store, "personal", 14)
	fx.store.SeedWorkspaceMember(personalWorkspace.PublicID, 12, tenancy.RoleAdmin)
	personalAccount := fx.mustAccount(t, ctx, personalWorkspace)
	fx.accounts.roles = map[int64]map[int64]string{personalAccount: {14: tenancy.BillingRoleOwner, 12: tenancy.BillingRoleAdmin}}
	for _, tc := range []struct {
		name      string
		workspace *tenancy.Workspace
		actor     int64
		want      bool
	}{
		{"organization owner", orgWorkspace, 11, true},
		{"organization admin", orgWorkspace, 12, true},
		{"organization member", orgWorkspace, 13, false},
		{"personal holder", personalWorkspace, 14, true},
		{"unrelated organization admin and billing admin", personalWorkspace, 12, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + tc.workspace.PublicID + "/billing-account", userID: tc.actor})
			if status != http.StatusOK || v1Data(t, payload).(map[string]any)["can_manage_member_quotas"] != tc.want {
				t.Fatalf("quota capability = %d %v, want %v", status, payload, tc.want)
			}
		})
	}

	fx.deps.Tenancy = quotaRoleLookupFailure{Store: fx.store, failedUserID: 12}
	status, _, payload := doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + orgWorkspace.PublicID + "/billing-account", userID: 12})
	if status != http.StatusServiceUnavailable || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("unavailable capability = %d %v, want 503", status, payload)
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
	fx.wallets.getErr[fx.mustAccount(t, ctx, ws)] = domain.ErrWalletRestricted
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
	accountID := fx.mustAccount(t, ctx, ws)
	fx.seedWallet(accountID, 980, 20)
	for i := int64(1); i <= 3; i++ {
		fx.wallets.ledgers[accountID] = append(fx.wallets.ledgers[accountID], domain.LedgerTransaction{
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

// TestV1BillingSharedBalanceSameAccount pins issue #922 rule 1 at the
// read edge under ADR-0038 scope sharing: two Personal Workspaces of one
// holder resolve one survivor Billing Account and report the same payer
// account and the same balance, while another holder's scope stays
// isolated. The balance names its workspace (consumption location) plus
// the stable payer account; it never derives from the Workspace Owner.
func TestV1BillingSharedBalanceSameAccount(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	wsA := v1WorkspaceFixture(t, ctx, fx.store, "billing-shared-a", 11)
	wsB := v1WorkspaceFixture(t, ctx, fx.store, "billing-shared-b", 11)
	// Same holder scope shares without any link step: the second Ensure
	// converges to the survivor anchored by the first.
	accountA := fx.mustAccount(t, ctx, wsA)
	if accountB := fx.mustAccount(t, ctx, wsB); accountB != accountA {
		t.Fatalf("same-scope accounts = %d vs %d, want one stable payer", accountB, accountA)
	}
	fx.seedWallet(accountA, 980, 20)
	engine := fx.engine()

	readBalance := func(wsID string) map[string]any {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + wsID + "/credits/balance", userID: 11})
		if status != http.StatusOK {
			t.Fatalf("balance %s status = %d (%v)", wsID, status, payload)
		}
		return v1Data(t, payload).(map[string]any)
	}
	first := readBalance(wsA.PublicID)
	second := readBalance(wsB.PublicID)
	if first["billing_account_id"] == "" || first["billing_account_id"] != second["billing_account_id"] {
		t.Fatalf("shared balance accounts = %v vs %v, want one stable payer", first["billing_account_id"], second["billing_account_id"])
	}
	if first["available_credits"] != float64(980) || second["available_credits"] != float64(980) {
		t.Fatalf("shared balances = %v vs %v, want 980/980", first["available_credits"], second["available_credits"])
	}
	if first["workspace_id"] != wsA.PublicID || second["workspace_id"] != wsB.PublicID {
		t.Fatalf("balance workspace scope = %v vs %v, want per-workspace consumption location", first["workspace_id"], second["workspace_id"])
	}

	// An independent holder scope keeps its own isolated account and balance.
	wsC := v1WorkspaceFixture(t, ctx, fx.store, "billing-shared-c", 12)
	fx.store.SeedWorkspaceMember(wsC.PublicID, 11, tenancy.RoleAdmin)
	accountC := fx.mustAccount(t, ctx, wsC)
	fx.seedWallet(accountC, 10, 0)
	third := readBalance(wsC.PublicID)
	if third["billing_account_id"] == first["billing_account_id"] {
		t.Fatalf("independent balance account = %v, want isolation from %v", third["billing_account_id"], first["billing_account_id"])
	}
	if third["available_credits"] != float64(10) {
		t.Fatalf("independent balance = %v, want 10", third["available_credits"])
	}
}

func TestV1BillingCheckoutIdempotency(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	accountID := fx.mustAccount(t, ctx, ws)
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
	if v1Data(t, first).(map[string]any)["billing_account_id"] != "ba_test_"+ws.PublicID {
		t.Fatalf("checkout must name its explicit target account: %v", first)
	}
	if fx.checkout.lastBill != accountID {
		t.Fatalf("checkout payer = %d, want stable account %d", fx.checkout.lastBill, accountID)
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
	if v1Data(t, got).(map[string]any)["billing_account_id"] != "ba_test_"+ws.PublicID {
		t.Fatalf("session must name its payer account: %v", got)
	}
}

// TestV1BillingFinanceRequiresBillingGrant pins the #919 financial
// boundary on the checkout surface: a workspace admin without a Billing
// Owner/Admin grant observes no session facts and cannot top up. The
// purchase-options/pending/recharges matrix lives in billing_gaps_test.go,
// where those seams are wired.
func TestV1BillingFinanceRequiresBillingGrant(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	if _, err := fx.store.AddWorkspaceMember(ctx, ws.PublicID, 12, tenancy.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	fx.accounts.deny = map[int64]bool{12: true}
	engine := fx.engine()
	base := "/api/v1/workspaces/" + ws.PublicID

	status, _, created := doV1(t, engine, v1Call{method: http.MethodPost, path: base + "/checkout-sessions", body: billingCheckoutBody(1000), userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "grant-key-1"}})
	if status != http.StatusCreated {
		t.Fatalf("manager checkout = %d (%v)", status, created)
	}
	sessionID := v1Data(t, created).(map[string]any)["session_id"].(string)

	status, _, forbiddenGet := doV1(t, engine, v1Call{method: http.MethodGet, path: base + "/checkout-sessions/" + sessionID, userID: 12})
	if status != http.StatusForbidden || v1ErrorCode(t, forbiddenGet) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-only admin session read = %d %v, want 403 billing_account_forbidden", status, forbiddenGet)
	}
	status, _, forbiddenCreate := doV1(t, engine, v1Call{method: http.MethodPost, path: base + "/checkout-sessions", body: billingCheckoutBody(1000), userID: 12, headers: map[string]string{contract.IdempotencyKeyHeader: "grant-key-4"}})
	if status != http.StatusForbidden || v1ErrorCode(t, forbiddenCreate) != string(contract.CodeBillingAccountForbidden) {
		t.Fatalf("space-only admin checkout = %d %v, want 403 billing_account_forbidden", status, forbiddenCreate)
	}
}

func TestV1BillingRecordsAndUsage(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "billing", 11)
	accountID := fx.mustAccount(t, ctx, ws)
	fx.seedWallet(accountID, 980, 0)
	fx.records.records[accountID] = []billingrecordsports.Record{{
		RecordID: "record-1", OccurredAt: billingTestNow, Kind: billingrecordsports.KindTopUp,
		Title: "Idea 充值", CreditDelta: 1000, AvailableCreditsAfter: func() *int64 { n := int64(1980); return &n }(),
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
		fx.usage.err = usageports.ErrInvalidBillingAccountID
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

// Full-account financial reads require a separate Billing grant. Workspace
// membership, including Workspace Owner/Admin, never grants that scope.
func TestV1BillingAccountReadsRequireFinancialGrant(t *testing.T) {
	ctx := context.Background()
	fx := newBillingTestFixture()
	ws := v1WorkspaceFixture(t, ctx, fx.store, "financial-reads", 11)
	accountID := fx.mustAccount(t, ctx, ws)
	fx.seedWallet(accountID, 980, 0)
	fx.accounts.deny = map[int64]bool{11: true, 12: true, 13: true}
	for _, member := range []struct {
		userID int64
		role   string
	}{{11, tenancy.RoleOwner}, {12, tenancy.RoleAdmin}, {13, tenancy.RoleMember}} {
		if member.userID != 11 {
			if _, err := fx.store.AddWorkspaceMember(ctx, ws.PublicID, member.userID, member.role); err != nil {
				t.Fatal(err)
			}
		}
		for _, suffix := range []string{"/credits/ledger"} {
			status, _, payload := doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + suffix, userID: member.userID})
			if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeBillingAccountForbidden) {
				t.Fatalf("%s %s = %d %v", member.role, suffix, status, payload)
			}
		}
		status, _, payload := doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/usage", userID: member.userID})
		if status != http.StatusOK {
			t.Fatalf("%s scoped usage = %d %v", member.role, status, payload)
		}
		expected := domain.UsageFilter{WorkspaceID: ws.InternalID}
		if member.role == tenancy.RoleMember {
			expected = domain.UsageFilter{ActorUserID: member.userID}
		}
		status, _, payload = doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-records", userID: member.userID})
		if status != http.StatusOK || (fx.records.lastInput.WorkspaceID != expected.WorkspaceID || fx.records.lastInput.ActorUserID != expected.ActorUserID) || fx.records.lastInput.BillingAccountID != accountID {
			t.Fatalf("%s scoped records = %d %v input=%+v, want %+v", member.role, status, payload, fx.records.lastInput, expected)
		}
		status, _, payload = doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-account", userID: member.userID})
		expectedScope := "workspace"
		if member.role == tenancy.RoleMember {
			expectedScope = "member"
		}
		if status != http.StatusOK || v1Data(t, payload).(map[string]any)["consumption_scope"] != expectedScope {
			t.Fatalf("%s billing-account scope=%d %v", member.role, status, payload)
		}
		if fx.usage.lastFilter != expected || fx.usage.lastAccountID != accountID {
			t.Fatalf("%s usage filter/account = %+v/%d, want %+v/%d", member.role, fx.usage.lastFilter, fx.usage.lastAccountID, expected, accountID)
		}
	}

	// A Billing Admin with only ordinary Workspace membership can read the
	// account, proving this is a financial-role gate rather than a space rank.
	delete(fx.accounts.deny, 13)
	fx.accounts.roles = map[int64]map[int64]string{accountID: {13: tenancy.BillingRoleAdmin}}
	for _, suffix := range []string{"/credits/ledger", "/billing-records", "/usage"} {
		status, _, payload := doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + suffix, userID: 13})
		if status != http.StatusOK {
			t.Fatalf("Billing Admin %s = %d %v", suffix, status, payload)
		}
	}
	status, _, payload := doV1(t, fx.engine(), v1Call{method: http.MethodGet, path: "/api/v1/workspaces/" + ws.PublicID + "/billing-account", userID: 13})
	if status != http.StatusOK || v1Data(t, payload).(map[string]any)["can_manage_member_quotas"] != false {
		t.Fatalf("Billing Admin quota capability = %d %v, want denied", status, payload)
	}
	if fx.records.lastInput.WorkspaceID != 0 || fx.records.lastInput.ActorUserID != 0 {
		t.Fatalf("financial admin records scoped unexpectedly: %+v", fx.records.lastInput)
	}
	if fx.usage.lastFilter != (domain.UsageFilter{}) {
		t.Fatalf("financial admin scoped unexpectedly: %+v", fx.usage.lastFilter)
	}

}

func TestV1BillingCheckoutKeysDoNotConflictAcrossAccounts(t *testing.T) {
	fx := newBillingTestFixture()
	first := v1WorkspaceFixture(t, t.Context(), fx.store, "first-account", 11)
	// Distinct holder scopes own distinct payer accounts (ADR-0038); the
	// same manager key stays independent across them. User 11 reads the
	// second scope as an explicit workspace member.
	second := v1WorkspaceFixture(t, t.Context(), fx.store, "second-account", 12)
	fx.store.SeedWorkspaceMember(second.PublicID, 11, tenancy.RoleAdmin)
	engine := fx.engine()
	var previousSession string
	for _, workspace := range []*tenancy.Workspace{first, second} {
		status, _, payload := doV1(t, engine, v1Call{method: http.MethodPost, path: "/api/v1/workspaces/" + workspace.PublicID + "/checkout-sessions", body: billingCheckoutBody(1000), userID: 11, headers: map[string]string{contract.IdempotencyKeyHeader: "same-account-manager-key"}})
		if status != http.StatusCreated {
			t.Fatalf("independent checkout = %d %v", status, payload)
		}
		session := v1Data(t, payload).(map[string]any)["session_id"].(string)
		if session == previousSession {
			t.Fatal("another account's session was replayed")
		}
		previousSession = session
	}
}
