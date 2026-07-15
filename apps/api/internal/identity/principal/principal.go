package principal

import (
	"errors"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

var ErrInvalid = errors.New("invalid authenticated principal")

const principalContextKey = "ideaboard.auth.principal"

// Principal is the complete, validated identity of an authenticated request.
type Principal struct {
	userID     int64
	authUserID string
	username   string
	role       Role
}

func New(userID int64, authUserID, username, role string) (Principal, error) {
	authUserID = strings.TrimSpace(authUserID)
	username = strings.TrimSpace(username)
	parsedRole, ok := parseRole(role)
	if userID <= 0 || authUserID == "" || username == "" || !ok {
		return Principal{}, ErrInvalid
	}
	return Principal{
		userID:     userID,
		authUserID: authUserID,
		username:   username,
		role:       parsedRole,
	}, nil
}

func (p Principal) UserID() int64      { return p.userID }
func (p Principal) AuthUserID() string { return p.authUserID }
func (p Principal) Username() string   { return p.username }
func (p Principal) Role() Role         { return p.role }
func (p Principal) IsAdmin() bool      { return p.role == RoleAdmin }
func (p Principal) IsValid() bool      { return isValidPrincipal(p) }
func (role Role) IsValid() bool        { return role == RoleUser || role == RoleAdmin }
func (role Role) String() string       { return string(role) }
func parseRole(value string) (Role, bool) {
	role := Role(value)
	return role, role.IsValid()
}

func isValidPrincipal(principal Principal) bool {
	return principal.userID > 0 &&
		principal.authUserID != "" && principal.authUserID == strings.TrimSpace(principal.authUserID) &&
		principal.username != "" && principal.username == strings.TrimSpace(principal.username) &&
		principal.role.IsValid()
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
