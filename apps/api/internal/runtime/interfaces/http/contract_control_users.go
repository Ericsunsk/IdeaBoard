package httpapi

// Control users family (issue #808): user administration reads and
// high-risk identity mutations behind control:users.
//
// Reads/writes go through the identity seams (AdminUserService reads,
// IdentityAdminCommands mutations, AdminUserImpersonator for
// impersonation); this file owns only the control transport (contract
// envelope, usr_ identifiers, reason + Idempotency-Key via the guard,
// control error codes, audit targets). Legacy /api/admin user routes
// are untouched.

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// controlUser is the Control Plane user projection. Identifiers are the
// external usr_ public ids; internal numeric ids never appear. Display
// fields mirror the legacy admin list item so operator tooling keeps
// working; absent relations default to zero values.
type controlUser struct {
	ID               string    `json:"id"`
	DisplayName      string    `json:"display_name"`
	Email            string    `json:"email,omitempty"`
	Providers        []string  `json:"providers,omitempty"`
	Avatar           string    `json:"avatar,omitempty"`
	Role             string    `json:"role"`
	Status           int16     `json:"status"`
	AvailableCredits int64     `json:"available_credits"`
	LastLoginIP      string    `json:"last_login_ip,omitempty"`
	LastLoginCountry string    `json:"last_login_country,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func newControlUser(user *domain.User) controlUser {
	out := controlUser{}
	if user == nil {
		return out
	}
	out.ID = tenancy.FormatUserPublicID(user.PublicID)
	out.DisplayName = user.DisplayName
	out.Role = user.Role
	out.Status = user.Status
	out.CreatedAt = user.CreatedAt
	out.UpdatedAt = user.UpdatedAt
	return out
}

func newControlUserFromListItem(item *identityports.AdminUserListItem) controlUser {
	out := controlUser{}
	if item == nil {
		return out
	}
	out.ID = tenancy.FormatUserPublicID(item.PublicID)
	out.DisplayName = item.DisplayName
	out.Email = item.Email
	out.Providers = item.Providers
	out.Avatar = item.Avatar
	out.Role = item.Role
	out.Status = item.Status
	out.AvailableCredits = item.AvailableCredits
	out.LastLoginIP = item.LastLoginIP
	out.LastLoginCountry = item.LastLoginCountry
	out.CreatedAt = item.CreatedAt
	out.UpdatedAt = item.UpdatedAt
	return out
}

// controlUserCursor is the opaque list cursor (page-based underneath the
// legacy list seam; never exposed as page numbers on the wire).
type controlUserCursor struct {
	Page int `json:"page"`
}

// parseControlUserID resolves a {user_id} path param (usr_ external id)
// to the UUID string the identity seams consume. Malformed shapes fail
// with 400; unknown users surface as 404 from the seams.
func parseControlUserID(c *gin.Context) (uuid.UUID, bool) {
	raw := strings.TrimSpace(c.Param("user_id"))
	parsed, err := tenancy.ParseUserPublicID(raw)
	if err != nil || parsed == uuid.Nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest9)
		return uuid.Nil, false
	}
	return parsed, true
}

// writeControlIdentityCommandError maps identity admin command failures
// to contract codes. Wire-stable mapping; frontend branches on code.
func writeControlIdentityCommandError(c *gin.Context, err error) {
	var commandErr *identityports.IdentityAdminCommandError
	if err == nil {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable29)
		return
	}
	if !errors.As(err, &commandErr) {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable29)
		return
	}
	message := commandErr.Message
	if message == "" {
		message = msgServiceUnavailable29
	}
	switch commandErr.Kind {
	case identityports.IdentityAdminInvalid:
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, message)
	case identityports.IdentityAdminTargetNotFound, identityports.IdentityAdminAuthIdentityNotFound:
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound5)
	case identityports.IdentityAdminConflict:
		contract.RenderError(c, http.StatusConflict, contract.CodeConflict, message)
	case identityports.IdentityAdminForbidden:
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, message)
	default:
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, message)
	}
}

func (d *ContractControlDeps) listControlUsers(c *gin.Context, _ controlContext) {
	if d == nil || d.Users == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable30)
		return
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := contract.DecodeCursor[controlUserCursor](raw)
		if err != nil || decoded == nil || decoded.Page < 1 {
			contract.RenderError(c, http.StatusBadRequest, contract.CodePaginationCursorInvalid, msgPaginationCursorInvalid)
			return
		}
		page = decoded.Page
	}
	pageSize := 20
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		parsed, err := contract.ParsePageSize(raw)
		if err != nil {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest13)
			return
		}
		pageSize = parsed
	}
	result, err := d.Users.ListUsers(c.Request.Context(), &identityports.UserListRequest{
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		Role:     strings.TrimSpace(c.Query("role")),
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		if validation.Is(err) {
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable31)
		return
	}
	items := make([]controlUser, 0, len(result.List))
	for _, item := range result.List {
		items = append(items, newControlUserFromListItem(item))
	}
	hasMore := int64(page*pageSize) < result.Total
	var nextCursor string
	if hasMore {
		encoded, err := contract.EncodeCursor(controlUserCursor{Page: page + 1})
		if err != nil {
			contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable31)
			return
		}
		nextCursor = encoded
	}
	contract.RenderPaginated(c, items, contract.NewPageMeta(nextCursor, hasMore))
}

func (d *ContractControlDeps) getControlUser(c *gin.Context, _ controlContext) {
	if d == nil || d.Users == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable30)
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	user, err := d.Users.GetByPublicID(c.Request.Context(), id.String())
	if err != nil {
		if database.IsNotFound(err) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound5)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable32)
		return
	}
	if user == nil || user.ID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound5)
		return
	}
	contract.RenderSuccess(c, newControlUser(user))
}

func (d *ContractControlDeps) requireControlIdentityCommands(c *gin.Context) (identityports.IdentityAdminCommands, bool) {
	if d == nil || d.Identity == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable33)
		return nil, false
	}
	return d.Identity, true
}

type controlSetRoleRequest struct {
	Role string `json:"role"`
}

func (d *ContractControlDeps) updateControlUserRole(c *gin.Context, ctx controlContext) {
	commands, ok := d.requireControlIdentityCommands(c)
	if !ok {
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	var req controlSetRoleRequest
	if err := contract.BindStrictJSON(c, &req); err != nil || strings.TrimSpace(req.Role) == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	outcome, err := commands.SetRole(c.Request.Context(), ctx.Operator, id.String(), strings.TrimSpace(req.Role))
	if err != nil {
		writeControlIdentityCommandError(c, err)
		return
	}
	contract.RenderSuccess(c, struct {
		Updated         bool `json:"updated"`
		AuthSyncPending bool `json:"auth_sync_pending"`
	}{Updated: true, AuthSyncPending: outcome.SyncPending})
}

type controlBanUserRequest struct {
	Reason        string `json:"reason"`
	ExpiresInDays int    `json:"expires_in_days"`
}

func (d *ContractControlDeps) banControlUser(c *gin.Context, ctx controlContext) {
	commands, ok := d.requireControlIdentityCommands(c)
	if !ok {
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	var req controlBanUserRequest
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest4)
		return
	}
	if req.ExpiresInDays < 0 {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest25)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = ctx.Reason
	}
	outcome, err := commands.Ban(c.Request.Context(), ctx.Operator, id.String(), identityports.IdentityAdminBanInput{
		Reason:        reason,
		ExpiresInDays: req.ExpiresInDays,
	})
	if err != nil {
		writeControlIdentityCommandError(c, err)
		return
	}
	contract.RenderSuccess(c, struct {
		Banned          bool `json:"banned"`
		AuthSyncPending bool `json:"auth_sync_pending"`
	}{Banned: true, AuthSyncPending: outcome.SyncPending})
}

func (d *ContractControlDeps) unbanControlUser(c *gin.Context, ctx controlContext) {
	commands, ok := d.requireControlIdentityCommands(c)
	if !ok {
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	outcome, err := commands.Unban(c.Request.Context(), ctx.Operator, id.String())
	if err != nil {
		writeControlIdentityCommandError(c, err)
		return
	}
	contract.RenderSuccess(c, struct {
		Unbanned        bool `json:"unbanned"`
		AuthSyncPending bool `json:"auth_sync_pending"`
	}{Unbanned: true, AuthSyncPending: outcome.SyncPending})
}

func (d *ContractControlDeps) revokeControlUserSessions(c *gin.Context, ctx controlContext) {
	commands, ok := d.requireControlIdentityCommands(c)
	if !ok {
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	if _, err := commands.RevokeSessions(c.Request.Context(), ctx.Operator, id.String()); err != nil {
		writeControlIdentityCommandError(c, err)
		return
	}
	contract.RenderSuccess(c, struct {
		Revoked bool `json:"revoked"`
	}{Revoked: true})
}

func (d *ContractControlDeps) deleteControlUser(c *gin.Context, ctx controlContext) {
	commands, ok := d.requireControlIdentityCommands(c)
	if !ok {
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	if _, err := commands.Delete(c.Request.Context(), ctx.Operator, id.String()); err != nil {
		writeControlIdentityCommandError(c, err)
		return
	}
	contract.RenderSuccess(c, struct {
		Deleted bool `json:"deleted"`
	}{Deleted: true})
}

func (d *ContractControlDeps) impersonateControlUser(c *gin.Context, ctx controlContext) {
	if d == nil || d.Users == nil || d.Impersonator == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailable34)
		return
	}
	id, ok := parseControlUserID(c)
	if !ok {
		return
	}
	target, err := d.Users.GetByPublicID(c.Request.Context(), id.String())
	if err != nil {
		if database.IsNotFound(err) {
			contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound5)
			return
		}
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable32)
		return
	}
	if target == nil || target.ID <= 0 {
		contract.RenderError(c, http.StatusNotFound, contract.CodeControlNotFound, msgControlNotFound5)
		return
	}
	if ctx.Operator.UserID() == target.ID {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest26)
		return
	}
	if strings.ToLower(strings.TrimSpace(target.Role)) == domain.RoleAdmin {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest27)
		return
	}
	authUserID := ""
	if target.AuthUserID != nil {
		authUserID = strings.TrimSpace(*target.AuthUserID)
	}
	if authUserID == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest28)
		return
	}
	cookies, err := d.Impersonator.ImpersonateUser(c.Request.Context(), c.GetHeader("Cookie"), authUserID)
	if err != nil {
		writeControlImpersonationError(c, err)
		return
	}
	for _, cookie := range cookies {
		c.Writer.Header().Add("Set-Cookie", cookie)
	}
	contract.RenderSuccess(c, struct {
		Impersonated bool `json:"impersonated"`
	}{Impersonated: true})
}

type controlImpersonationStatusError interface {
	error
	StatusCode() int
}

func writeControlImpersonationError(c *gin.Context, err error) {
	var statusErr controlImpersonationStatusError
	if err == nil || !errors.As(err, &statusErr) {
		contract.RenderError(c, http.StatusBadGateway, contract.CodeServiceUnavailable, msgServiceUnavailable35)
		return
	}
	switch statusErr.StatusCode() {
	case http.StatusUnauthorized, http.StatusForbidden:
		contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, msgForbidden3)
	case http.StatusBadRequest, http.StatusNotFound:
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, msgBadRequest29)
	default:
		contract.RenderError(c, http.StatusBadGateway, contract.CodeServiceUnavailable, msgServiceUnavailable35)
	}
}
