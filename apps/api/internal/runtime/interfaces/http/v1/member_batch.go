package v1

// Shared identity/display batch adapters for member and billing projections.
// Required public identities fail closed; display facts stay best-effort.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var errUserDirectoryMissing = errors.New("user directory is not configured")
var errUserPublicIDsUnavailable = errors.New("user public IDs are unavailable")

// batchUserPublicIDs resolves many numeric user ids to usr_ ids in ONE
// directory round trip. Unknown ids fail closed with 503, matching the
// single-row toMemberDTO behavior (member rows must never silently drop).
// A nil directory renders 501, matching the single-row guards.
func batchUserPublicIDs(ctx context.Context, c *gin.Context, dir runtimehttp.V1UserDirectory, userIDs []int64) (map[int64]string, bool) {
	ids, err := readUserPublicIDs(ctx, dir, userIDs)
	if err != nil {
		renderUserPublicIDsError(c, err)
		return nil, false
	}
	return ids, true
}

func renderUserPublicIDsError(c *gin.Context, err error) {
	if errors.Is(err, errUserDirectoryMissing) {
		contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
	} else {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
	}
	c.Abort()
}

// readUserPublicIDs is shared by HTTP projections and the overview read
// adapter. It never renders a response and preserves the snapshot carrier.
func readUserPublicIDs(ctx context.Context, dir runtimehttp.V1UserDirectory, userIDs []int64) (map[int64]string, error) {
	if dir == nil {
		return nil, errUserDirectoryMissing
	}
	seen := make(map[int64]struct{}, len(userIDs))
	deduped := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	publicIDs, err := dir.GetUserPublicIDs(ctx, deduped)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUserPublicIDsUnavailable, err)
	}
	out := make(map[int64]string, len(deduped))
	for _, id := range deduped {
		raw, ok := publicIDs[id]
		if !ok || raw == uuid.Nil {
			return nil, errUserPublicIDsUnavailable
		}
		out[id] = tenancy.FormatUserPublicID(raw)
	}
	return out, nil
}

// batchDisplayFacts resolves workspace-visible display facts (Auth-owned
// email plus avatar public URL) in ONE provider round trip. A nil provider,
// lookup error, or missing fact resolves to omitted fields, never failing
// the request (same fail-closed shape as enrichMemberDisplay).
func batchDisplayFacts(ctx context.Context, provider V1MemberDisplayProvider, userIDs []int64) map[int64]V1MemberDisplay {
	out := make(map[int64]V1MemberDisplay, len(userIDs))
	if provider == nil || len(userIDs) == 0 {
		return out
	}
	seen := make(map[int64]struct{}, len(userIDs))
	deduped := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return out
	}
	facts, err := provider.GetMemberDisplay(ctx, deduped)
	if err != nil {
		return out
	}
	for id, fact := range facts {
		out[id] = fact
	}
	return out
}

// applyDisplayFact projects one display fact onto email/avatar fields:
// empty or missing facts stay omitted, never failing the request.
func applyDisplayFact(display map[int64]V1MemberDisplay, userID int64) (email, avatar string) {
	fact, ok := display[userID]
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(fact.Email), strings.TrimSpace(fact.Avatar)
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
func userIDsOfWorkspaceMembers(members []*tenancy.WorkspaceMember) []int64 {
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
func userIDsOfOrganizationMembers(members []*tenancy.OrganizationMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m == nil || m.UserID <= 0 {
			continue
		}
		ids = append(ids, m.UserID)
	}
	return ids
}

// userIDsOfBillingMembers collects billing manager user ids.
func userIDsOfBillingMembers(members []tenancy.BillingAccountMember) []int64 {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m.UserID <= 0 {
			continue
		}
		ids = append(ids, m.UserID)
	}
	return ids
}
