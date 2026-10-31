package tenancy

// Shared identity/display batch adapters for member and billing projections.
// Required public identities fail closed; display facts stay best-effort.

import (
	"context"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

var errUserDirectoryMissing = shared.ErrUserDirectoryMissing
var errUserPublicIDsUnavailable = shared.ErrUserPublicIDsUnavailable

// batchUserPublicIDs resolves many numeric user ids to usr_ ids in ONE
// directory round trip. Unknown ids fail closed with 503, matching the
// single-row toMemberDTO behavior (member rows must never silently drop).
// A nil directory renders 501, matching the single-row guards.
func batchUserPublicIDs(ctx context.Context, c *gin.Context, dir runtimehttp.V1UserDirectory, userIDs []int64) (map[int64]string, bool) {
	return shared.BatchUserPublicIDs(ctx, c, dir, userIDs)
}

func renderUserPublicIDsError(c *gin.Context, err error) {
	shared.RenderUserPublicIDsError(c, err)
}

// readUserPublicIDs is shared by HTTP projections and the overview read
// adapter. It never renders a response and preserves the snapshot carrier.
func readUserPublicIDs(ctx context.Context, dir runtimehttp.V1UserDirectory, userIDs []int64) (map[int64]string, error) {
	return shared.ReadUserPublicIDs(ctx, dir, userIDs)
}

// batchDisplayFacts resolves workspace-visible display facts (Auth-owned
// email plus avatar public URL) in ONE provider round trip. A nil provider,
// lookup error, or missing fact resolves to omitted fields, never failing
// the request (same fail-closed shape as enrichMemberDisplay).
func batchDisplayFacts(ctx context.Context, provider V1MemberDisplayProvider, userIDs []int64) map[int64]V1MemberDisplay {
	return shared.BatchDisplayFacts(ctx, provider, userIDs)
}

// applyDisplayFact projects one display fact onto email/avatar fields:
// empty or missing facts stay omitted, never failing the request.
func applyDisplayFact(display map[int64]V1MemberDisplay, userID int64) (email, avatar string) {
	return shared.ApplyDisplayFact(display, userID)
}

// parseExpandQuery parses a comma-separated ?expand= list against the
// allowed set (default: nothing expanded). Unknown values render 400
// bad_request (canonical generic bad params); empty means no expansion.
func parseExpandQuery(c *gin.Context, allowed ...string) (map[string]bool, bool) {
	raw := strings.TrimSpace(c.Query("expand"))
	out := make(map[string]bool, len(allowed))
	if raw == "" {
		return out, true
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, ok := allowedSet[name]; !ok {
			contract.RenderErrorWithDetails(c, contract.StatusOf(contract.CodeBadRequest), contract.CodeBadRequest,
				runtimehttp.MsgBadRequest4, contract.FieldErrors{Fields: map[string]string{"expand": "invalid"}})
			c.Abort()
			return nil, false
		}
		out[name] = true
	}
	return out, true
}

// userIDsOfWorkspaceMembers collects member user ids in list order.
func userIDsOfWorkspaceMembers(members []*domaintenancy.WorkspaceMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m == nil || m.UserID <= 0 {
			continue
		}
		ids = append(ids, m.UserID)
	}
	return ids
}

// userIDsOfOrganizationMembers collects member user ids in list order.
func userIDsOfOrganizationMembers(members []*domaintenancy.OrganizationMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m == nil || m.UserID <= 0 {
			continue
		}
		ids = append(ids, m.UserID)
	}
	return ids
}
