package http

// Shared Public Data Plane kernel seams (split by plane: v1/ + control/
// both import this package, never each other).
//
// V1UserDirectory + NewUsersRepositoryDirectory + usersRepositoryDirectory
// moved here from v1/routes.go so the Control Plane's
// ContractControlDeps.UserDirectory does not force a control -> v1
// import edge. Planes stay independent; the kernel owns the shared type.
// V1AuthMiddleware moved here from v1/routes.go for the same reason:
// both RegisterContractV1Routes (v1) and RegisterContractControlRoutes
// (control) mount it, so it must live in the kernel and be exported.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/principal"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// V1UserDirectory translates between internal numeric user ids and the
// external usr_<hex> identifiers without leaking database ids to the API.
// Production adapts the identity UserRepository; tests use a static map.
// GetUserPublicIDs resolves many ids in ONE repository round trip,
// replacing per-member GetUserPublicID N+1 loops in member/quota/billing
// list projections; unknown ids are omitted by the directory and required
// identity projections reject missing ids.
type V1UserDirectory interface {
	ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error)
	GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error)
	GetUserPublicIDs(ctx context.Context, userIDs []int64) (map[int64]uuid.UUID, error)
}

// usersRepositoryDirectory adapts identityports.UserRepository to
// V1UserDirectory (production wiring).
type usersRepositoryDirectory struct {
	repo identityports.UserRepository
}

// NewUsersRepositoryDirectory exposes the production V1UserDirectory
// adapter for composition wiring outside this package.
func NewUsersRepositoryDirectory(repo identityports.UserRepository) V1UserDirectory {
	return usersRepositoryDirectory{repo: repo}
}

// V1UserProfileDirectory owns the persisted user profile projection,
// independent of session principals.
type V1UserProfileDirectory interface {
	UpdateUserAvatar(ctx context.Context, userID int64, avatar string) error
	UpdateDisplayNameByID(ctx context.Context, userID int64, displayName string) error
	GetUserByID(ctx context.Context, userID int64) (*identitydomain.User, error)
	GetUserByStorageUID(ctx context.Context, storageUID string) (*identitydomain.User, error)
}

func NewV1UserProfileDirectory(repo identityports.UserRepository) V1UserProfileDirectory {
	return usersRepositoryDirectory{repo: repo}
}

func (d usersRepositoryDirectory) ResolveUserID(ctx context.Context, publicID uuid.UUID) (int64, error) {
	if d.repo == nil || publicID == uuid.Nil {
		return 0, tenancy.ErrNotFound
	}
	user, err := d.repo.GetByPublicID(ctx, publicID.String())
	if err != nil || user == nil {
		return 0, tenancy.ErrNotFound
	}
	return user.ID, nil
}

func (d usersRepositoryDirectory) GetUserPublicID(ctx context.Context, userID int64) (uuid.UUID, error) {
	if d.repo == nil || userID <= 0 {
		return uuid.Nil, tenancy.ErrNotFound
	}
	user, err := d.repo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return uuid.Nil, tenancy.ErrNotFound
	}
	return user.PublicID, nil
}

// GetUserPublicIDs resolves many ids in ONE GetByIDs round trip. Unknown
// ids are omitted; required identity projections reject missing ids.
// A nil repository resolves to no facts (handlers guard Users==nil).
func (d usersRepositoryDirectory) GetUserPublicIDs(ctx context.Context, userIDs []int64) (map[int64]uuid.UUID, error) {
	out := make(map[int64]uuid.UUID, len(userIDs))
	if d.repo == nil {
		return out, nil
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
		return out, nil
	}
	usersByID, err := d.repo.GetByIDs(ctx, deduped)
	if err != nil {
		return nil, err
	}
	for id, user := range usersByID {
		if user == nil || user.PublicID == uuid.Nil {
			continue
		}
		out[id] = user.PublicID
	}
	return out, nil
}

func (d usersRepositoryDirectory) UpdateUserAvatar(ctx context.Context, userID int64, avatar string) error {
	if d.repo == nil || userID <= 0 {
		return tenancy.ErrNotFound
	}
	return d.repo.UpdateAvatarByID(ctx, userID, avatar)
}

func (d usersRepositoryDirectory) UpdateDisplayNameByID(ctx context.Context, userID int64, displayName string) error {
	if d.repo == nil || userID <= 0 {
		return tenancy.ErrNotFound
	}
	return d.repo.UpdateDisplayNameByID(ctx, userID, displayName)
}

func (d usersRepositoryDirectory) GetUserByID(ctx context.Context, userID int64) (*identitydomain.User, error) {
	if d.repo == nil || userID <= 0 {
		return nil, tenancy.ErrNotFound
	}
	return d.repo.GetByID(ctx, userID)
}

func (d usersRepositoryDirectory) GetUserByStorageUID(ctx context.Context, storageUID string) (*identitydomain.User, error) {
	if d.repo == nil || !identitydomain.IsValidStorageUID(storageUID) {
		return nil, tenancy.ErrNotFound
	}
	user, err := d.repo.GetByStorageUID(ctx, strings.TrimSpace(storageUID))
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, tenancy.ErrNotFound
	}
	return user, nil
}

// V1AuthMiddleware keeps the cookie-session path working in production by
// delegating to the Better Auth session middleware when it is configured;
// in tests (no auth service) it accepts an injected principal instead.
// Either way ResolvePrincipal() runs next to resolve the TenantScope.
func V1AuthMiddleware(authDeps AuthDeps) gin.HandlerFunc {
	if authDeps.AuthServiceClient != nil && authDeps.UserRepo != nil {
		return auth.AuthMiddleware(authDeps.AuthServiceClient, authDeps.UserRepo, authDeps.UserLifecycle)
	}
	return func(c *gin.Context) {
		if _, ok := principal.From(c); !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, MsgUnauthorized)
			c.Abort()
			return
		}
		c.Next()
	}
}

// V1PublicAvatarURL renders the canonical public route for one storage
// UID (sizeless; callers append ?size= for a display variant).
// (Moved to the kernel during the plane split: v1 avatar/member
// surfaces and the control user read share it.)
func V1PublicAvatarURL(storageUID string) string {
	return ContractV1GroupPath + "/public/users/" + strings.TrimSpace(storageUID) + "/avatar"
}

// V1PublicAvatarURLForRow projects a stored users.avatar value to its
// public route URL. Empty keys (no custom avatar) project to "" so
// callers keep the omitted-avatar shape. DiceBear seed URLs use the same
// public route as stored objects; other external URLs pass through untouched.
func V1PublicAvatarURLForRow(storageUID, avatarKey string) string {
	key := strings.TrimSpace(avatarKey)
	if key == "" {
		return ""
	}
	if !imgproxy.IsDicebearAvatarURL(key) && (strings.HasPrefix(key, "/") || strings.Contains(key, "://")) {
		return key
	}
	if strings.TrimSpace(storageUID) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return V1PublicAvatarURL(storageUID) + "?v=" + hex.EncodeToString(sum[:8])
}
