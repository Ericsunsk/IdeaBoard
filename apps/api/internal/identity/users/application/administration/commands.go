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
		return identityAdminError(ports.IdentityAdminInvalid, "role 仅支持 admin 或 user", nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{requireAuth: true})
	if err != nil {
		return err
	}

	currentRole := normalizeAdminRole(target.user.Role)
	if currentRole == "" {
		return identityAdminError(ports.IdentityAdminBusinessFailure, "目标用户的业务侧角色无效", nil)
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
		return identityAdminError(ports.IdentityAdminInvalid, "expires_in_days 不能小于 0", nil)
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
		return identityAdminError(ports.IdentityAdminInvalid, "expires_in_days 超出支持范围", nil)
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
			return authMutationError(err, "目标用户在身份系统中不存在，已拒绝封禁以避免状态漂移", "身份侧封禁失败")
		},
		localFailureMsg:    "业务侧封禁失败",
		rollbackFailureMsg: "业务侧封禁失败，且身份侧回滚失败，请按运维流程补偿处理",
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
			return authMutationError(err, "目标用户在身份系统中不存在，已拒绝解封以避免状态漂移", "身份侧解封失败")
		},
		localFailureMsg:    "业务侧解封失败",
		rollbackFailureMsg: "业务侧解封失败，且身份侧回滚失败，请按运维流程补偿处理",
	})
}

func (commands *IdentityAdminCommands) RevokeSessions(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if err := commands.auth.RevokeUserSessions(ctx, target.authUserID); err != nil {
		return authMutationError(err, "目标用户在身份系统中不存在，无法吊销会话", "身份侧会话吊销失败")
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
			return identityAdminError(ports.IdentityAdminConflict, "存在未结算的 Payment、Refund 或 Dispute，暂不能注销用户", err)
		}
		if errors.Is(err, domain.ErrUserDeletionAuthUnavailable) {
			return identityAdminError(ports.IdentityAdminUnavailable, "认证服务未配置", err)
		}
		return identityAdminError(ports.IdentityAdminBusinessFailure, "用户删除流程已安全冻结，可重试以继续收敛", err)
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
			return authMutationError(err, "目标用户在身份系统中不存在，已拒绝改角色以避免状态漂移", "同步 Auth 管理员角色失败")
		},
		localFailureMsg:    "业务侧角色更新失败",
		rollbackFailureMsg: "业务侧角色更新失败，且 Auth 角色回滚失败，请按运维流程补偿处理",
	})
}

func (commands *IdentityAdminCommands) demote(ctx context.Context, userID int64, authUserID string) error {
	if err := commands.users.UpdateRole(ctx, userID, principal.RoleUser.String()); err != nil {
		return identityAdminError(ports.IdentityAdminBusinessFailure, "业务侧角色更新失败", err)
	}
	if err := commands.auth.SetUserRole(ctx, authUserID, principal.RoleUser.String()); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return identityAdminError(ports.IdentityAdminAuthIdentityNotFound, "业务侧已取消管理员，但目标用户在身份系统中不存在，请按运维流程核对", err)
		}
		return identityAdminError(ports.IdentityAdminReconciliationRequired, "业务侧已取消管理员，但 Auth 角色同步失败，请重试或按运维流程补偿处理", err)
	}
	return nil
}

func (commands *IdentityAdminCommands) reconcileAuthRole(ctx context.Context, authUserID, nextRole string) error {
	if err := commands.auth.SetUserRole(ctx, authUserID, nextRole); err != nil {
		if nextRole == principal.RoleUser.String() {
			if errors.Is(err, autherrors.ErrAuthUserNotFound) {
				return identityAdminError(ports.IdentityAdminAuthIdentityNotFound, "业务侧已取消管理员，但目标用户在身份系统中不存在，请按运维流程核对", err)
			}
			return identityAdminError(ports.IdentityAdminReconciliationRequired, "业务侧已取消管理员，但 Auth 角色同步失败，请重试或按运维流程补偿处理", err)
		}
		return authMutationError(err, "目标用户在身份系统中不存在，已拒绝改角色以避免状态漂移", "同步 Auth 管理员角色失败")
	}
	return nil
}

func (commands *IdentityAdminCommands) loadTarget(ctx context.Context, actor principal.Principal, targetPublicID string, policy identityAdminTargetPolicy) (*identityAdminTarget, error) {
	if !actor.IsValid() {
		return nil, identityAdminError(ports.IdentityAdminInvalid, "管理员身份无效", nil)
	}
	if !actor.IsAdmin() {
		return nil, identityAdminError(ports.IdentityAdminForbidden, "需要管理员权限", nil)
	}
	target, err := commands.users.GetByPublicID(ctx, targetPublicID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, identityAdminError(ports.IdentityAdminTargetNotFound, "用户不存在", err)
		}
		return nil, identityAdminError(ports.IdentityAdminBusinessFailure, "读取用户失败", err)
	}
	if target == nil || target.ID <= 0 {
		return nil, identityAdminError(ports.IdentityAdminTargetNotFound, "用户不存在", nil)
	}
	if actor.UserID() == target.ID {
		return nil, identityAdminError(ports.IdentityAdminInvalid, "不能操作当前登录管理员自己", nil)
	}
	if policy.rejectAdmins && normalizeAdminRole(target.Role) == principal.RoleAdmin.String() {
		return nil, identityAdminError(ports.IdentityAdminInvalid, "不能操作管理员账号", nil)
	}
	result := &identityAdminTarget{user: target}
	if policy.requireAuth {
		if commands.auth == nil {
			return nil, identityAdminError(ports.IdentityAdminUnavailable, "认证服务未配置", errors.New("admin auth service client not configured"))
		}
		result.authUserID = strings.TrimSpace(derefString(target.AuthUserID))
		if result.authUserID == "" {
			return nil, identityAdminError(ports.IdentityAdminInvalid, "目标用户缺少 auth_user_id，已拒绝操作以避免状态漂移", nil)
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
