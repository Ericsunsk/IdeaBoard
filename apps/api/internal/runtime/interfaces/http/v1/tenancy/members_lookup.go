package tenancy

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/autherrors"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	domaintenancy "ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// V1MemberLookupProvider resolves a registered user's public usr_ identity
// from an Auth-owned email for the workspace invite search. Read-only:
// the email is normalized and resolved per call, never persisted, and the
// response carries only the usr_ identifier (no email echo, no membership
// facts). Unknown emails converge to tenancy.ErrNotFound so unregistered
// identities and provision gaps stay indistinguishable.
type V1MemberLookupProvider interface {
	LookupUserPublicID(ctx context.Context, normalizedEmail string) (uuid.UUID, error)
}

// V1MemberAuthLookupReader is the narrow Auth seam for email resolution:
// one canonical email to one Auth identity id. Production wires the Auth
// ServiceClient; tests inject fakes.
type V1MemberAuthLookupReader interface {
	LookupUserByEmail(ctx context.Context, email string) (string, error)
}

// memberLookupService joins Auth-owned emails to business usr_ identities:
// Auth LookupUserByEmail resolves the AuthUserID, then users.GetByAuthUserID
// resolves the business row. Either hop missing converges to
// tenancy.ErrNotFound; transport failures surface so the handler renders
// 503 instead of a misleading 404.
type memberLookupService struct {
	users identityports.UserRepository
	auth  V1MemberAuthLookupReader
}

// NewMemberLookupService builds the production V1MemberLookupProvider over
// the users repository plus the Auth lookup reader. Either side may be nil
// in tests; lookups then resolve to not-found.
func NewMemberLookupService(users identityports.UserRepository, auth V1MemberAuthLookupReader) V1MemberLookupProvider {
	return &memberLookupService{users: users, auth: auth}
}

func (s *memberLookupService) LookupUserPublicID(ctx context.Context, normalizedEmail string) (uuid.UUID, error) {
	if s == nil || s.users == nil || s.auth == nil {
		return uuid.Nil, domaintenancy.ErrNotFound
	}
	email := NormalizeMemberLookupEmail(normalizedEmail)
	if !IsValidMemberLookupEmail(email) {
		return uuid.Nil, domaintenancy.ErrNotFound
	}
	authUserID, err := s.auth.LookupUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) || errors.Is(err, domaintenancy.ErrNotFound) {
			return uuid.Nil, domaintenancy.ErrNotFound
		}
		return uuid.Nil, err
	}
	user, err := s.users.GetByAuthUserID(ctx, strings.TrimSpace(authUserID))
	if err != nil || user == nil || user.PublicID == uuid.Nil {
		return uuid.Nil, domaintenancy.ErrNotFound
	}
	return user.PublicID, nil
}

// NormalizeMemberLookupEmail trims and lower-cases a raw invite-search
// email so matching is an exact, case-insensitive comparison.
func NormalizeMemberLookupEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// IsValidMemberLookupEmail mirrors the Auth canonical-email shape (max
// 320, single @, local part max 64, dotted domain): malformed input fails
// closed with 400 before any identity lookup runs.
func IsValidMemberLookupEmail(email string) bool {
	if email == "" || len(email) > 320 || strings.Contains(email, " ") {
		return false
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	if len(email[:at]) > 64 {
		return false
	}
	domain := email[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return true
}

// v1MemberLookupDTO is the query-only invite-search shape: the resolved
// usr_ identifier with no email echo and no membership facts.
type v1MemberLookupDTO struct {
	UserID string `json:"user_id"`
}

// lookupWorkspaceMemberByEmail resolves one registered user to its usr_
// identifier by exact email match for the invite flow. It performs no
// membership write: callers invite with POST members (resolved usr_ +
// role + Idempotency-Key). Unknown emails converge to 404
// workspace_member_not_found, indistinguishable from unregistered
// identities; the email is resolved through Auth per call and never stored.
func (h *Handler) lookupWorkspaceMemberByEmail(c *gin.Context) {
	h.withWorkspace(c, shared.WorkspaceOpts{Allow: shared.AllowOwnerOrAdmin}, func(wctx shared.WorkspaceCtx) {
		normalized := NormalizeMemberLookupEmail(c.Query("email"))
		if !IsValidMemberLookupEmail(normalized) {
			contract.RenderErrorWithDetails(c, http.StatusBadRequest, contract.CodeBadRequest,
				runtimehttp.MsgBadRequest35, contract.FieldErrors{Fields: map[string]string{"email": "invalid"}})
			c.Abort()
			return
		}
		if h.deps.Members.MemberLookup == nil {
			contract.RenderError(c, http.StatusNotImplemented,
				contract.CodeContractNotImplemented, "The v1 contract surface is not implemented yet")
			c.Abort()
			return
		}
		publicID, err := h.deps.Members.MemberLookup.LookupUserPublicID(c.Request.Context(), normalized)
		if err == nil && publicID != uuid.Nil {
			contract.RenderSuccess(c, v1MemberLookupDTO{UserID: domaintenancy.FormatUserPublicID(publicID)})
			return
		}
		if err != nil && !errors.Is(err, domaintenancy.ErrNotFound) {
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable47)
			c.Abort()
			return
		}
		contract.RenderError(c, http.StatusNotFound, contract.CodeWorkspaceMemberNotFound, runtimehttp.MsgWorkspaceMemberNotFound)
		c.Abort()
	})
}
