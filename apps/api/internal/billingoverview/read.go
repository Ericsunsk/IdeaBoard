// Package billingoverview composes the Billing Account panel read. Credits,
// Usage, MemberQuotas and Tenancy retain ownership of their facts and writes.
package billingoverview

import (
	"context"
	"errors"
	"sync"

	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/memberquotas"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/funds"
	usageapplication "ideaboard/api/internal/usage/application"
	usagedomain "ideaboard/api/internal/usage/domain"
)

type AccountReader interface {
	ResolveFundsForActor(ctx context.Context, workspacePublicID string, actorUserID int64) (funds.Identity, error)
	GetBillingAccountRole(context.Context, int64, int64) (string, bool, error)
	ListBillingAccountMembers(context.Context, int64, int64) ([]tenancy.BillingAccountMember, error)
	ListBillingTransferInvitesForAccount(context.Context, int64, int64, int64, int) ([]*tenancy.BillingTransferInvite, error)
}

type TenancyReader interface {
	memberquotas.OrganizationRoleReader
	ListWorkspaceMembers(context.Context, string) ([]*tenancy.WorkspaceMember, error)
}

type WalletReader interface {
	GetWallet(context.Context, int64) (creditsdomain.WalletDetails, error)
}
type UsageReader interface {
	GetCurrent(context.Context, int64, creditsdomain.UsageFilter, usagedomain.Range) (usageapplication.Usage, error)
}

// Person combines required public identity with best-effort display facts.
type Person struct{ PublicID, Email, Avatar string }
type PeopleReader func(context.Context, []int64) (map[int64]Person, error)
type Snapshot func(context.Context, func(context.Context) error) error

// Reader owns provisioning order, visibility, snapshot context and background
// read lifetime. Snapshot is required; in-memory adapters may invoke the
// callback directly. Every snapshot participant receives its context carrier.
type Reader struct {
	Accounts AccountReader
	Tenancy  TenancyReader
	Wallets  WalletReader
	Usage    UsageReader
	Quotas   memberquotas.ReadStore
	People   PeopleReader
	Snapshot Snapshot
}

type Input struct {
	Workspace     *tenancy.Workspace
	WorkspaceRole string
	ActorUserID   int64
	Range         usagedomain.Range
}

type Result struct {
	Account         tenancy.BillingAccount
	Scope           string
	CanManageQuotas bool
	Wallet          creditsdomain.WalletDetails
	Usage           usageapplication.Usage
	Quota           memberquotas.Reading
	Quotas          []memberquotas.Reading
	BillingMembers  []tenancy.BillingAccountMember
	LiveInvites     []*tenancy.BillingTransferInvite
	MembersCount    int
	People          map[int64]Person
}

// Stage identifies the failed owner read; transport adapters map its cause.
type Stage uint8

const (
	Account Stage = iota
	WorkspaceMembers
	Quota
	People
	SnapshotRead
	Wallet
	Usage
)

type Error struct {
	Stage Stage
	Err   error
}

func (e *Error) Error() string            { return "billing overview: " + e.Err.Error() }
func (e *Error) Unwrap() error            { return e.Err }
func failed(stage Stage, err error) error { return &Error{Stage: stage, Err: err} }

func (r Reader) Read(ctx context.Context, input Input) (Result, error) {
	var result Result
	switch {
	case r.Quotas == nil:
		return result, failed(Quota, errors.New("quota reader is not configured"))
	case r.Wallets == nil:
		return result, failed(Wallet, errors.New("wallet reader is not configured"))
	case r.Usage == nil:
		return result, failed(Usage, errors.New("usage reader is not configured"))
	case r.Accounts == nil || r.Tenancy == nil:
		return result, failed(Account, errors.New("account reader is not configured"))
	case r.Snapshot == nil:
		return result, failed(SnapshotRead, errors.New("snapshot reader is not configured"))
	}
	// First-touch provisioning must precede the read-only transaction.
	// Personal scopes resolve the viewer's own account (payer = actor,
	// #973/#978) so members each see their own (BillingAccount, User)
	// column with zero cross pollution; Org scopes keep the shared pool
	// (actor-independent, same as the owner order).
	identity, err := r.Accounts.ResolveFundsForActor(ctx, input.Workspace.PublicID, input.ActorUserID)
	if err != nil {
		return result, failed(Account, err)
	}
	if identity.BillingAccountID <= 0 || identity.BA() == "" {
		return result, failed(Account, tenancy.ErrBillingNotFound)
	}
	account := identity.ToBillingAccount()
	result.Account = account
	var filter creditsdomain.UsageFilter
	var role string
	var member bool
	if input.Workspace.IsPersonal() {
		filter.ActorUserID = input.ActorUserID
		result.Scope = "member"
	} else {
		role, member, err = r.Accounts.GetBillingAccountRole(ctx, account.InternalID, input.ActorUserID)
		if err != nil {
			return result, failed(Account, err)
		}
		filter, result.Scope = ConsumptionScope(input.Workspace.InternalID, input.ActorUserID, input.WorkspaceRole, role, member)
		policy := memberquotas.Policy{OrganizationRoles: r.Tenancy}
		result.CanManageQuotas, err = policy.CanManage(ctx, memberquotas.ScopeForWorkspace(input.Workspace), input.ActorUserID)
		if err != nil {
			return result, failed(Account, err)
		}
	}

	workCtx, cancel := context.WithCancel(ctx)
	var group sync.WaitGroup
	// Cancel and join on every exit; snapshot failures take precedence over
	// concurrent wallet/usage failures and never return partial data.
	defer func() { cancel(); group.Wait() }()
	var wallet creditsdomain.WalletDetails
	var usage usageapplication.Usage
	var walletErr, usageErr error
	group.Go(func() { wallet, walletErr = r.Wallets.GetWallet(workCtx, account.InternalID) })
	group.Go(func() { usage, usageErr = r.Usage.GetCurrent(workCtx, account.InternalID, filter, input.Range) })
	err = r.Snapshot(ctx, func(snapCtx context.Context) error {
		return r.snapshotBody(snapCtx, input, member, role, &result)
	})
	if err != nil {
		var readErr *Error
		if errors.As(err, &readErr) {
			return Result{}, err
		}
		return Result{}, failed(SnapshotRead, err)
	}
	group.Wait()
	if walletErr != nil {
		return Result{}, failed(Wallet, walletErr)
	}
	if usageErr != nil {
		return Result{}, failed(Usage, usageErr)
	}
	result.Wallet, result.Usage = wallet, usage
	return result, nil
}

// snapshotBody is the single snapshot callback behind both scopes. Org
// scopes fan out manager/transfer/quota-list facts; Personal scopes read
// only their own unlimited quota with no financial-manager sections.
func (r Reader) snapshotBody(snapCtx context.Context, input Input, member bool, role string, result *Result) error {
	account := result.Account
	personal := input.Workspace.IsPersonal()
	members, err := r.Tenancy.ListWorkspaceMembers(snapCtx, input.Workspace.PublicID)
	if err != nil {
		return failed(WorkspaceMembers, err)
	}
	result.MembersCount = len(members)
	userIDs := []int64{}
	if !personal {
		if member && tenancy.IsBillingManager(role) {
			result.BillingMembers, err = r.Accounts.ListBillingAccountMembers(snapCtx, account.InternalID, input.ActorUserID)
			if err != nil {
				return failed(Account, err)
			}
			for _, m := range result.BillingMembers {
				userIDs = append(userIDs, m.UserID)
			}
		}
		invites, err := r.Accounts.ListBillingTransferInvitesForAccount(snapCtx, account.InternalID, input.ActorUserID, 0, 50)
		if err != nil {
			return failed(Account, err)
		}
		for _, invite := range invites {
			if invite == nil || invite.Status != tenancy.BillingTransferPending {
				continue
			}
			result.LiveInvites = append(result.LiveInvites, invite)
			userIDs = append(userIDs, invite.FromUserID, invite.ToUserID)
			if invite.DecidedBy != nil && *invite.DecidedBy > 0 {
				userIDs = append(userIDs, *invite.DecidedBy)
			}
		}
	}
	var quotaScope memberquotas.Scope
	if !personal {
		quotaScope = memberquotas.ScopeForWorkspace(input.Workspace)
	}
	policy := memberquotas.Policy{OrganizationRoles: r.Tenancy}
	quotas := memberquotas.Reader{Store: r.Quotas, Policy: policy}
	if personal {
		// Personal has no quota concept (#975): return the unlimited
		// constant directly without a Reader/Policy/Scope call.
		result.Quota = memberquotas.Reading{UserID: input.ActorUserID, Period: memberquotas.PeriodUnlimited}
	} else if result.CanManageQuotas {
		rows, err := r.Quotas.ListQuotas(snapCtx, account.InternalID)
		if err != nil {
			return failed(Quota, err)
		}
		result.Quotas, err = quotas.Page(snapCtx, quotaScope, account.InternalID, rows)
		if err != nil {
			return failed(Quota, err)
		}
		found := false
		for _, reading := range result.Quotas {
			userIDs = append(userIDs, reading.UserID)
			if reading.UserID == input.ActorUserID {
				result.Quota, found = reading, true
			}
		}
		if !found {
			result.Quota, err = quotas.Read(snapCtx, account.InternalID, input.ActorUserID, memberquotas.PeriodUnlimited, nil)
			if err != nil {
				return failed(Quota, err)
			}
		}
	} else {
		result.Quota, err = quotas.Self(snapCtx, quotaScope, account.InternalID, input.ActorUserID)
		if err != nil {
			return failed(Quota, err)
		}
	}
	if r.People == nil {
		return failed(People, errors.New("people reader is not configured"))
	}
	// Personal has no quota concept (#975): the unlimited constant above
	// already applies, and the manager list path stays disabled so no
	// write entry appears.
	result.People, err = r.People(snapCtx, userIDs)
	if err != nil {
		return failed(People, err)
	}
	return nil
}

// ConsumptionScope is shared with the itemized single reads. Workspace
// membership alone never grants account-wide financial visibility.
//
// Personal workspaces never use this helper: they force the member filter
// in Reader.Read and the billing handlers so each viewer reads
// only their own (BillingAccount, User) column. Org scopes keep this order.
func ConsumptionScope(workspaceID, actorUserID int64, workspaceRole, billingRole string, billingMember bool) (creditsdomain.UsageFilter, string) {
	if billingMember && tenancy.IsBillingManager(billingRole) {
		return creditsdomain.UsageFilter{}, "account"
	}
	if workspaceRole == tenancy.RoleOwner || workspaceRole == tenancy.RoleAdmin {
		return creditsdomain.UsageFilter{WorkspaceID: workspaceID}, "workspace"
	}
	return creditsdomain.UsageFilter{ActorUserID: actorUserID}, "member"
}
