package principal

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// SubjectType classifies the credential kind behind a Principal (issue
// #801: Workspace, Organization, and Principal lifecycle).
//
// Human and integration subjects (human_session, api_key, oauth_app,
// service_account) carry Public Data Plane capabilities only. Control
// Plane capabilities (control:*) are reserved for internal_worker and
// are rejected for every other subject type at construction time, so a
// public capability can never imply a control capability.
type SubjectType string

const (
	SubjectHumanSession   SubjectType = "human_session"
	SubjectAPIKey         SubjectType = "api_key"
	SubjectOAuthApp       SubjectType = "oauth_app"
	SubjectServiceAccount SubjectType = "service_account"
	SubjectInternalWorker SubjectType = "internal_worker"
)

// Public Data Plane capability names (issue #801). Control capabilities
// always carry the control: prefix and are never granted to non-worker
// subjects (see validateCapabilities).
const (
	CapabilityWorkspaceRead  = "workspace:read"
	CapabilityWorkspaceWrite = "workspace:write"
	CapabilityBoardRead      = "board:read"
	CapabilityControlPrefix  = "control:"
)

// IsValid reports whether the subject type is a known credential kind.
func (s SubjectType) IsValid() bool {
	switch s {
	case SubjectHumanSession, SubjectAPIKey, SubjectOAuthApp, SubjectServiceAccount, SubjectInternalWorker:
		return true
	default:
		return false
	}
}

// IsControlCapability reports whether a capability name belongs to the
// Control Plane. Public capabilities must never imply these.
func IsControlCapability(capability string) bool {
	trimmed := strings.TrimSpace(capability)
	if trimmed == "*" {
		return true
	}
	return trimmed == "control" || trimmed == "control:*" || strings.HasPrefix(trimmed, CapabilityControlPrefix)
}

// validateCapabilities enforces the anti-implication rule: only
// internal_worker subjects may hold control:* capabilities. Empty
// capability names are always rejected.
func validateCapabilities(subjectType SubjectType, capabilities []string) error {
	for _, capability := range capabilities {
		trimmed := strings.TrimSpace(capability)
		if trimmed == "" {
			return ErrInvalid
		}
		if IsControlCapability(trimmed) && subjectType != SubjectInternalWorker {
			return ErrInvalid
		}
	}
	return nil
}

var ErrInvalid = errors.New("invalid authenticated principal")

const principalContextKey = "ideaboard.auth.principal"

// Principal is the complete, validated identity of an authenticated request.
//
// Legacy accessors (UserID, Role, IsAdmin, Require, RequireAdmin) are
// preserved for the /api/user and /api/admin surfaces and are DEPRECATED
// for new /api/v1 code, which must use Subject/SubjectType/TenantScope/
// Capabilities and RequireCapability instead. Removal is #809.
type Principal struct {
	userID      int64
	publicID    uuid.UUID
	authUserID  string
	displayName string
	role        Role
	// Subject is the stable external subject identifier (e.g. usr_<hex>).
	subject string
	// SubjectType classifies the credential kind (human_session by default).
	subjectType SubjectType
	// TenantScope carries the explicit workspace context (ws_ public_id)
	// resolved from the request path, or empty when unscoped.
	tenantScope string
	// Capabilities is the granted capability set. Public capabilities
	// never imply control:* (enforced at construction).
	capabilities map[string]struct{}
	// AuthMethod names how the subject authenticated (e.g. session).
	authMethod string
}

func New(userID int64, publicID uuid.UUID, authUserID, displayName, role string) (Principal, error) {
	authUserID = strings.TrimSpace(authUserID)
	displayName = strings.TrimSpace(displayName)
	parsedRole, ok := parseRole(role)
	if userID <= 0 || publicID == uuid.Nil || authUserID == "" || displayName == "" || !ok {
		return Principal{}, ErrInvalid
	}
	return Principal{
		userID:       userID,
		publicID:     publicID,
		authUserID:   authUserID,
		displayName:  displayName,
		role:         parsedRole,
		subject:      authUserID,
		subjectType:  SubjectHumanSession,
		capabilities: map[string]struct{}{},
		authMethod:   "session",
	}, nil
}

// NewExtended builds a Principal with explicit v2 identity fields (issue
// #801). Capabilities are validated so public subjects can never imply
// control:* capabilities; tenantScope must be empty or carry ws_ shape
// (validated lightly here, authoritatively by platform/contract at the
// middleware seam).
func NewExtended(userID int64, publicID uuid.UUID, authUserID, displayName, role string, subjectType SubjectType, authMethod string, tenantScope string, capabilities []string) (Principal, error) {
	authUserID = strings.TrimSpace(authUserID)
	displayName = strings.TrimSpace(displayName)
	authMethod = strings.TrimSpace(authMethod)
	tenantScope = strings.TrimSpace(tenantScope)
	parsedRole, ok := parseRole(role)
	if userID <= 0 || publicID == uuid.Nil || authUserID == "" || displayName == "" || !ok {
		return Principal{}, ErrInvalid
	}
	if !subjectType.IsValid() {
		return Principal{}, ErrInvalid
	}
	if tenantScope != "" && !isWorkspaceScopeShape(tenantScope) {
		return Principal{}, ErrInvalid
	}
	if err := validateCapabilities(subjectType, capabilities); err != nil {
		return Principal{}, err
	}
	capSet := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		capSet[strings.TrimSpace(capability)] = struct{}{}
	}
	subject := authUserID
	if authMethod == "" {
		authMethod = "session"
	}
	return Principal{
		userID:       userID,
		publicID:     publicID,
		authUserID:   authUserID,
		displayName:  displayName,
		role:         parsedRole,
		subject:      subject,
		subjectType:  subjectType,
		tenantScope:  tenantScope,
		capabilities: capSet,
		authMethod:   authMethod,
	}, nil
}

// isWorkspaceScopeShape lightly validates ws_ scope shape without importing
// platform/contract (authoritative validation lives at the middleware seam
// via contract.IsWorkspaceID).
func isWorkspaceScopeShape(scope string) bool {
	const prefix = "ws_"
	if !strings.HasPrefix(scope, prefix) || len(scope) <= len(prefix) {
		return false
	}
	for _, r := range scope[len(prefix):] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func (p Principal) UserID() int64       { return p.userID }
func (p Principal) PublicID() uuid.UUID { return p.publicID }
func (p Principal) AuthUserID() string  { return p.authUserID }
func (p Principal) DisplayName() string { return p.displayName }
func (p Principal) Role() Role          { return p.role }
func (p Principal) IsAdmin() bool       { return p.role == RoleAdmin }

// Subject returns the stable external subject identifier.
func (p Principal) Subject() string { return p.subject }

// SubjectType classifies the credential kind behind this principal.
func (p Principal) SubjectType() SubjectType { return p.subjectType }

// TenantScope carries the explicit workspace context (ws_ public_id) or
// empty when the request is unscoped.
func (p Principal) TenantScope() string { return p.tenantScope }

// AuthMethod names how the subject authenticated (e.g. session).
func (p Principal) AuthMethod() string { return p.authMethod }

// Capabilities returns a copy of the granted capability set.
func (p Principal) Capabilities() map[string]struct{} {
	out := make(map[string]struct{}, len(p.capabilities))
	for capability := range p.capabilities {
		out[capability] = struct{}{}
	}
	return out
}

// HasCapability reports whether the principal holds a capability. Control
// capabilities are only ever present on internal_worker subjects (see
// NewExtended), so a public capability check can never succeed for them.
func (p Principal) HasCapability(capability string) bool {
	if p.capabilities == nil {
		return false
	}
	_, ok := p.capabilities[strings.TrimSpace(capability)]
	return ok
}

// WithTenantScope returns a copy scoped to the given workspace. An empty
// scope clears the tenant context. Malformed scopes fail closed with
// ErrInvalid; callers that must never fail (middleware resolving an
// optional path param) should validate shape first and leave the scope
// empty instead of calling this with untrusted input.
func (p Principal) WithTenantScope(scope string) (Principal, error) {
	trimmed := strings.TrimSpace(scope)
	if trimmed != "" && !isWorkspaceScopeShape(trimmed) {
		return Principal{}, ErrInvalid
	}
	copy := p
	copy.tenantScope = trimmed
	if !copy.IsValid() {
		return Principal{}, ErrInvalid
	}
	return copy, nil
}

// RequireCapability fails closed when the principal lacks a capability:
// it renders 403 and aborts the context. Visibility-aware callers (hidden
// resources converge to 404 per Baseline 4) must branch on visibility
// first via the httpapi RequireWorkspaceCapability helper instead.
func (p Principal) RequireCapability(c *gin.Context, capability string) bool {
	if p.HasCapability(capability) {
		return true
	}
	if c != nil {
		response.Error(c, http.StatusForbidden, "权限不足")
		c.Abort()
	}
	return false
}
func (p Principal) IsValid() bool { return isValidPrincipal(p) }
func (role Role) IsValid() bool   { return role == RoleUser || role == RoleAdmin }
func (role Role) String() string  { return string(role) }
func parseRole(value string) (Role, bool) {
	role := Role(value)
	return role, role.IsValid()
}

func isValidPrincipal(principal Principal) bool {
	if !(principal.userID > 0 &&
		principal.publicID != uuid.Nil &&
		principal.authUserID != "" && principal.authUserID == strings.TrimSpace(principal.authUserID) &&
		principal.displayName != "" && principal.displayName == strings.TrimSpace(principal.displayName) &&
		principal.role.IsValid()) {
		return false
	}
	if principal.subjectType != "" && !principal.subjectType.IsValid() {
		return false
	}
	if principal.tenantScope != "" && !isWorkspaceScopeShape(principal.tenantScope) {
		return false
	}
	for capability := range principal.capabilities {
		trimmed := strings.TrimSpace(capability)
		if trimmed == "" || trimmed != capability {
			return false
		}
		if IsControlCapability(trimmed) && principal.subjectType != "" && principal.subjectType != SubjectInternalWorker {
			return false
		}
	}
	return true
}

func Set(c *gin.Context, principal Principal) error {
	if c == nil {
		return ErrInvalid
	}
	// Poison any previous value before validating so a failed overwrite cannot retain authority.
	c.Set(principalContextKey, Principal{})
	if !principal.IsValid() {
		return ErrInvalid
	}
	c.Set(principalContextKey, principal)
	return nil
}

func From(c *gin.Context) (Principal, bool) {
	if c == nil {
		return Principal{}, false
	}
	value, exists := c.Get(principalContextKey)
	principal, ok := value.(Principal)
	if !exists || !ok || !principal.IsValid() {
		return Principal{}, false
	}
	return principal, true
}

func Require(c *gin.Context) (Principal, bool) {
	principal, ok := From(c)
	if ok {
		return principal, true
	}
	if c != nil {
		response.Error(c, http.StatusUnauthorized, "未登录")
		c.Abort()
	}
	return Principal{}, false
}

func RequireAdmin(c *gin.Context) (Principal, bool) {
	identityPrincipal, ok := Require(c)
	if !ok {
		return Principal{}, false
	}
	if !identityPrincipal.IsAdmin() {
		response.Error(c, http.StatusForbidden, "需要管理员权限")
		c.Abort()
		return Principal{}, false
	}
	return identityPrincipal, true
}
