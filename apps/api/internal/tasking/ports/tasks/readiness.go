package tasks

import (
	"context"
	"time"

	creditsdomain "ideaboard/api/internal/credits/domain"
	taskdomain "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tenancy/funds"
)

// ReadinessChecker checks advisory eligibility without reserving funds or work.
type ReadinessChecker interface {
	Check(context.Context, ReadinessParams) (taskdomain.BillingSnapshot, error)
}

type ReadinessParams struct {
	WorkspaceID string
	ActorUserID int64
	BoardID     string
	Service     string
	Resolution  string
	Quantity    int64
}

// ReadinessWorkspace contains current membership and consumption-scope facts.
// Organization affiliation, rather than a funds label, determines quota scope.
type ReadinessWorkspace struct {
	Role         string
	Organization bool
}

type ReadinessWorkspaceReader interface {
	ResolveReadinessWorkspace(context.Context, string, int64) (ReadinessWorkspace, error)
}

type ReadinessFundsReader interface {
	ResolveFundsForActor(context.Context, string, int64) (funds.Identity, error)
}

type ReadinessWalletReader interface {
	GetWallet(context.Context, int64) (creditsdomain.WalletDetails, error)
}

type ReadinessQuotaReader interface {
	CheckQuotaForQuote(context.Context, int64, int64, int64, time.Time) error
}

// FundsResolutionError preserves the funds failure identity separately from
// task eligibility failures; callers retain their existing account error mapping.
type FundsResolutionError struct {
	Cause error
}

func (e *FundsResolutionError) Error() string { return "task readiness funds: " + e.Cause.Error() }
func (e *FundsResolutionError) Unwrap() error { return e.Cause }
