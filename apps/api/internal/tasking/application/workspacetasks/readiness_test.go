package workspacetasks

import (
	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/memberquotas"
	taskadapters "ideaboard/api/internal/tasking/infrastructure/adapters"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"

	"context"
	"errors"
	creditsdomain "ideaboard/api/internal/credits/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/tenancy/funds"
	"testing"
	"time"
)

// Test adapters cross the production Check interface without HTTP, settlement,
// Hold, Task persistence or queue dependencies.
type readinessFacts struct {
	scope                                                taskports.ReadinessWorkspace
	scopeErr                                             error
	board                                                *BoardAccess
	balances                                             map[int64]int64
	restrictions                                         bool
	fundsErr, walletErr, quotaErr                        error
	fundsCalls, quotaCalls                               int
	walletAccount, quotaAccount, quotaActor, quotaAmount int64
	quotaAt                                              time.Time
	ownerType                                            string
}

func (f *readinessFacts) ResolveReadinessWorkspace(context.Context, string, int64) (taskports.ReadinessWorkspace, error) {
	return f.scope, f.scopeErr
}
func (f *readinessFacts) ResolveBoardAccess(context.Context, string, string, int64, string) (*BoardAccess, error) {
	return f.board, nil
}
func (f *readinessFacts) ResolveFundsForActor(_ context.Context, workspace string, actor int64) (funds.Identity, error) {
	f.fundsCalls++
	return funds.Identity{WorkspacePublicID: workspace, BillingAccountID: 8000 + actor, OwnerType: f.ownerType}, f.fundsErr
}
func (f *readinessFacts) GetWallet(_ context.Context, account int64) (creditsdomain.WalletDetails, error) {
	f.walletAccount = account
	wallet := creditsdomain.WalletDetails{}
	wallet.Available = f.balances[account]
	if f.restrictions {
		wallet.ActiveRestrictions = []creditsdomain.WalletRestriction{{Type: "ledger_integrity"}}
	}
	return wallet, f.walletErr
}
func (f *readinessFacts) CheckQuotaForQuote(_ context.Context, account, actor, amount int64, at time.Time) error {
	f.quotaCalls++
	f.quotaAccount = account
	f.quotaActor = actor
	f.quotaAmount = amount
	f.quotaAt = at
	return f.quotaErr
}
func newReadinessFacts() *readinessFacts {
	return &readinessFacts{scope: taskports.ReadinessWorkspace{Role: "member"}, balances: map[int64]int64{8003: 100, 8004: 100}, ownerType: funds.OwnerTypeWorkspace}
}
func readinessForFacts(f *readinessFacts) *Readiness {
	return NewReadiness(ReadinessDeps{Workspaces: f, Boards: f, BillingAccounts: f, Wallets: f, Quotas: f,
		Quoter: &fakeQuoter{snapshot: BillingSnapshot{UnitCredits: 10, QuotedAt: time.Date(2026, 11, 6, 23, 59, 59, 0, time.UTC)}}, Resolver: allowTestPoolResolver()})
}
func readinessInput() taskports.ReadinessParams {
	return taskports.ReadinessParams{WorkspaceID: "ws_readiness01", ActorUserID: 3, Service: "render", Quantity: 1}
}

func TestReadinessActorWalletAndScope(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		actorBalance, ownerBalance int64
		organization               bool
		fundsLabel                 string
		quotaErr, want             error
		quotaCalls                 int
	}{
		{name: "owner broke actor funded", actorBalance: 100, ownerBalance: 0, fundsLabel: funds.OwnerTypeWorkspace},
		{name: "owner funded actor broke", actorBalance: 5, ownerBalance: 10000, fundsLabel: funds.OwnerTypeWorkspace, want: ErrInsufficientCredits},
		{name: "Personal ignores misleading funds label", actorBalance: 100, fundsLabel: funds.OwnerTypeOrganization, quotaErr: ErrMemberQuotaExhausted},
		{name: "Organization enforces despite misleading funds label", actorBalance: 100, organization: true, fundsLabel: funds.OwnerTypeWorkspace, quotaErr: ErrMemberQuotaExhausted, want: ErrMemberQuotaExhausted, quotaCalls: 1},
		{name: "Organization unlimited", actorBalance: 100, organization: true, fundsLabel: funds.OwnerTypeOrganization, quotaCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReadinessFacts()
			f.balances[8001] = tc.ownerBalance
			f.balances[8003] = tc.actorBalance
			f.scope.Organization = tc.organization
			f.ownerType = tc.fundsLabel
			f.quotaErr = tc.quotaErr
			var checker taskports.ReadinessChecker = readinessForFacts(f)
			snapshot, err := checker.Check(context.Background(), readinessInput())
			if !errors.Is(err, tc.want) {
				t.Fatalf("Check error=%v, want %v", err, tc.want)
			}
			if f.walletAccount != 8003 || f.quotaCalls != tc.quotaCalls {
				t.Fatalf("payer=%d quota calls=%d", f.walletAccount, f.quotaCalls)
			}
			if err == nil && (snapshot.TotalCredits != 10 || snapshot.SettlementState != "quoted" || snapshot.BillingAccountID != 0) {
				t.Fatalf("price-only snapshot=%+v", snapshot)
			}
			if f.quotaCalls > 0 && (f.quotaAccount != 8003 || f.quotaActor != 3 || f.quotaAmount != 10 || !f.quotaAt.Equal(time.Date(2026, 11, 6, 23, 59, 59, 0, time.UTC))) {
				t.Fatalf("quota attribution=%+v", f)
			}
		})
	}
}
func TestReadinessBoardEligibilityPrecedesFunds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		board *BoardAccess
		want  error
	}{
		{name: "invisible", want: ErrBoardNotFound},
		{name: "viewer", board: &BoardAccess{CanRead: true}, want: ErrForbidden},
		{name: "archived", board: &BoardAccess{CanRead: true, CanGenerate: true, Archived: true}, want: ErrBoardArchived},
		{name: "editor", board: &BoardAccess{CanRead: true, CanWrite: true, CanGenerate: true}},
		{name: "creator", board: &BoardAccess{IsOwner: true, CanRead: true, CanWrite: true, CanGenerate: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReadinessFacts()
			f.board = tc.board
			input := readinessInput()
			input.BoardID = "board_readiness01"
			_, err := readinessForFacts(f).Check(context.Background(), input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Check=%v want %v", err, tc.want)
			}
			if tc.want != nil && f.fundsCalls != 0 {
				t.Fatal("denied Board reached payer resolution")
			}
		})
	}
	f := newReadinessFacts()
	f.scopeErr = ErrNotFound
	input := readinessInput()
	input.BoardID = "malformed"
	if _, err := readinessForFacts(f).Check(context.Background(), input); !IsValidationError(err) {
		t.Fatalf("Board validation before scope reread=%v", err)
	}
	input.BoardID = ""
	if _, err := readinessForFacts(f).Check(context.Background(), input); !errors.Is(err, ErrNotFound) || f.fundsCalls != 0 {
		t.Fatalf("hidden scope=%v funds=%d", err, f.fundsCalls)
	}
}
func TestReadinessFundsFailuresAndFreeQuote(t *testing.T) {
	t.Run("restrictions precede balance and quota", func(t *testing.T) {
		f := newReadinessFacts()
		f.scope.Organization = true
		f.balances[8003] = 0
		f.restrictions = true
		f.quotaErr = ErrMemberQuotaExhausted
		if _, err := readinessForFacts(f).Check(context.Background(), readinessInput()); !errors.Is(err, ErrWalletRestricted) || f.quotaCalls != 0 {
			t.Fatalf("restricted=%v quota=%d", err, f.quotaCalls)
		}
	})
	for _, origin := range []string{"funds", "wallet"} {
		t.Run(origin+" preserves origin and cause", func(t *testing.T) {
			cause := errors.New("same reader outage")
			f := newReadinessFacts()
			if origin == "funds" {
				f.fundsErr = cause
			} else {
				f.walletErr = cause
			}
			_, err := readinessForFacts(f).Check(context.Background(), readinessInput())
			var fundsError *taskports.FundsResolutionError
			if !errors.Is(err, cause) || errors.As(err, &fundsError) != (origin == "funds") {
				t.Fatalf("error identity/origin=%v", err)
			}
		})
	}
	t.Run("Wallet restricted sentinel", func(t *testing.T) {
		f := newReadinessFacts()
		f.walletErr = creditsdomain.ErrWalletRestricted
		if _, err := readinessForFacts(f).Check(context.Background(), readinessInput()); !errors.Is(err, ErrWalletRestricted) {
			t.Fatalf("restricted reader=%v", err)
		}
	})
	t.Run("routing unavailable", func(t *testing.T) {
		f := newReadinessFacts()
		r := readinessForFacts(f)
		r.deps.Resolver = nil
		if _, err := r.Check(context.Background(), readinessInput()); err == nil || f.walletAccount != 0 {
			t.Fatalf("routing=%v wallet=%d", err, f.walletAccount)
		}
	})
	t.Run("free skips quota", func(t *testing.T) {
		f := newReadinessFacts()
		f.scope.Organization = true
		f.quotaErr = ErrMemberQuotaExhausted
		r := readinessForFacts(f)
		r.deps.Quoter = &fakeQuoter{}
		if _, err := r.Check(context.Background(), readinessInput()); err != nil || f.quotaCalls != 0 {
			t.Fatalf("free=%v quota=%d", err, f.quotaCalls)
		}
	})
	t.Run("nil quota is unlimited", func(t *testing.T) {
		f := newReadinessFacts()
		f.scope.Organization = true
		r := readinessForFacts(f)
		r.deps.Quotas = nil
		if _, err := r.Check(context.Background(), readinessInput()); err != nil {
			t.Fatalf("nil quota=%v", err)
		}
	})
}

func productionReadiness(env *atomicEnv) (*Readiness, *boards.PostgresWorkspaceBoardStore) {
	boardStore := boards.NewPostgresWorkspaceBoardStore(env.pool, workspacemedia.ValidateDocumentAssets)
	return NewReadiness(ReadinessDeps{
		Workspaces:      &taskadapters.ReadinessWorkspaceReader{Store: tenancypg.NewStore(env.pool, nil)},
		Boards:          &boardtaskaccess.Authorizer{Store: boardStore},
		BillingAccounts: tenancypg.NewBillingAccountStore(env.pool), Wallets: env.credits, Quotas: memberquotas.NewStore(env.pool),
		Quoter: &taskadapters.WorkspaceTaskQuoter{Catalog: env.catalog}, Resolver: env.serviceDeps.Resolver,
	}), boardStore
}

// Snapshot actual persisted side effects, including the enqueue transaction
// probe used by the canonical settlement fixture (not a live River worker).
func readinessWriteCounts(t *testing.T, env *atomicEnv) [4]int {
	t.Helper()
	var counts [4]int
	if err := env.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM credit_transactions), (SELECT count(*) FROM credit_holds), (SELECT count(*) FROM tasks), (SELECT count(*) FROM settle_enqueue_probe)`).Scan(&counts[0], &counts[1], &counts[2], &counts[3]); err != nil {
		t.Fatal(err)
	}
	return counts
}

func TestReadinessProductionPersonalIsAdvisory(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	actor, account := seedPersonalCollaborator(t, env)
	checker, boardStore := productionReadiness(env)
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.CreateBoard(ctx, env.ws, env.userID, "readiness", boardID); err != nil {
		t.Fatal(err)
	}
	input := taskports.ReadinessParams{WorkspaceID: env.ws, ActorUserID: actor, BoardID: boardID, Service: "render", Resolution: "1k", Quantity: 1}
	before := readinessWriteCounts(t, env)
	if _, err := checker.Check(ctx, input); !errors.Is(err, ErrBoardNotFound) {
		t.Fatalf("private Board=%v", err)
	}
	access, err := boardStore.GetBoardAccess(ctx, env.ws, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.ReplaceBoardAccess(ctx, env.ws, boardID, env.userID, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{actor}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := checker.Check(ctx, input)
	if err != nil {
		t.Fatalf("production readiness=%v", err)
	}
	if snapshot.TotalCredits != 10 || snapshot.SettlementState != "quoted" {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if after := readinessWriteCounts(t, env); after != before {
		t.Fatalf("readiness wrote ledger/Hold/Task/queue: before=%v after=%v", before, after)
	}
	if available, reserved := collaboratorWallet(t, env, account); available != 1000 || reserved != 0 {
		t.Fatalf("advisory actor wallet=%d/%d", available, reserved)
	}
	created, err := env.service.Create(ctx, CreateInput{WorkspaceID: env.ws, OwnerUserID: actor, BoardID: boardID, Service: "render", Quantity: 1, Resolution: "1k"}, "readiness-personal-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatal(err)
	}
	if created.BillingAccountID != account || created.OwnerUserID != actor || created.BillingSnapshot.TotalCredits != snapshot.TotalCredits {
		t.Fatalf("submit parity=%+v", created)
	}
	if available, reserved := collaboratorWallet(t, env, account); available != 990 || reserved != 10 {
		t.Fatalf("authoritative actor wallet=%d/%d", available, reserved)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("owner wallet changed=%d/%d", available, reserved)
	}
}

func TestReadinessProductionOrganizationQuotaParity(t *testing.T) {
	env, quotas := openQuotaAtomicEnv(t)
	ctx := context.Background()
	checker, _ := productionReadiness(env)
	input := taskports.ReadinessParams{WorkspaceID: env.ws, ActorUserID: env.userID, Service: "render", Resolution: "1k", Quantity: 1}
	setDailyQuota(t, quotas, env.accountID, env.userID, 5)
	before := readinessWriteCounts(t, env)
	if _, err := checker.Check(ctx, input); !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("Org readiness cap=%v", err)
	}
	if _, err := env.service.Create(ctx, CreateInput{WorkspaceID: env.ws, OwnerUserID: env.userID, Service: "render", Quantity: 1, Resolution: "1k"}, "readiness-org-denied-"+randomAtomicHex(t, 4)); !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("Org submit cap=%v", err)
	}
	if after := readinessWriteCounts(t, env); after != before {
		t.Fatalf("denied preflight/submit wrote=%v -> %v", before, after)
	}
	setDailyQuota(t, quotas, env.accountID, env.userID, 15)
	snapshot, err := checker.Check(ctx, input)
	if err != nil {
		t.Fatalf("funded Org readiness=%v", err)
	}
	if after := readinessWriteCounts(t, env); after != before {
		t.Fatalf("allowed readiness wrote=%v -> %v", before, after)
	}
	created := env.createPaidTaskKeyed(t, 1, "readiness-org-ok-"+randomAtomicHex(t, 4))
	if created.BillingAccountID != env.accountID || created.BillingSnapshot.TotalCredits != snapshot.TotalCredits {
		t.Fatalf("Org submit parity=%+v", created)
	}
	if _, err := checker.Check(ctx, input); !errors.Is(err, ErrMemberQuotaExhausted) {
		t.Fatalf("pending Hold not reflected in Org preflight=%v", err)
	}
}
