package shared

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

var ErrUserDirectoryMissing = errors.New("user directory is not configured")
var ErrUserPublicIDsUnavailable = errors.New("user public IDs are unavailable")

// BatchUserPublicIDs resolves many numeric user ids to usr_ ids in ONE
// directory round trip. Unknown ids fail closed with 503, matching the
// single-row toMemberDTO behavior (member rows must never silently drop).
// A nil directory renders 501, matching the single-row guards.
func BatchUserPublicIDs(ctx context.Context, c *gin.Context, dir runtimehttp.V1UserDirectory, userIDs []int64) (map[int64]string, bool) {
	ids, err := ReadUserPublicIDs(ctx, dir, userIDs)
	if err != nil {
		RenderUserPublicIDsError(c, err)
		return nil, false
	}
	return ids, true
}

// RenderUserPublicIDsError renders the appropriate error for user public ID failures.
func RenderUserPublicIDsError(c *gin.Context, err error) {
	if errors.Is(err, ErrUserDirectoryMissing) {
		contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
	} else {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
	}
	c.Abort()
}

// ReadUserPublicIDs is shared by HTTP projections and the overview read
// adapter. It never renders a response and preserves the snapshot carrier.
func ReadUserPublicIDs(ctx context.Context, dir runtimehttp.V1UserDirectory, userIDs []int64) (map[int64]string, error) {
	if dir == nil {
		return nil, ErrUserDirectoryMissing
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
		return nil, fmt.Errorf("%w: %w", ErrUserPublicIDsUnavailable, err)
	}
	out := make(map[int64]string, len(deduped))
	for _, id := range deduped {
		raw, ok := publicIDs[id]
		if !ok || raw == uuid.Nil {
			return nil, ErrUserPublicIDsUnavailable
		}
		out[id] = tenancy.FormatUserPublicID(raw)
	}
	return out, nil
}

// BatchDisplayFacts resolves workspace-visible display facts (Auth-owned
// email plus avatar public URL) in ONE provider round trip. A nil provider,
// lookup error, or missing fact resolves to omitted fields, never failing
// the request (same fail-closed shape as enrichMemberDisplay).
func BatchDisplayFacts(ctx context.Context, provider MemberDisplayProvider, userIDs []int64) map[int64]MemberDisplay {
	out := make(map[int64]MemberDisplay, len(userIDs))
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

// ApplyDisplayFact projects one display fact onto email/avatar fields:
// empty or missing facts stay omitted, never failing the request.
func ApplyDisplayFact(display map[int64]MemberDisplay, userID int64) (email, avatar string) {
	fact, ok := display[userID]
	if !ok {
		return "", ""
	}
	return strings.TrimSpace(fact.Email), strings.TrimSpace(fact.Avatar)
}
