package workspacetasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/generation/routing"
	tasks "ideaboard/api/internal/tasking/domain"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
	"ideaboard/api/internal/workflow"
)

type ReadinessDeps struct {
	Workspaces      taskports.ReadinessWorkspaceReader
	Boards          taskports.BoardAuthorizer
	BillingAccounts taskports.ReadinessFundsReader
	Wallets         taskports.ReadinessWalletReader
	Quotas          taskports.ReadinessQuotaReader
	Quoter          taskports.Quoter
	Resolver        *routing.RoutingResolver
}

// Readiness owns advisory Task eligibility. It has no settlement, Hold, Task
// persistence or queue capability; Create remains the authoritative fence.
type Readiness struct {
	deps ReadinessDeps
}

var _ taskports.ReadinessChecker = (*Readiness)(nil)

func NewReadiness(deps ReadinessDeps) *Readiness { return &Readiness{deps: deps} }

func (r *Readiness) Check(ctx context.Context, input taskports.ReadinessParams) (tasks.BillingSnapshot, error) {
	boardID := strings.TrimSpace(input.BoardID)
	if err := tasks.ValidateBoardID(boardID); err != nil {
		return tasks.BillingSnapshot{}, err
	}
	if r.deps.Workspaces == nil {
		return tasks.BillingSnapshot{}, tasks.ErrSettlementUnavailable
	}
	workspace, err := r.deps.Workspaces.ResolveReadinessWorkspace(ctx, input.WorkspaceID, input.ActorUserID)
	if err != nil {
		return tasks.BillingSnapshot{}, err
	}
	if boardID != "" {
		if r.deps.Boards == nil {
			return tasks.BillingSnapshot{}, tasks.ErrSettlementUnavailable
		}
		access, err := r.deps.Boards.ResolveBoardAccess(ctx, input.WorkspaceID, boardID, input.ActorUserID, workspace.Role)
		if err != nil {
			return tasks.BillingSnapshot{}, err
		}
		var facts tasks.BoardTaskFacts
		if access != nil {
			facts = tasks.BoardTaskFacts{CanRead: access.CanRead, CanGenerate: access.CanGenerate, CanWrite: access.CanWrite, Archived: access.Archived}
		}
		switch tasks.BoardPolicy.Check(facts, tasks.BoardOpSubmit) {
		case tasks.BoardDecisionAllow:
		case tasks.BoardDecisionArchived:
			return tasks.BillingSnapshot{}, tasks.ErrBoardArchived
		case tasks.BoardDecisionForbidden:
			return tasks.BillingSnapshot{}, tasks.ErrForbidden
		default:
			return tasks.BillingSnapshot{}, tasks.ErrBoardNotFound
		}
	}
	if r.deps.BillingAccounts == nil || r.deps.Wallets == nil {
		return tasks.BillingSnapshot{}, tasks.ErrSettlementUnavailable
	}
	account, err := r.deps.BillingAccounts.ResolveFundsForActor(ctx, input.WorkspaceID, input.ActorUserID)
	if err != nil {
		return tasks.BillingSnapshot{}, &taskports.FundsResolutionError{Cause: err}
	}
	snapshot, err := r.priceQuote(ctx, tasks.QuoteParams{Service: input.Service, Quantity: input.Quantity, Resolution: input.Resolution})
	if err != nil {
		return tasks.BillingSnapshot{}, err
	}
	wallet, err := r.deps.Wallets.GetWallet(ctx, account.BillingAccountID)
	if err != nil {
		if errors.Is(err, creditsdomain.ErrWalletRestricted) {
			return tasks.BillingSnapshot{}, tasks.ErrWalletRestricted
		}
		return tasks.BillingSnapshot{}, err
	}
	if err := tasks.CheckFundsGate(wallet.Available, snapshot.TotalCredits, len(wallet.ActiveRestrictions) > 0); err != nil {
		return tasks.BillingSnapshot{}, err
	}
	if r.deps.Quotas != nil && snapshot.TotalCredits > 0 && workspace.Organization {
		if err := r.deps.Quotas.CheckQuotaForQuote(ctx, account.BillingAccountID, input.ActorUserID, snapshot.TotalCredits, snapshot.QuotedAt); err != nil {
			return tasks.BillingSnapshot{}, err
		}
	}
	return snapshot, nil
}

func (r *Readiness) priceQuote(ctx context.Context, params tasks.QuoteParams) (tasks.BillingSnapshot, error) {
	if r.deps.Quoter == nil {
		return tasks.BillingSnapshot{}, fmt.Errorf("workspacetasks: quoter unavailable: %w", tasks.ErrQuoteUnavailable)
	}
	kind, err := tasks.ValidateCreate(params.Service, nil, map[string]any{})
	if err != nil {
		return tasks.BillingSnapshot{}, err
	}
	quantity := params.Quantity
	if quantity <= 0 {
		quantity = 1
	}
	resolution := strings.TrimSpace(params.Resolution)
	if resolution == "" {
		resolution = workflow.OutputResolution1K
	}
	if _, err := r.resolveSnapshot(ctx, kind, resolution); err != nil {
		return tasks.BillingSnapshot{}, err
	}
	snapshot, err := r.deps.Quoter.Quote(ctx, kind, resolution, quantity)
	if err != nil {
		return tasks.BillingSnapshot{}, err
	}
	snapshot.SettlementState = "quoted"
	return snapshot, nil
}

func (r *Readiness) resolveSnapshot(ctx context.Context, service, resolution string) (routing.RoutingSnapshot, error) {
	if r.deps.Resolver == nil {
		requested := routing.Resolution(strings.ToLower(strings.TrimSpace(resolution)))
		return routing.RoutingSnapshot{}, &routing.ResolutionUnavailableError{Service: service, Resolution: requested}
	}
	snap, err := r.deps.Resolver.Resolve(ctx, service, resolution)
	if err != nil {
		var inputErr *routing.InvalidRoutingInputError
		if errors.As(err, &inputErr) {
			return routing.RoutingSnapshot{}, tasks.NewValidationError(tasks.FieldError{Field: inputErr.Field, Message: "invalid"})
		}
		return routing.RoutingSnapshot{}, fmt.Errorf("workspacetasks: routing unavailable: %w", err)
	}
	return snap, nil
}
