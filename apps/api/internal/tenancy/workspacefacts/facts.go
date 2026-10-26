// Package workspacefacts reads shared Workspace facts in the caller transaction.
// It does not decide membership, authorization, or the paying Billing Account.
package workspacefacts

import (
	"context"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

// ResolveInternalID maps an opaque workspace public id to its
// internal id. The input is trimmed before lookup; the query error returns
// unchanged so callers map "not found" to their own operation-scoped
// sentinel (hidden workspaces must never leak a distinct status). Callers
// that fail closed on malformed ids keep their own shape pre-check.
//
// Reads use the caller-supplied Queries and never open a transaction.
func ResolveInternalID(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (int64, error) {
	row, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspacePublicID))
	if err != nil {
		return 0, err
	}
	return row.InternalID, nil
}

// IsOrganizationClosed reports whether the workspace belongs
// to a closed Organization (#955). Personal scopes are open; missing rows
// and query failures propagate so an unknown state never grants access.
// Reads use the caller-supplied Queries. Authorization decisions and error
// mapping stay with the caller.
func IsOrganizationClosed(ctx context.Context, q *dbsqlc.Queries, workspacePublicID string) (bool, error) {
	row, err := q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(workspacePublicID))
	if err != nil {
		return false, err
	}
	if !row.OrganizationID.Valid {
		return false, nil
	}
	org, err := q.GetOrganizationByInternalID(ctx, row.OrganizationID.Int64)
	if err != nil {
		return false, err
	}
	return org.ClosedAt.Valid, nil
}
