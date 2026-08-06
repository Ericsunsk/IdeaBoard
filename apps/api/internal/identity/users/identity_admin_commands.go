package users

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/database"
)

type IdentityAdminErrorKind string

const (
	IdentityAdminInvalid                IdentityAdminErrorKind = "invalid"
	IdentityAdminConflict               IdentityAdminErrorKind = "conflict"
	IdentityAdminForbidden              IdentityAdminErrorKind = "forbidden"
	IdentityAdminTargetNotFound         IdentityAdminErrorKind = "target_not_found"
	IdentityAdminAuthIdentityNotFound   IdentityAdminErrorKind = "auth_identity_not_found"
	IdentityAdminAuthFailure            IdentityAdminErrorKind = "auth_failure"
	IdentityAdminBusinessFailure        IdentityAdminErrorKind = "business_failure"
	IdentityAdminReconciliationRequired IdentityAdminErrorKind = "reconciliation_required"
	IdentityAdminUnavailable            IdentityAdminErrorKind = "unavailable"
)

type IdentityAdminCommandError struct {
	Kind          IdentityAdminErrorKind
	Message       string
	Cause         error
	RollbackCause error
}

func (e *IdentityAdminCommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "identity admin command failed"
}

func (e *IdentityAdminCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type AuthUserManager interface {
	DeleteUser(ctx context.Context, authUserID string) error
	SetUserRole(ctx context.Context, authUserID string, role string) error
	BanUser(ctx context.Context, authUserID string, reason string, expiresInSeconds int) error
	UnbanUser(ctx context.Context, authUserID string) error
	RevokeUserSessions(ctx context.Context, authUserID string) error
}

type IdentityAdminBanInput struct {
	Reason        string
	ExpiresInDays int
}

type identityAdminUsers interface {
	GetByPublicID(ctx context.Context, publicID string) (*User, error)
	UpdateRole(ctx context.Context, userID int64, role string) error
	UpdateStatus(ctx context.Context, userID int64, status int16) error
	DeleteUser(ctx context.Context, userID int64, authDeleter AuthUserDeleter) error
}

type identityAdminTarget struct {
	user       *User
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

// IdentityAdminCommands is the complete admin identity mutation surface used by transport adapters.
type IdentityAdminCommands interface {
	SetRole(ctx context.Context, actor principal.Principal, targetPublicID string, role string) error
	Ban(ctx context.Context, actor principal.Principal, targetPublicID string, input IdentityAdminBanInput) error
	Unban(ctx context.Context, actor principal.Principal, targetPublicID string) error
	RevokeSessions(ctx context.Context, actor principal.Principal, targetPublicID string) error
	Delete(ctx context.Context, actor principal.Principal, targetPublicID string) error
}

// identityAdminCommands owns the cross-system ordering and recovery policy.
type identityAdminCommands struct {
	users identityAdminUsers
	auth  AuthUserManager
}

func NewIdentityAdminCommands(userService *UserService, authService AuthUserManager) IdentityAdminCommands {
	return newIdentityAdminCommands(userService, authService)
}

func newIdentityAdminCommands(users identityAdminUsers, authService AuthUserManager) *identityAdminCommands {
	return &identityAdminCommands{users: users, auth: authService}
}

func (commands *identityAdminCommands) SetRole(ctx context.Context, actor principal.Principal, targetPublicID string, role string) error {
	nextRole := normalizeAdminRole(role)
	if nextRole == "" {
		return identityAdminError(IdentityAdminInvalid, "role 仅支持 admin 或 user", nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{requireAuth: true})
	if err != nil {
		return err
	}

	currentRole := normalizeAdminRole(target.user.Role)
	if currentRole == "" {
		return identityAdminError(IdentityAdminBusinessFailure, "目标用户的业务侧角色无效", nil)
	}
	if currentRole == nextRole {
		return commands.reconcileAuthRole(ctx, target.authUserID, nextRole)
	}
	if nextRole == principal.RoleAdmin.String() {
		return commands.promote(ctx, target.user.ID, target.authUserID, currentRole)
	}
	return commands.demote(ctx, target.user.ID, target.authUserID)
}

func (commands *identityAdminCommands) Ban(ctx context.Context, actor principal.Principal, targetPublicID string, input IdentityAdminBanInput) error {
	if input.ExpiresInDays < 0 {
		return identityAdminError(IdentityAdminInvalid, "expires_in_days 不能小于 0", nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if target.user.Status == 0 {
		return nil
	}

	const secondsPerDay = 24 * 60 * 60
	if input.ExpiresInDays > int(^uint(0)>>1)/secondsPerDay {
		return identityAdminError(IdentityAdminInvalid, "expires_in_days 超出支持范围", nil)
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
			return commands.users.UpdateStatus(ctx, target.user.ID, 0)
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

func (commands *identityAdminCommands) Unban(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if target.user.Status == 1 {
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
			return commands.users.UpdateStatus(ctx, target.user.ID, 1)
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

func (commands *identityAdminCommands) RevokeSessions(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true, requireAuth: true})
	if err != nil {
		return err
	}
	if err := commands.auth.RevokeUserSessions(ctx, target.authUserID); err != nil {
		return authMutationError(err, "目标用户在身份系统中不存在，无法吊销会话", "身份侧会话吊销失败")
	}
	return nil
}

func (commands *identityAdminCommands) Delete(ctx context.Context, actor principal.Principal, targetPublicID string) error {
	target, err := commands.loadTarget(ctx, actor, targetPublicID, identityAdminTargetPolicy{rejectAdmins: true})
	if err != nil {
		return err
	}
	if err := commands.users.DeleteUser(ctx, target.user.ID, commands.auth); err != nil {
		if errors.Is(err, ErrUserDeletionBlockedByPaymentState) {
			return identityAdminError(IdentityAdminConflict, "存在未结算的 Payment、Refund 或 Dispute，暂不能注销用户", err)
		}
		if errors.Is(err, ErrUserDeletionAuthUnavailable) {
			return identityAdminError(IdentityAdminUnavailable, "认证服务未配置", err)
		}
		return identityAdminError(IdentityAdminBusinessFailure, "用户删除流程已安全冻结，可重试以继续收敛", err)
	}
	return nil
}

func (commands *identityAdminCommands) promote(ctx context.Context, userID int64, authUserID, currentRole string) error {
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

func (commands *identityAdminCommands) demote(ctx context.Context, userID int64, authUserID string) error {
	if err := commands.users.UpdateRole(ctx, userID, principal.RoleUser.String()); err != nil {
		return identityAdminError(IdentityAdminBusinessFailure, "业务侧角色更新失败", err)
	}
	if err := commands.auth.SetUserRole(ctx, authUserID, principal.RoleUser.String()); err != nil {
		if errors.Is(err, autherrors.ErrAuthUserNotFound) {
			return identityAdminError(IdentityAdminAuthIdentityNotFound, "业务侧已取消管理员，但目标用户在身份系统中不存在，请按运维流程核对", err)
		}
		return identityAdminError(IdentityAdminReconciliationRequired, "业务侧已取消管理员，但 Auth 角色同步失败，请重试或按运维流程补偿处理", err)
	}
	return nil
}

func (commands *identityAdminCommands) reconcileAuthRole(ctx context.Context, authUserID, nextRole string) error {
	if err := commands.auth.SetUserRole(ctx, authUserID, nextRole); err != nil {
		if nextRole == principal.RoleUser.String() {
			if errors.Is(err, autherrors.ErrAuthUserNotFound) {
				return identityAdminError(IdentityAdminAuthIdentityNotFound, "业务侧已取消管理员，但目标用户在身份系统中不存在，请按运维流程核对", err)
			}
			return identityAdminError(IdentityAdminReconciliationRequired, "业务侧已取消管理员，但 Auth 角色同步失败，请重试或按运维流程补偿处理", err)
		}
		return authMutationError(err, "目标用户在身份系统中不存在，已拒绝改角色以避免状态漂移", "同步 Auth 管理员角色失败")
	}
	return nil
}

func (commands *identityAdminCommands) loadTarget(ctx context.Context, actor principal.Principal, targetPublicID string, policy identityAdminTargetPolicy) (*identityAdminTarget, error) {
	if !actor.IsValid() {
		return nil, identityAdminError(IdentityAdminInvalid, "管理员身份无效", nil)
	}
	if !actor.IsAdmin() {
		return nil, identityAdminError(IdentityAdminForbidden, "需要管理员权限", nil)
	}
	if commands == nil || commands.users == nil {
		return nil, identityAdminError(IdentityAdminUnavailable, "用户服务未配置", errors.New("identity admin user service not configured"))
	}
	target, err := commands.users.GetByPublicID(ctx, targetPublicID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, identityAdminError(IdentityAdminTargetNotFound, "用户不存在", err)
		}
		return nil, identityAdminError(IdentityAdminBusinessFailure, "读取用户失败", err)
	}
	if target == nil || target.ID <= 0 {
		return nil, identityAdminError(IdentityAdminTargetNotFound, "用户不存在", nil)
	}
	if actor.UserID() == target.ID {
		return nil, identityAdminError(IdentityAdminInvalid, "不能操作当前登录管理员自己", nil)
	}
	if policy.rejectAdmins && normalizeAdminRole(target.Role) == principal.RoleAdmin.String() {
		return nil, identityAdminError(IdentityAdminInvalid, "不能操作管理员账号", nil)
	}
	result := &identityAdminTarget{user: target}
	if policy.requireAuth {
		if commands.auth == nil {
			return nil, identityAdminError(IdentityAdminUnavailable, "认证服务未配置", errors.New("admin auth service client not configured"))
		}
		result.authUserID = strings.TrimSpace(derefString(target.AuthUserID))
		if result.authUserID == "" {
			return nil, identityAdminError(IdentityAdminInvalid, "目标用户缺少 auth_user_id，已拒绝操作以避免状态漂移", nil)
		}
	}
	return result, nil
}

func (commands *identityAdminCommands) applyAuthThenLocal(mutation authFirstMutation) error {
	if err := mutation.applyAuth(); err != nil {
		return mutation.authFailure(err)
	}
	if err := mutation.applyLocal(); err != nil {
		rollbackErr := mutation.compensateAuth()
		if rollbackErr != nil {
			slog.Warn("identity admin mutation compensation failed", "operation", mutation.operation, "user_id", mutation.userID, "auth_user_id", mutation.authUserID, "rollback_err", rollbackErr)
			return reconciliationError(mutation.rollbackFailureMsg, err, rollbackErr)
		}
		return identityAdminError(IdentityAdminBusinessFailure, mutation.localFailureMsg, err)
	}
	return nil
}

func identityAdminError(kind IdentityAdminErrorKind, message string, cause error) *IdentityAdminCommandError {
	return &IdentityAdminCommandError{Kind: kind, Message: message, Cause: cause}
}

func reconciliationError(message string, cause, rollbackCause error) *IdentityAdminCommandError {
	return &IdentityAdminCommandError{
		Kind:          IdentityAdminReconciliationRequired,
		Message:       message,
		Cause:         cause,
		RollbackCause: rollbackCause,
	}
}

func authMutationError(err error, notFoundMessage, failureMessage string) *IdentityAdminCommandError {
	if errors.Is(err, autherrors.ErrAuthUserNotFound) {
		return identityAdminError(IdentityAdminAuthIdentityNotFound, notFoundMessage, err)
	}
	return identityAdminError(IdentityAdminAuthFailure, failureMessage, err)
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
