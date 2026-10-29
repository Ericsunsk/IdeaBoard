package credits

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/credits/application/commands"
	"ideaboard/api/internal/credits/application/health"
	"ideaboard/api/internal/credits/application/readmodel"
	appreconciliation "ideaboard/api/internal/credits/application/reconciliation"
	"ideaboard/api/internal/credits/application/usage"
	appwallet "ideaboard/api/internal/credits/application/wallet"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/infrastructure/postgres"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	initErr        error
	pool           *pgxpool.Pool
	commandBackend commands.Backend
	walletBackend  appwallet.Backend
	commands       *commands.Service
	wallet         *appwallet.Service
	readmodel      *readmodel.Service
	healthSvc      *health.Service
	usageSvc       *usage.Service
	reconciliation *appreconciliation.Service
}

type postgresBackendsSet struct {
	commands       commands.Backend
	wallet         appwallet.Backend
	readmodel      readmodel.Backend
	health         health.Backend
	usage          usage.Backend
	reconciliation appreconciliation.Backend
}

func postgresBackends() postgresBackendsSet {
	return postgresBackendsSet{
		commands: commands.Backend{
			ExecuteCreditCommand:               postgres.ExecuteCreditCommand,
			ExecuteCreditPurchase:              postgres.ExecuteCreditPurchase,
			ExecuteCreditPurchaseRefundHold:    postgres.ExecuteCreditPurchaseRefundHold,
			ExecuteCreditPurchaseRefund:        postgres.ExecuteCreditPurchaseRefund,
			ExecuteCreditPaymentDisputeDebit:   postgres.ExecuteCreditPaymentDisputeDebit,
			ExecuteCreditPaymentDisputeRestore: postgres.ExecuteCreditPaymentDisputeRestore,
		},
		wallet: appwallet.Backend{
			MapDatabaseError: postgres.MapCreditDatabaseError, ResolvePersonalAccount: postgres.ResolvePersonalAccount,
			ProvisionAccountWallet: postgres.ProvisionAccountWallet, BindIdentityKey: postgres.BindIdentityKey,
			SetAccountWalletFrozen: postgres.SetAccountWalletFrozen, SetSoleOwnedWalletsFrozen: postgres.SetSoleOwnedWalletsFrozen,
			RemoveSharedBillingMembership: postgres.RemoveSharedBillingMembership,
			ClaimWelcome:                  postgres.ClaimWelcome, ExecuteGrant: postgres.ExecuteGrant,
			GetWallet: postgres.GetWalletInTx, RestrictWallet: postgres.RestrictWallet, ReleaseWallet: postgres.ReleaseWallet,
		},
		readmodel: readmodel.Backend{GetAvailableBalances: postgres.GetAvailableBalances, GetWallet: postgres.GetWallet, ListLedger: postgres.ListLedger},
		health:    health.Backend{GetSnapshot: postgres.GetCreditHealthSnapshot},
		usage:     usage.Backend{GetCaptureUsage: postgres.GetCaptureUsage, GetDailyCaptureUsage: postgres.GetDailyCaptureUsage},
		reconciliation: appreconciliation.Backend{
			CreateReconciliationRun: postgres.CreateReconciliationRun, ExecuteReconciliation: postgres.ExecuteReconciliation,
			FailReconciliation: postgres.FailReconciliation, GetReconciliationRun: postgres.GetReconciliationRun,
			ListReconciliationRuns: postgres.ListReconciliationRuns, ListGlobalReconciliationFindings: postgres.ListGlobalReconciliationFindings,
		},
	}
}

var defaultPostgresBackends = postgresBackends()

var errServiceUnavailable = fmt.Errorf("credits service: %w", database.ErrPoolNotConfigured)

func (s *Service) serviceError() error {
	if s == nil {
		return errServiceUnavailable
	}
	return s.initErr
}

func (s *Service) requireCommands() (*commands.Service, error) {
	if err := s.serviceError(); err != nil {
		return nil, err
	}
	if s.commands == nil {
		return nil, errServiceUnavailable
	}
	return s.commands, nil
}

func (s *Service) requireWallet() (*appwallet.Service, error) {
	if err := s.serviceError(); err != nil {
		return nil, err
	}
	if s.wallet == nil {
		return nil, errServiceUnavailable
	}
	return s.wallet, nil
}

func (s *Service) requireReadModel() (*readmodel.Service, error) {
	if err := s.serviceError(); err != nil {
		return nil, err
	}
	if s.readmodel == nil {
		return nil, errServiceUnavailable
	}
	return s.readmodel, nil
}

func (s *Service) requireUsage() (*usage.Service, error) {
	if err := s.serviceError(); err != nil {
		return nil, err
	}
	if s.usageSvc == nil {
		return nil, errServiceUnavailable
	}
	return s.usageSvc, nil
}

func (s *Service) requireHealth() (*health.Service, error) {
	if err := s.serviceError(); err != nil {
		return nil, err
	}
	if s.healthSvc == nil {
		return nil, errServiceUnavailable
	}
	return s.healthSvc, nil
}

func (s *Service) requireReconciliation() (*appreconciliation.Service, error) {
	if err := s.serviceError(); err != nil {
		return nil, err
	}
	if s.reconciliation == nil {
		return nil, errServiceUnavailable
	}
	return s.reconciliation, nil
}

func buildService(pool *pgxpool.Pool, dispatcher ports.ReconciliationRunDispatcher) (*Service, error) {
	if pool == nil {
		return nil, fmt.Errorf("configure Credits: pool is nil")
	}
	backend := defaultPostgresBackends
	commandsSvc, err := commands.NewService(pool, backend.commands)
	if err != nil {
		return nil, err
	}
	walletSvc, err := appwallet.NewService(pool, backend.wallet)
	if err != nil {
		return nil, err
	}
	readSvc, err := readmodel.NewService(pool, backend.readmodel)
	if err != nil {
		return nil, err
	}
	healthSvc, err := health.NewService(pool, backend.health)
	if err != nil {
		return nil, err
	}
	usageSvc, err := usage.NewService(pool, backend.usage)
	if err != nil {
		return nil, err
	}
	var reconciliationSvc *appreconciliation.Service
	if dispatcher == nil {
		reconciliationSvc, err = appreconciliation.NewServiceWithoutDispatcher(pool, backend.reconciliation)
	} else {
		reconciliationSvc, err = appreconciliation.NewService(pool, dispatcher, backend.reconciliation)
	}
	if err != nil {
		return nil, err
	}
	return &Service{
		pool:           pool,
		commandBackend: backend.commands,
		walletBackend:  backend.wallet,
		commands:       commandsSvc,
		wallet:         walletSvc,
		readmodel:      readSvc,
		healthSvc:      healthSvc,
		usageSvc:       usageSvc,
		reconciliation: reconciliationSvc,
	}, nil
}

func NewService(pool *pgxpool.Pool) *Service {
	if pool == nil {
		return &Service{}
	}
	service, err := buildService(pool, nil)
	if err != nil {
		return &Service{initErr: err}
	}
	return service
}

func NewServiceWithReconciliation(pool *pgxpool.Pool, dispatcher ports.ReconciliationRunDispatcher) (*Service, error) {
	if dispatcher == nil {
		return nil, fmt.Errorf("configure Credits reconciliation: dispatcher is nil")
	}
	return buildService(pool, dispatcher)
}

// commandsInTx is the shared caller-owned-transaction view over the
// default postgres backend set. The caller owns commit and rollback.
// Narrow role faces below expose it per settlement role; the wide
// ports.Commands value is only the carrier that each face narrows.
func (s *Service) commandsInTx(tx pgx.Tx) ports.Commands {
	if s == nil || s.commands == nil {
		return commands.NewTransactionCommands(tx, commands.Backend{})
	}
	return commands.NewTransactionCommands(tx, s.commandBackend)
}

// SettlementInTx lets workspace-scoped spenders (Tasking, Vision Search)
// compose Hold/Capture/Release into the caller-owned PostgreSQL
// transaction. The caller owns commit and rollback.
func (s *Service) SettlementInTx(tx pgx.Tx) ports.SettlementCommands {
	return s.commandsInTx(tx)
}

// AdministrativeInTx is the transaction-scoped Control fund-repair face
// (Grant/Deduct/Correct/Refund). The caller owns commit and rollback.
func (s *Service) AdministrativeInTx(tx pgx.Tx) ports.AdministrativeCommands {
	return s.commandsInTx(tx)
}

// GrantsInTx is the transaction-scoped Grant-only face (notably Rewards,
// which must not see Deduct/Correct/Refund). The caller owns commit and
// rollback.
func (s *Service) GrantsInTx(tx pgx.Tx) ports.Granter {
	return s.commandsInTx(tx)
}

// PaymentInTx is the transaction-scoped Payments face
// (Purchase/PurchaseRefundHold/PurchaseRefund/Release/dispute pair) for
// Service owners. Standalone repositories without an
// application-lifetime Service use the package-level CommandsInTx
// instead. The caller owns commit and rollback.
func (s *Service) PaymentInTx(tx pgx.Tx) ports.PaymentCommands {
	return s.commandsInTx(tx)
}

// CommandsInTx composes Payments-owned Commands into an existing pgx
// transaction without requiring an application-lifetime Service. It is
// the canonical seam for the standalone Payments repositories; Service
// owners use (s *Service).PaymentInTx instead. The caller owns commit
// and rollback.
func CommandsInTx(tx pgx.Tx) (ports.PaymentCommands, error) {
	if tx == nil {
		return nil, fmt.Errorf("credits: transaction is nil")
	}
	return commands.NewTransactionCommands(tx, defaultPostgresBackends.commands), nil
}

// ResolvePersonalAccountInTx ensures and returns the deterministic initial
// personal Billing Account for one user inside the caller's transaction
// (#916). Ledger-writing callers without a workspace scope (Rewards,
// Payments, Welcome, Admin) resolve the payer/operator through this seam
// so every Command carries a Billing Account with the real Actor.
// Workspace-scoped spenders (Tasks, Vision Search) resolve the consuming
// Workspace's stable account instead; this seam is never their fallback.
func ResolvePersonalAccountInTx(ctx context.Context, tx pgx.Tx) (PersonalAccountResolver, error) {
	if tx == nil {
		return nil, fmt.Errorf("credits: transaction is nil")
	}
	return personalAccountResolver{tx: tx}, nil
}

// PersonalAccountResolver resolves initial personal accounts on a borrowed
// transaction.
type PersonalAccountResolver interface {
	ResolvePersonalAccount(ctx context.Context, userID int64) (int64, error)
}

type personalAccountResolver struct {
	tx pgx.Tx
}

func (r personalAccountResolver) ResolvePersonalAccount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, fmt.Errorf("credits: invalid user for personal account resolution")
	}
	var accountID int64
	if err := r.tx.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		return 0, err
	}
	if accountID <= 0 {
		return 0, fmt.Errorf("credits: personal billing account not resolved")
	}
	return accountID, nil
}

// LifecycleInTx lets Identity compose Wallet changes with user writes.
// The caller owns commit and rollback.
func (s *Service) LifecycleInTx(tx pgx.Tx) ports.WalletLifecycle {
	if s == nil || s.wallet == nil {
		return appwallet.NewLifecycleTransaction(tx, appwallet.Backend{})
	}
	return appwallet.NewLifecycleTransaction(tx, s.walletBackend)
}

// RestrictionsInTx lets callers compose restriction changes with their pgx
// transaction and the same Credits-owned SECURITY DEFINER seam. It is the
// canonical restriction seam.
func RestrictionsInTx(tx pgx.Tx) ports.WalletRestrictionCommands {
	return appwallet.NewRestrictionTransaction(tx, defaultPostgresBackends.wallet)
}

var (
	_ ports.AdministrativeCommands = managedAdministrativeCommands{}
	_ ports.SettlementCommands     = managedSettlementCommands{}
	_ ports.PaymentCommands        = managedPaymentCommands{}
)

// managedAdministrativeCommands is the pool-owned Control fund-repair
// view. Each call resolves the command service and runs inside one
// managed (or ambient guard-carried) transaction through the single
// commands.Service.ManagedCommands runner. Misconfigured services fail
// at call time with the same service-unavailable error the previous
// per-Command pass-throughs returned.
type managedAdministrativeCommands struct {
	service *Service
}

func (m managedAdministrativeCommands) Grant(ctx context.Context, command ports.GrantCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Grant(ctx, command)
	})
}

func (m managedAdministrativeCommands) Deduct(ctx context.Context, command ports.DeductionCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Deduct(ctx, command)
	})
}

func (m managedAdministrativeCommands) Correct(ctx context.Context, command ports.CorrectionCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Correct(ctx, command)
	})
}

func (m managedAdministrativeCommands) Refund(ctx context.Context, command ports.RefundCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Refund(ctx, command)
	})
}

// managedSettlementCommands is the pool-owned workspace-spending view
// (Hold/Capture/Release outside a caller transaction). Terminal
// settlement prefers the transaction-scoped SettlementInTx face; this
// view serves non-transactional holders.
type managedSettlementCommands struct {
	service *Service
}

func (m managedSettlementCommands) Hold(ctx context.Context, command ports.HoldCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Hold(ctx, command)
	})
}

func (m managedSettlementCommands) Capture(ctx context.Context, command ports.CaptureCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Capture(ctx, command)
	})
}

func (m managedSettlementCommands) Release(ctx context.Context, command ports.ReleaseCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Release(ctx, command)
	})
}

// Administrative returns the pool-owned Control fund-repair face
// (Grant/Deduct/Correct/Refund). It never returns nil: a misconfigured
// service yields a view whose calls fail with the service-unavailable
// error, matching the previous pass-through behavior.
func (s *Service) Administrative() ports.AdministrativeCommands {
	return managedAdministrativeCommands{service: s}
}

// managedPaymentCommands is the pool-owned Payments view
// (Purchase/PurchaseRefundHold/PurchaseRefund/Release/dispute pair).
// Production Payments paths prefer the transaction-scoped PaymentInTx
// face; this view serves non-transactional validation and tests.
type managedPaymentCommands struct {
	service *Service
}

func (m managedPaymentCommands) Purchase(ctx context.Context, command ports.PurchaseCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Purchase(ctx, command)
	})
}

func (m managedPaymentCommands) PurchaseRefundHold(ctx context.Context, command ports.PurchaseRefundHoldCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.PurchaseRefundHold(ctx, command)
	})
}

func (m managedPaymentCommands) PurchaseRefund(ctx context.Context, command ports.PurchaseRefundCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.PurchaseRefund(ctx, command)
	})
}

func (m managedPaymentCommands) Release(ctx context.Context, command ports.ReleaseCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.Release(ctx, command)
	})
}

func (m managedPaymentCommands) PaymentDisputeDebit(ctx context.Context, command ports.PaymentDisputeDebitCommand) (ports.PaymentDisputeDebitResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.PaymentDisputeDebitResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.PaymentDisputeDebitResult, error) {
		return commands.PaymentDisputeDebit(ctx, command)
	})
}

func (m managedPaymentCommands) PaymentDisputeRestore(ctx context.Context, command ports.PaymentDisputeRestoreCommand) (ports.CommandResult, error) {
	svc, err := m.service.requireCommands()
	if err != nil {
		return ports.CommandResult{}, err
	}
	return commands.ManagedCommands(ctx, svc, func(commands ports.Commands) (ports.CommandResult, error) {
		return commands.PaymentDisputeRestore(ctx, command)
	})
}

// Payment returns the pool-owned Payments face
// (Purchase/PurchaseRefundHold/PurchaseRefund/Release/dispute pair). It
// never returns nil: a misconfigured service yields a view whose calls
// fail with the service-unavailable error, matching the previous
// pass-through behavior.
func (s *Service) Payment() ports.PaymentCommands {
	return managedPaymentCommands{service: s}
}

// Settlement returns the pool-owned workspace-spending face
// (Hold/Capture/Release). It never returns nil: a misconfigured service
// yields a view whose calls fail with the service-unavailable error,
// matching the previous pass-through behavior.
func (s *Service) Settlement() ports.SettlementCommands {
	return managedSettlementCommands{service: s}
}

func (s *Service) RestrictWallet(
	ctx context.Context,
	command ports.RestrictWalletCommand,
) (ports.WalletRestrictionChange, error) {
	service, err := s.requireWallet()
	if err != nil {
		return ports.WalletRestrictionChange{}, err
	}
	return service.RestrictWallet(ctx, command)
}

func (s *Service) ReleaseWalletRestriction(
	ctx context.Context,
	command ports.ReleaseWalletRestrictionCommand,
) (ports.WalletRestrictionChange, error) {
	service, err := s.requireWallet()
	if err != nil {
		return ports.WalletRestrictionChange{}, err
	}
	return service.ReleaseWalletRestriction(ctx, command)
}

func (s *Service) GetAvailableBalances(
	ctx context.Context,
	billingAccountIDs []int64,
) (map[int64]int64, error) {
	service, err := s.requireReadModel()
	if err != nil {
		return nil, err
	}
	return service.GetAvailableBalances(ctx, billingAccountIDs)
}

func (s *Service) GetWallet(ctx context.Context, billingAccountID int64) (domain.WalletDetails, error) {
	service, err := s.requireReadModel()
	if err != nil {
		return domain.WalletDetails{}, err
	}
	return service.GetWallet(ctx, billingAccountID)
}

// ResolvePersonalAccount ensures and returns the deterministic initial
// personal Billing Account for one user on the pool (#916). Transactional
// callers use ResolvePersonalAccountInTx instead.
func (s *Service) ResolvePersonalAccount(ctx context.Context, userID int64) (int64, error) {
	if err := s.serviceError(); err != nil {
		return 0, err
	}
	if s == nil || s.pool == nil {
		return 0, errServiceUnavailable
	}
	if userID <= 0 {
		return 0, domain.ErrInvalidCommand
	}
	// Join the ambient guard transaction when staged (control audit
	// atomicity): the ensure is idempotent, and the caller-owned commit
	// still owns visibility. Without a carrier the pool path runs.
	if ambient, ok := dbutil.AmbientTxFromContext(ctx); ok {
		var accountID int64
		if err := ambient.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
			return 0, err
		}
		if accountID <= 0 {
			return 0, domain.ErrWalletNotFound
		}
		return accountID, nil
	}
	var accountID int64
	if err := s.pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		return 0, err
	}
	if accountID <= 0 {
		return 0, domain.ErrWalletNotFound
	}
	return accountID, nil
}

func (s *Service) ListLedger(ctx context.Context, input domain.ListLedgerInput) (domain.LedgerPage, error) {
	service, err := s.requireReadModel()
	if err != nil {
		return domain.LedgerPage{}, err
	}
	return service.ListLedger(ctx, input)
}

func (s *Service) GetCaptureUsage(
	ctx context.Context,
	billingAccountID int64,
	filter domain.UsageFilter,
	location *time.Location,
) (totalCredits int64, peakDailyCredits int64, firstCaptureDay time.Time, err error) {
	service, err := s.requireUsage()
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	return service.GetCaptureUsage(ctx, billingAccountID, filter, location)
}

func (s *Service) GetDailyCaptureUsage(
	ctx context.Context,
	billingAccountID int64,
	filter domain.UsageFilter,
	location *time.Location,
	start time.Time,
	endExclusive time.Time,
) (map[string]int64, error) {
	service, err := s.requireUsage()
	if err != nil {
		return nil, err
	}
	return service.GetDailyCaptureUsage(ctx, billingAccountID, filter, location, start, endExclusive)
}

func (s *Service) GetCreditHealth(ctx context.Context) (health.CreditHealth, error) {
	service, err := s.requireHealth()
	if err != nil {
		return health.CreditHealth{}, err
	}
	return service.GetCreditHealth(ctx)
}

func (s *Service) StartReconciliation(
	ctx context.Context,
	input ports.StartManualReconciliationInput,
) (ports.ReconciliationRun, error) {
	service, err := s.requireReconciliation()
	if err != nil {
		return ports.ReconciliationRun{}, err
	}
	return service.StartReconciliation(ctx, input)
}

func (s *Service) ScheduleReconciliation(
	ctx context.Context,
	scheduledFor time.Time,
	onlyIfOverdue bool,
) (*ports.ReconciliationRun, error) {
	service, err := s.requireReconciliation()
	if err != nil {
		return nil, err
	}
	return service.ScheduleReconciliation(ctx, scheduledFor, onlyIfOverdue)
}

func (s *Service) ExecuteReconciliation(ctx context.Context, runID int64) error {
	service, err := s.requireReconciliation()
	if err != nil {
		return err
	}
	return service.ExecuteReconciliation(ctx, runID)
}

func (s *Service) FailReconciliation(ctx context.Context, runID int64, cause error) error {
	service, err := s.requireReconciliation()
	if err != nil {
		return err
	}
	return service.FailReconciliation(ctx, runID, cause)
}

func (s *Service) GetReconciliationRun(ctx context.Context, runID int64) (ports.ReconciliationRun, error) {
	service, err := s.requireReconciliation()
	if err != nil {
		return ports.ReconciliationRun{}, err
	}
	return service.GetReconciliationRun(ctx, runID)
}

func (s *Service) ListReconciliationRuns(ctx context.Context, input ports.ListReconciliationInput) (ports.ReconciliationRunPage, error) {
	service, err := s.requireReconciliation()
	if err != nil {
		return ports.ReconciliationRunPage{}, err
	}
	return service.ListReconciliationRuns(ctx, input)
}

func (s *Service) ListGlobalReconciliationFindings(ctx context.Context, status ports.ReconciliationFindingStatus, input ports.ListReconciliationInput) (ports.ReconciliationFindingPage, error) {
	service, err := s.requireReconciliation()
	if err != nil {
		return ports.ReconciliationFindingPage{}, err
	}
	return service.ListGlobalReconciliationFindings(ctx, status, input)
}
