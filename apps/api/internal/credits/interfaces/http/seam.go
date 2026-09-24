package http

// Legacy /api/user and /api/admin Credits handlers were removed in #809
// (Public reads moved to /api/v1 workspace billing, administration to
// /api/control/v1 credits). This package now carries only the shared
// identity/service seams consumed by the Control Plane; HTTP rendering
// lives in platform/contract.

import (
	"context"

	"ideaboard/api/internal/credits/application/health"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"

	"github.com/google/uuid"
)

type AdminCommandService interface {
	Grant(context.Context, ports.GrantCommand) (ports.CommandResult, error)
	Deduct(context.Context, ports.DeductionCommand) (ports.CommandResult, error)
	Correct(context.Context, ports.CorrectionCommand) (ports.CommandResult, error)
	Refund(context.Context, ports.RefundCommand) (ports.CommandResult, error)
}

type UserWalletService interface {
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
}

type AdminReadService interface {
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
	ListLedger(context.Context, domain.ListLedgerInput) (domain.LedgerPage, error)
}

type ReconciliationService interface {
	GetCreditHealth(context.Context) (health.CreditHealth, error)
	StartReconciliation(context.Context, ports.StartManualReconciliationInput) (ports.ReconciliationRun, error)
	GetReconciliationRun(context.Context, int64) (ports.ReconciliationRun, error)
	ListReconciliationRuns(context.Context, ports.ListReconciliationInput) (ports.ReconciliationRunPage, error)
	ListGlobalReconciliationFindings(context.Context, ports.ReconciliationFindingStatus, ports.ListReconciliationInput) (ports.ReconciliationFindingPage, error)
}

// UserIdentity is the HTTP identity projection shared by Credits inbound
// adapters. The resolver boundary keeps Credits independent from the users
// lifecycle package (which itself depends on Credits for Wallet lifecycle).
type UserIdentity struct {
	ID          int64
	PublicID    uuid.UUID
	DisplayName string
}

type UserPublicIDResolver interface {
	ResolvePublicID(context.Context, string) (UserIdentity, error)
}

type UserIDResolver interface {
	ResolveID(context.Context, int64) (UserIdentity, error)
}

type AdminReadIdentityResolver interface {
	UserPublicIDResolver
	UserIDResolver
}
