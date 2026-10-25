package tenancy

// Workspace member invitation domain models and persistence seams (#957, ADR-0039).

import (
	"context"
	"time"
)

// WorkspaceMemberInvite represents an invitation to join a workspace.
type WorkspaceMemberInvite struct {
	InternalID        int64
	PublicID          string
	WorkspaceID       int64
	WorkspacePublicID string
	WorkspaceName     string
	InviterUserID     int64
	InviteeEmail      string
	InviteeUserID     *int64
	Role              string
	Status            string
	TokenHash         string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	RespondedAt       *time.Time
	RespondedByUserID *int64
}

// WorkspaceInviteStore groups workspace invitation persistence.
type WorkspaceInviteStore interface {
	CreateWorkspaceMemberInvite(ctx context.Context, workspacePublicID string, inviterUserID int64, inviteeEmail, role string, inviteeUserID int64) (*WorkspaceMemberInvite, string, error)
	ListWorkspaceMemberInvites(ctx context.Context, workspacePublicID string) ([]*WorkspaceMemberInvite, error)
	CancelWorkspaceMemberInvite(ctx context.Context, workspacePublicID, invitePublicID string, callerUserID int64) error
	ListPendingWorkspaceInvitesForUser(ctx context.Context, userID int64, userEmail string) ([]*WorkspaceMemberInvite, error)
	GetWorkspaceInviteByToken(ctx context.Context, rawToken string) (*WorkspaceMemberInvite, error)
	GetWorkspaceMemberInvite(ctx context.Context, invitePublicID string) (*WorkspaceMemberInvite, error)
	// Accept returns the member plus the authoritative post-decision
	// invite; Decline returns the authoritative post-decision invite.
	// Callers forward the returned invite to the notification path without
	// a pre-read snapshot or post-write re-fetch (see the unified core in
	// member_invites.go / postgres/member_invites.go).
	AcceptWorkspaceInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*WorkspaceMember, *WorkspaceMemberInvite, error)
	DeclineWorkspaceInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*WorkspaceMemberInvite, error)
}
