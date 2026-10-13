package administration

// Durable admin mutations (issue #888): business authorization state is
// the fact source. Every Role/Ban/Unban mutation applies the business
// write (role/status, Wallet alignment, fencing version) and persists one
// Auth sync intent in a SINGLE transaction (Store.Transact joins the
// guard-owned ambient transaction on the control path, so the audit row
// commits atomically with them; otherwise the store owns the
// transaction). External Auth I/O never runs on the request path: it only
// runs after commit, from the stored desired snapshot, driven by the
// authsync sweep. A returned error always means nothing was committed;
// success with SyncPending=true means committed, Auth converging by
// retry.
//
// Fail-closed holds by construction: demotion and ban deny in business
// first, so an Auth outage can delay convergence but never widen access.
// Promotion grants in business first as well (business is the fact
// source); the confirming intent is persisted in the same commit.

import (
	"context"
	"errors"
	"strings"
	"time"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/application/authsync"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"
)

type IdentityAdminCommands struct {
	users ports.UserMutationQueries
	auth  ports.AuthUserManager
	sync  *authsync.Store
}

func NewIdentityAdminCommands(users ports.UserMutationQueries, auth ports.AuthUserManager, sync *authsync.Store) *IdentityAdminCommands {
	return &IdentityAdminCommands{users: users, auth: auth, sync: sync}
}

type identityAdminTarget struct {
	user       *domain.User
	authUserID string
}

type identityAdminTargetPolicy struct {
	rejectAdmins bool
	requireAuth  bool
}

func (commands *IdentityAdminCommands) SetRole(ctx context.Context, actor principal.Principal, targetPublicID string, role string) (ports.IdentityAdminMutationOutcome, error) {
	nextRole := normalizeAdminRole(role)
	if nextRole == "" {
		return ports.IdentityAdminMutationOutcome{}, identityAdminError(ports.IdentityAdminInvalid, msgSetRole, nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{requireAuth: true})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	store, err := commands.syncStore()
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}

	var outcome ports.IdentityAdminMutationOutcome
	err = store.Transact(ctx, func(txCtx context.Context) error {
		if err := store.LockUser(txCtx, target.user.ID); err != nil {
			return err
		}
		fresh, err := store.ReadUserState(txCtx, target.user.ID, true)
		if err != nil {
			return err
		}
		if !fresh.Found {
			return identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, nil)
		}
		if normalizeAdminRole(fresh.Role) == "" {
			return identityAdminError(ports.IdentityAdminBusinessFailure, msgSetRole2, nil)
		}
		if strings.TrimSpace(fresh.AuthUserID) == "" {
			return identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget7, nil)
		}
		if normalizeAdminRole(fresh.Role) == nextRole {
			// Same-state retry: the business fact already holds. A
			// pending intent already covers convergence, so report it
			// without fabricating a duplicate push; otherwise persist
			// one confirming intent to heal any Auth drift.
			// SetUserRole is metadata-free, so re-pushing is safe.
			pending, err := store.HasPendingIntent(txCtx, target.user.ID)
			if err != nil {
				return err
			}
			if pending {
				outcome = ports.IdentityAdminMutationOutcome{SyncPending: true}
				return nil
			}
			intentID, err := store.InsertIntent(txCtx, authsync.InsertIntentParams{
				UserID:        fresh.ID,
				AuthUserID:    fresh.AuthUserID,
				Op:            authsync.OpRole,
				DesiredRole:   nextRole,
				DesiredBanned: fresh.Banned(),
				BaseVersion:   fresh.Version,
			})
			if err != nil {
				return err
			}
			outcome = ports.IdentityAdminMutationOutcome{SyncPending: true, IntentID: intentID}
			return nil
		}
		if err := commands.users.UpdateRole(txCtx, target.user.ID, nextRole); err != nil {
			if database.IsNotFound(err) {
				return identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, err)
			}
			return identityAdminError(ports.IdentityAdminBusinessFailure, msgPromote3, err)
		}
		version, err := store.BumpVersion(txCtx, target.user.ID)
		if err != nil {
			return err
		}
		intentID, err := store.InsertIntent(txCtx, authsync.InsertIntentParams{
			UserID:        target.user.ID,
			AuthUserID:    fresh.AuthUserID,
			Op:            authsync.OpRole,
			DesiredRole:   nextRole,
			DesiredBanned: fresh.Banned(),
			BaseVersion:   version,
		})
		if err != nil {
			return err
		}
		outcome = ports.IdentityAdminMutationOutcome{SyncPending: true, IntentID: intentID}
		return nil
	})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	return outcome, nil
}

func (commands *IdentityAdminCommands) Ban(ctx context.Context, actor principal.Principal, targetPublicID string, input ports.IdentityAdminBanInput) (ports.IdentityAdminMutationOutcome, error) {
	if input.ExpiresInDays < 0 {
		return ports.IdentityAdminMutationOutcome{}, identityAdminError(ports.IdentityAdminInvalid, msgBan, nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	const secondsPerDay = 24 * 60 * 60
	if input.ExpiresInDays > int(^uint(0)>>1)/secondsPerDay {
		return ports.IdentityAdminMutationOutcome{}, identityAdminError(ports.IdentityAdminInvalid, msgBan2, nil)
	}
	expiresInSeconds := input.ExpiresInDays * secondsPerDay
	store, err := commands.syncStore()
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	// The business deadline mirrors the Auth expiry: 0 days means a
	// permanent ban (NULL deadline, never expires).
	var deadline *time.Time
	if input.ExpiresInDays > 0 {
		at := store.Now().Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour)
		deadline = &at
	}

	var outcome ports.IdentityAdminMutationOutcome
	err = store.Transact(ctx, func(txCtx context.Context) error {
		if err := store.LockUser(txCtx, target.user.ID); err != nil {
			return err
		}
		fresh, err := store.ReadUserState(txCtx, target.user.ID, true)
		if err != nil {
			return err
		}
		if !fresh.Found {
			return identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, nil)
		}
		if normalizeAdminRole(fresh.Role) == principal.RoleAdmin.String() {
			return identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget6, nil)
		}
		if strings.TrimSpace(fresh.AuthUserID) == "" {
			return identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget7, nil)
		}
		if fresh.Banned() {
			if deadline == nil && fresh.BanExpiresAt == nil {
				// Permanent re-ban: never fabricate a replacement ban
				// push (the stored expiry would clobber the
				// Auth-side metadata). Report pending honestly instead.
				pending, err := store.HasPendingIntent(txCtx, target.user.ID)
				if err != nil {
					return err
				}
				outcome = ports.IdentityAdminMutationOutcome{SyncPending: pending}
				return nil
			}
			// Finite re-ban refreshes the deadline (extend or shorten):
			// status and Wallet already deny, so only the deadline,
			// version, and a fresh ban intent move, atomically.
			version, err := store.BumpVersionWithDeadline(txCtx, target.user.ID, deadline)
			if err != nil {
				return err
			}
			intentID, err := store.InsertIntent(txCtx, authsync.InsertIntentParams{
				UserID:           target.user.ID,
				AuthUserID:       fresh.AuthUserID,
				Op:               authsync.OpBan,
				DesiredRole:      normalizeAdminRole(fresh.Role),
				DesiredBanned:    true,
				ExpiresInSeconds: expiresInSeconds,
				BaseVersion:      version,
			})
			if err != nil {
				return err
			}
			outcome = ports.IdentityAdminMutationOutcome{SyncPending: true, IntentID: intentID}
			return nil
		}
		// Business denies first (status + Wallet freeze in one unit),
		// fail-closed: an Auth outage delays convergence but never
		// leaves the account usable.
		if err := commands.users.UpdateStatus(txCtx, target.user.ID, domain.StatusBanned); err != nil {
			if database.IsNotFound(err) {
				return identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, err)
			}
			return identityAdminError(ports.IdentityAdminBusinessFailure, msgBan5, err)
		}
		version, err := store.BumpVersionWithDeadline(txCtx, target.user.ID, deadline)
		if err != nil {
			return err
		}
		intentID, err := store.InsertIntent(txCtx, authsync.InsertIntentParams{
			UserID:           target.user.ID,
			AuthUserID:       fresh.AuthUserID,
			Op:               authsync.OpBan,
			DesiredRole:      normalizeAdminRole(fresh.Role),
			DesiredBanned:    true,
			ExpiresInSeconds: expiresInSeconds,
			BaseVersion:      version,
		})
		if err != nil {
			return err
		}
		outcome = ports.IdentityAdminMutationOutcome{SyncPending: true, IntentID: intentID}
		return nil
	})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	return outcome, nil
}

func (commands *IdentityAdminCommands) Unban(ctx context.Context, actor principal.Principal, targetPublicID string) (ports.IdentityAdminMutationOutcome, error) {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	store, err := commands.syncStore()
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}

	var outcome ports.IdentityAdminMutationOutcome
	err = store.Transact(ctx, func(txCtx context.Context) error {
		if err := store.LockUser(txCtx, target.user.ID); err != nil {
			return err
		}
		fresh, err := store.ReadUserState(txCtx, target.user.ID, true)
		if err != nil {
			return err
		}
		if !fresh.Found {
			return identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, nil)
		}
		if normalizeAdminRole(fresh.Role) == principal.RoleAdmin.String() {
			return identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget6, nil)
		}
		if strings.TrimSpace(fresh.AuthUserID) == "" {
			return identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget7, nil)
		}
		if !fresh.Banned() {
			pending, err := store.HasPendingIntent(txCtx, target.user.ID)
			if err != nil {
				return err
			}
			outcome = ports.IdentityAdminMutationOutcome{SyncPending: pending}
			return nil
		}
		// Deletion-owned users stay frozen: the lifecycle seam rejects
		// the unban and nothing commits, so no intent outlives it.
		if err := commands.users.UpdateStatus(txCtx, target.user.ID, domain.StatusActive); err != nil {
			if database.IsNotFound(err) {
				return identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, err)
			}
			return identityAdminError(ports.IdentityAdminBusinessFailure, msgUnban3, err)
		}
		// Manual unban clears any finite deadline: the business fact is
		// active, so no expiry may fire afterwards.
		version, err := store.BumpVersionWithDeadline(txCtx, target.user.ID, nil)
		if err != nil {
			return err
		}
		intentID, err := store.InsertIntent(txCtx, authsync.InsertIntentParams{
			UserID:        target.user.ID,
			AuthUserID:    fresh.AuthUserID,
			Op:            authsync.OpUnban,
			DesiredRole:   normalizeAdminRole(fresh.Role),
			DesiredBanned: false,
			BaseVersion:   version,
		})
		if err != nil {
			return err
		}
		outcome = ports.IdentityAdminMutationOutcome{SyncPending: true, IntentID: intentID}
		return nil
	})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	return outcome, nil
}

func (commands *IdentityAdminCommands) RevokeSessions(ctx context.Context, actor principal.Principal, targetPublicID string) (ports.IdentityAdminMutationOutcome, error) {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	// Sessions carry no business fact: revocation is Auth-only and stays
	// synchronous and retryable, with no intent to persist.
	if err := commands.auth.RevokeUserSessions(ctx, target.authUserID); err != nil {
		return ports.IdentityAdminMutationOutcome{}, authMutationError(err, msgRevokeSessions, msgRevokeSessions2)
	}
	return ports.IdentityAdminMutationOutcome{}, nil
}

func (commands *IdentityAdminCommands) Delete(ctx context.Context, actor principal.Principal, targetPublicID string) (ports.IdentityAdminMutationOutcome, error) {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true})
	if err != nil {
		return ports.IdentityAdminMutationOutcome{}, err
	}
	if err := commands.users.DeleteUser(ctx, target.user.ID, commands.auth); err != nil {
		if errors.Is(err, domain.ErrUserDeletionBlockedByPaymentState) {
			return ports.IdentityAdminMutationOutcome{}, identityAdminError(ports.IdentityAdminConflict, msgDelete, err)
		}
		if errors.Is(err, domain.ErrUserDeletionAuthUnavailable) {
			return ports.IdentityAdminMutationOutcome{}, identityAdminError(ports.IdentityAdminUnavailable, msgDelete2, err)
		}
		return ports.IdentityAdminMutationOutcome{}, identityAdminError(ports.IdentityAdminBusinessFailure, msgDelete3, err)
	}
	return ports.IdentityAdminMutationOutcome{}, nil
}

func (commands *IdentityAdminCommands) syncStore() (*authsync.Store, error) {
	if commands == nil || commands.sync == nil {
		return nil, identityAdminError(ports.IdentityAdminUnavailable, msgSyncStore, nil)
	}
	return commands.sync, nil
}

func (commands *IdentityAdminCommands) loadTarget(ctx context.Context, actor principal.Principal, targetPublicID string, policy identityAdminTargetPolicy) (*identityAdminTarget, error) {
	if !actor.IsValid() {
		return nil, identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget, nil)
	}
	if !actor.IsAdmin() {
		return nil, identityAdminError(ports.IdentityAdminForbidden, msgLoadTarget2, nil)
	}
	target, err := commands.users.GetByPublicID(ctx, targetPublicID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, err)
		}
		return nil, identityAdminError(ports.IdentityAdminBusinessFailure, msgLoadTarget4, err)
	}
	if target == nil || target.ID <= 0 {
		return nil, identityAdminError(ports.IdentityAdminTargetNotFound, msgLoadTarget3, nil)
	}
	if actor.UserID() == target.ID {
		return nil, identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget5, nil)
	}
	if policy.rejectAdmins && normalizeAdminRole(target.Role) == principal.RoleAdmin.String() {
		return nil, identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget6, nil)
	}
	result := &identityAdminTarget{user: target}
	if policy.requireAuth {
		if commands.auth == nil {
			return nil, identityAdminError(ports.IdentityAdminUnavailable, msgDelete2, errors.New("admin auth service client not configured"))
		}
		result.authUserID = strings.TrimSpace(derefString(target.AuthUserID))
		if result.authUserID == "" {
			return nil, identityAdminError(ports.IdentityAdminInvalid, msgLoadTarget7, nil)
		}
	}
	return result, nil
}

func identityAdminError(kind ports.IdentityAdminErrorKind, message string, cause error) *ports.IdentityAdminCommandError {
	return &ports.IdentityAdminCommandError{Kind: kind, Message: message, Cause: cause}
}

func authMutationError(err error, notFoundMessage, failureMessage string) *ports.IdentityAdminCommandError {
	if errors.Is(err, autherrors.ErrAuthUserNotFound) {
		return identityAdminError(ports.IdentityAdminAuthIdentityNotFound, notFoundMessage, err)
	}
	return identityAdminError(ports.IdentityAdminAuthFailure, failureMessage, err)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeAdminRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	if value == principal.RoleAdmin.String() || value == principal.RoleUser.String() {
		return value
	}
	return ""
}

var _ ports.IdentityAdminCommands = (*IdentityAdminCommands)(nil)
