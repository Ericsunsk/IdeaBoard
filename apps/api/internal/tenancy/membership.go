package tenancy

// Workspace and organization membership projections and grant-cleanup seams.

import (
	"context"
	"time"

	eventoutbox "ideaboard/api/internal/events/outbox"
)

// WorkspaceMember is one workspace_memberships row projected for the API.
type WorkspaceMember struct {
	WorkspacePublicID string
	UserID            int64
	UserExternalID    string
	Role              string
	CreatedAt         time.Time
}

// MemberGrantCleanup describes a Board access change caused by membership departure.
type MemberGrantCleanup struct {
	BoardPublicID string
	AccessVersion int64
	AccessSignals []eventoutbox.BoardAccessOutboxRecord
}

// WorkspaceMemberGrantStore joins Board access cleanup to a membership departure.
type WorkspaceMemberGrantStore interface {
	RemoveWorkspaceMemberGrants(ctx context.Context, workspacePublicID string, userID int64) ([]MemberGrantCleanup, error)
}

// OrganizationMember is one organization_memberships row projected for the API.
type OrganizationMember struct {
	OrganizationPublicID string
	UserID               int64
	UserExternalID       string
	Role                 string
	CreatedAt            time.Time
}
