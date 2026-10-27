// Package invitations owns the atomic collaboration write: invitation/membership
// facts, in-app notifications, and durable mail enqueue share one transaction.
package invitations

import (
	"context"
	"errors"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transaction func(context.Context, func(context.Context) error) error

func PostgresTransaction(pool *pgxpool.Pool) Transaction {
	return func(ctx context.Context, fn func(context.Context) error) error {
		scope, err := dbutil.Begin(ctx, pool, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer scope.Rollback(ctx)
		txCtx := dbutil.ContextWithAmbientTx(scope.Context(), scope.Tx())
		if scope.Owned() {
			txCtx = dbutil.ContextWithAmbientPostCommit(txCtx)
		}
		if err := fn(txCtx); err != nil {
			return err
		}
		if err := scope.Commit(ctx); err != nil {
			return err
		}
		if scope.Owned() {
			dbutil.RunAmbientPostCommit(txCtx)
		}
		return nil
	}
}

type Store interface {
	tenancy.WorkspaceInviteStore
	tenancy.OrganizationInviteStore
}

type TransferStore interface {
	InitiateBillingTransferInvite(context.Context, int64, int64, int64, string) (*tenancy.BillingTransferInvite, error)
}

// Service commits invitation, notification facts, and mail enqueue atomically.
// Notification hints run only after the transaction owner commits.
type Service struct {
	Store       Store
	Identities  IdentityReader
	Transfers   TransferStore
	Events      tenancy.CollaborationEventDispatcher
	Transaction Transaction
}

func (s *Service) run(ctx context.Context, fn func(context.Context) error) error {
	if s == nil || s.Transaction == nil || s.Events == nil {
		return errors.New("invitations: atomic collaboration writes unavailable")
	}
	return s.Transaction(ctx, fn)
}

func (s *Service) CreateWorkspaceMemberInvite(ctx context.Context, workspaceID string, actorID int64, email, role string) (invite *tenancy.WorkspaceMemberInvite, token string, err error) {
	inviteeID, err := s.inviteeID(ctx, email)
	if err != nil {
		return nil, "", err
	}
	err = s.run(ctx, func(ctx context.Context) error {
		var err error
		invite, token, err = s.Store.CreateWorkspaceMemberInvite(ctx, workspaceID, actorID, email, role, inviteeID)
		if err != nil {
			return err
		}
		return s.Events.OnWorkspaceMemberInvited(ctx, invite, token)
	})
	if err != nil {
		return nil, "", err
	}
	return
}

func (s *Service) CancelWorkspaceMemberInvite(ctx context.Context, workspaceID, inviteID string, actorID int64) error {
	return s.run(ctx, func(ctx context.Context) error {
		if err := s.Store.CancelWorkspaceMemberInvite(ctx, workspaceID, inviteID, actorID); err != nil {
			return err
		}
		return s.Events.OnMemberInviteCancelled(ctx, inviteID)
	})
}

func (s *Service) AcceptWorkspaceInvite(ctx context.Context, ref string, actorID int64) (member *tenancy.WorkspaceMember, invite *tenancy.WorkspaceMemberInvite, err error) {
	email, err := s.decisionEmail(ctx, tenancy.InviteScopeWorkspace, ref, actorID)
	if err != nil {
		return nil, nil, err
	}
	err = s.run(ctx, func(ctx context.Context) error {
		var err error
		member, invite, err = s.Store.AcceptWorkspaceInvite(ctx, ref, actorID, email)
		if err != nil {
			return err
		}
		return s.Events.OnWorkspaceMemberInviteAccepted(ctx, invite, member, email)
	})
	if err != nil {
		return nil, nil, err
	}
	return
}

func (s *Service) DeclineWorkspaceInvite(ctx context.Context, ref string, actorID int64) (invite *tenancy.WorkspaceMemberInvite, err error) {
	email, err := s.decisionEmail(ctx, tenancy.InviteScopeWorkspace, ref, actorID)
	if err != nil {
		return nil, err
	}
	err = s.run(ctx, func(ctx context.Context) error {
		var err error
		invite, err = s.Store.DeclineWorkspaceInvite(ctx, ref, actorID, email)
		if err != nil {
			return err
		}
		return s.Events.OnWorkspaceMemberInviteDeclined(ctx, invite)
	})
	if err != nil {
		return nil, err
	}
	return
}

func (s *Service) CreateOrganizationMemberInvite(ctx context.Context, organizationID string, actorID int64, email, role string) (invite *tenancy.OrganizationMemberInvite, token string, err error) {
	inviteeID, err := s.inviteeID(ctx, email)
	if err != nil {
		return nil, "", err
	}
	err = s.run(ctx, func(ctx context.Context) error {
		var err error
		invite, token, err = s.Store.CreateOrganizationMemberInvite(ctx, organizationID, actorID, email, role, inviteeID)
		if err != nil {
			return err
		}
		return s.Events.OnOrganizationMemberInvited(ctx, invite)
	})
	if err != nil {
		return nil, "", err
	}
	return
}

func (s *Service) CancelOrganizationMemberInvite(ctx context.Context, organizationID, inviteID string, actorID int64) error {
	return s.run(ctx, func(ctx context.Context) error {
		if err := s.Store.CancelOrganizationMemberInvite(ctx, organizationID, inviteID, actorID); err != nil {
			return err
		}
		return s.Events.OnMemberInviteCancelled(ctx, inviteID)
	})
}

func (s *Service) AcceptOrganizationInvite(ctx context.Context, ref string, actorID int64) (member *tenancy.OrganizationMember, invite *tenancy.OrganizationMemberInvite, err error) {
	email, err := s.decisionEmail(ctx, tenancy.InviteScopeOrganization, ref, actorID)
	if err != nil {
		return nil, nil, err
	}
	err = s.run(ctx, func(ctx context.Context) error {
		var err error
		member, invite, err = s.Store.AcceptOrganizationInvite(ctx, ref, actorID, email)
		if err != nil {
			return err
		}
		return s.Events.OnOrganizationMemberInviteAccepted(ctx, invite, member, email)
	})
	if err != nil {
		return nil, nil, err
	}
	return
}

func (s *Service) DeclineOrganizationInvite(ctx context.Context, ref string, actorID int64) (invite *tenancy.OrganizationMemberInvite, err error) {
	email, err := s.decisionEmail(ctx, tenancy.InviteScopeOrganization, ref, actorID)
	if err != nil {
		return nil, err
	}
	err = s.run(ctx, func(ctx context.Context) error {
		var err error
		invite, err = s.Store.DeclineOrganizationInvite(ctx, ref, actorID, email)
		if err != nil {
			return err
		}
		return s.Events.OnOrganizationMemberInviteDeclined(ctx, invite)
	})
	if err != nil {
		return nil, err
	}
	return
}

func (s *Service) InitiateTransferInvite(ctx context.Context, accountID, fromID, toID int64, key string, params tenancy.BillingTransferInitiatedParams) (invite *tenancy.BillingTransferInvite, err error) {
	err = s.run(ctx, func(ctx context.Context) error {
		if s.Transfers == nil {
			return errors.New("invitations: transfer store unavailable")
		}
		var err error
		invite, err = s.Transfers.InitiateBillingTransferInvite(ctx, accountID, fromID, toID, key)
		if err != nil {
			return err
		}
		if invite.IdempotencyKey == "" {
			return errors.New("invitations: handover lifecycle identity missing")
		}
		// Reuses the unified notification value object so reissues and
		// retries collapse to one durable notification.
		params.NotificationKey = tenancy.TransferNotificationKey(invite.PublicID, invite.IdempotencyKey)
		return s.Events.OnBillingTransferInitiated(ctx, invite, params)
	})
	if err != nil {
		return nil, err
	}
	return
}
