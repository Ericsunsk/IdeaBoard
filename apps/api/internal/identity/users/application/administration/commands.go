package administration

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/platform/database"
)

type IdentityAdminCommands struct {
	users ports.UserMutationQueries
	auth  ports.AuthUserManager
}

func NewIdentityAdminCommands(users ports.UserMutationQueries, auth ports.AuthUserManager) *IdentityAdminCommands {
	return &IdentityAdminCommands{users: users, auth: auth}
}

type identityAdminTarget struct {
	user       *domain.User
	authUserID string
}

type identityAdminTargetPolicy struct {
	rejectAdmins bool
	requireAuth  bool
}

type authFirstMutation struct {
	operation          string
	userID             int64
	authUserID         string
	applyAuth          func() error
	applyLocal         func() error
	compensateAuth     func() error
	authFailure        func(error) error
	localFailureMsg    string
	rollbackFailureMsg string
}

func (commands *IdentityAdminCommands) SetRole(ctx context.Context, actor principal.Principal, targetPublicID string, role string) error {
	nextRole := normalizeAdminRole(role)
	if nextRole == "" {
		return identityAdminError(ports.IdentityAdminInvalid, msgSetRole, nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{requireAuth: true})
	if err != nil {
		return err
	}

	currentRole := normalizeAdminRole(target.user.Role)
	if currentRole == "" {
		return identityAdminError(ports.IdentityAdminBusinessFailure, msgSetRole2, nil)
	}
	if currentRole == nextRole {
		return commands.reconcileAuthRole(ctx, target.authUserID, nextRole)
	}
	if nextRole == principal.RoleAdmin.String() {
		return commands.promote(ctx, target.user.ID, target.authUserID, currentRole)
	}
	return commands.demote(ctx, target.user.ID, target.authUserID)
}

func (commands *IdentityAdminCommands) Ban(ctx context.Context, actor principal.Principal, targetPublicID string, input ports.IdentityAdminBanInput) error {
	if input.ExpiresInDays < 0 {
		return identityAdminError(ports.IdentityAdminInvalid, msgBan, nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if target.user.Status == domain.StatusBanned {
		return nil
	}

	const secondsPerDay = 24 * 60 * 60
	if input.ExpiresInDays > int(^uint(0)>>1)/secondsPerDay {
		return identityAdminError(ports.IdentityAdminInvalid, msgBan2, nil)
	}
	expiresInSeconds := input.ExpiresInDays * secondsPerDay
	return commands.applyAuthThenLocal(authFirstMutation{
		operation:  "ban",
		userID:     target.user.ID,
		authUserID: target.authUserID,
		applyAuth: func() error {
			return commands.auth.BanUser(ctx, target.authUserID, strings.TrimSpace(input.Reason), expiresInSeconds)
		},
		applyLocal: func() error {
			return commands.users.UpdateStatus(ctx, target.user.ID, domain.StatusBanned)
		},
		compensateAuth: func() error {
			return commands.auth.UnbanUser(ctx, target.authUserID)
		},
		authFailure: func(err error) error {
			return authMutationError(err, msgBan3, msgBan4)
		},
		localFailureMsg:    msgBan5,
		rollbackFailureMsg: msgBan6,
	})
}

func (commands *IdentityAdminCommands) Unban(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if target.user.Status == domain.StatusActive {
		return nil
	}

	return commands.applyAuthThenLocal(authFirstMutation{
		operation:  "unban",
		userID:     target.user.ID,
		authUserID: target.authUserID,
		applyAuth: func() error {
			return commands.auth.UnbanUser(ctx, target.authUserID)
		},
		applyLocal: func() error {
			return commands.users.UpdateStatus(ctx, target.user.ID, domain.StatusActive)
		},
		compensateAuth: func() error {
			return commands.auth.BanUser(ctx, target.authUserID, "rollback after local unban failure", 0)
		},
		authFailure: func(err error) error {
			return authMutationError(err, msgUnban, msgUnban2)
		},
		localFailureMsg:    msgUnban3,
		rollbackFailureMsg: msgUnban4,
	})
}

func (commands *IdentityAdminCommands) RevokeSessions(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if err := commands.auth.RevokeUserSessions(ctx, target.authUserID); err != nil {
		return authMutationError(err, msgRevokeSessions, msgRevokeSessions2)
	}
	return nil
}

func (commands *IdentityAdminCommands) Delete(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true})
	if err != nil {
		return err
	}
	if err := commands.users.DeleteUser(ctx, target.user.ID, commands.auth); err != nil {
		if errors.Is(err, domain.ErrUserDeletionBlockedByPaymentState) {
			return identityAdminError(ports.IdentityAdminConflict, msgDelete, err)
		}
		if errors.Is(err, domain.ErrUserDeletionAuthUnavailable) {
			return identityAdminError(ports.IdentityAdminUnavailable, msgDelete2, err)
		}
		return identityAdminError(ports.IdentityAdminBusinessFailure, msgDelete3, err)
	}
	return nil
}

func (commands *IdentityAdminCommands) promote(ctx context.Context, userID int64, authUserID, currentRole string) error {
	return commands.applyAuthThenLocal(authFirstMutation{
		operation:  "role promotion",
		userID:     userID,
		authUserID: authUserID,
		applyAuth: func() error {
			return commands.auth.SetUserRole(ctx, authUserID, principal.RoleAdmin.String())
		},
		applyLocal: func() error {
			return commands.users.UpdateRole(ctx, userID, principal.RoleAdmin.String())
		},
		compensateAuth: func() error {
			return commands.auth.SetUserRole(ctx, authUserID, currentRole)
		},
		authFailure: func(err error) error {
			return authMutationError(err, msgPromote, msgPromote2)
		},
		localFailureMsg:    msgPromote3,
		rollbackFailureMsg: msgPromote4,
	})
}

func (commands *IdentityAdminCommands) demote(ctx context.Context, userID int64, authUserID string) error {
	if err := commands.users.UpdateRole(ctx, userID, principal.RoleUser.String()); err != nil {
		return identityAdminError(ports.IdentityAdminBusinessFailure, msgPromote3, err)
	}
	if err := commands.auth.SetUserRole(ctx, authUserID, principal.RoleUser.String()); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return identityAdminError(ports.IdentityAdminAuthIdentityNotFound, msgDemote, err)
		}
		return identityAdminError(ports.IdentityAdminReconciliationRequired, msgDemote2, err)
	}
	return nil
}

func (commands *IdentityAdminCommands) reconcileAuthRole(ctx context.Context, authUserID, nextRole string) error {
	if err := commands.auth.SetUserRole(ctx, authUserID, nextRole); err != nil {
		if nextRole == principal.RoleUser.String() {
			if errors.Is(err, autherrors.ErrAuthUserNotFound) {
				return identityAdminError(ports.IdentityAdminAuthIdentityNotFound, msgDemote, err)
			}
			return identityAdminError(ports.IdentityAdminReconciliationRequired, msgDemote2, err)
		}
		return authMutationError(err, msgPromote, msgPromote2)
	}
	return nil
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

func (commands *IdentityAdminCommands) applyAuthThenLocal(mutation authFirstMutation) error {
	if err := mutation.applyAuth(); err != nil {
		return mutation.authFailure(err)
	}
	if err := mutation.applyLocal(); err != nil {
		rollbackErr := mutation.compensateAuth()
		if rollbackErr != nil {
			slog.Warn("identity admin mutation compensation failed", "operation", mutation.operation, "user_id", mutation.userID, "auth_user_id", mutation.authUserID, "rollback_err", rollbackErr)
			return reconciliationError(mutation.rollbackFailureMsg, err, rollbackErr)
		}
		return identityAdminError(ports.IdentityAdminBusinessFailure, mutation.localFailureMsg, err)
	}
	return nil
}

func identityAdminError(kind ports.IdentityAdminErrorKind, message string, cause error) *ports.IdentityAdminCommandError {
	return &ports.IdentityAdminCommandError{Kind: kind, Message: message, Cause: cause}
}

func reconciliationError(message string, cause, rollbackCause error) *ports.IdentityAdminCommandError {
	return &ports.IdentityAdminCommandError{
		Kind:          ports.IdentityAdminReconciliationRequired,
		Message:       message,
		Cause:         cause,
		RollbackCause: rollbackCause,
	}
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
