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
	GetByID(ctx context.Context, id int64) (*User, error)
	UpdateRole(ctx context.Context, userID int64, role string) error
	UpdateStatus(ctx context.Context, userID int64, status int16) error
	DeleteUser(ctx context.Context, userID int64, authDeleter AuthUserDeleter) error
}

// IdentityAdminCommands is the complete admin identity mutation surface used by transport adapters.
type IdentityAdminCommands interface {
	SetRole(ctx context.Context, actor principal.Principal, targetUserID int64, role string) error
	Ban(ctx context.Context, actor principal.Principal, targetUserID int64, input IdentityAdminBanInput) error
	Unban(ctx context.Context, actor principal.Principal, targetUserID int64) error
	RevokeSessions(ctx context.Context, actor principal.Principal, targetUserID int64) error
	Delete(ctx context.Context, actor principal.Principal, targetUserID int64) error
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

func (commands *identityAdminCommands) SetRole(ctx context.Context, actor principal.Principal, targetUserID int64, role string) error {
	nextRole := normalizeAdminRole(role)
	if nextRole == "" {
		return identityAdminError(IdentityAdminInvalid, "role 仅支持 admin 或 user", nil)
	}
	target, err := commands.loadTarget(ctx, actor, targetUserID, false)
	if err != nil {
		return err
	}
	authUserID, err := commands.requireAuthIdentity(target, "目标用户缺少 auth_user_id，已拒绝改角色以避免状态漂移")
	if err != nil {
		return err
	}
	if commands.auth == nil {
		return identityAdminError(IdentityAdminUnavailable, "认证服务未配置", errors.New("admin auth service client not configured"))
	}

	currentRole := normalizeAdminRole(target.Role)
	if currentRole == "" {
		return identityAdminError(IdentityAdminBusinessFailure, "目标用户的业务侧角色无效", nil)
	}
	if currentRole == nextRole {
		return commands.reconcileAuthRole(ctx, authUserID, nextRole)
	}
	if nextRole == principal.RoleAdmin.String() {
		return commands.promote(ctx, target.ID, authUserID, currentRole)
	}
	return commands.demote(ctx, target.ID, authUserID)
}

func (commands *identityAdminCommands) Ban(ctx context.Context, actor principal.Principal, targetUserID int64, input IdentityAdminBanInput) error {
	if input.ExpiresInDays < 0 {
		return identityAdminError(IdentityAdminInvalid, "expires_in_days 不能小于 0", nil)
	}
	target, authUserID, err := commands.loadManagedAuthUser(ctx, actor, targetUserID)
	if err != nil {
		return err
	}
	if target.Status == 0 {
		return nil
	}

	const secondsPerDay = 24 * 60 * 60
	if input.ExpiresInDays > int(^uint(0)>>1)/secondsPerDay {
		return identityAdminError(IdentityAdminInvalid, "expires_in_days 超出支持范围", nil)
	}
	expiresInSeconds := input.ExpiresInDays * secondsPerDay
	if err := commands.auth.BanUser(ctx, authUserID, strings.TrimSpace(input.Reason), expiresInSeconds); err != nil {
		return authMutationError(err, "目标用户在身份系统中不存在，已拒绝封禁以避免状态漂移", "身份侧封禁失败")
	}
	if err := commands.users.UpdateStatus(ctx, target.ID, 0); err != nil {
		rollbackErr := commands.auth.UnbanUser(ctx, authUserID)
		if rollbackErr != nil {
			slog.Warn("ban compensation failed", "user_id", target.ID, "auth_user_id", authUserID, "rollback_err", rollbackErr)
			return reconciliationError("业务侧封禁失败，且身份侧回滚失败，请按运维流程补偿处理", err, rollbackErr)
		}
		return identityAdminError(IdentityAdminBusinessFailure, "业务侧封禁失败", err)
	}
	return nil
}

func (commands *identityAdminCommands) Unban(ctx context.Context, actor principal.Principal, targetUserID int64) error {
	target, authUserID, err := commands.loadManagedAuthUser(ctx, actor, targetUserID)
	if err != nil {
		return err
	}
	if target.Status == 1 {
		return nil
	}

	if err := commands.auth.UnbanUser(ctx, authUserID); err != nil {
		return authMutationError(err, "目标用户在身份系统中不存在，已拒绝解封以避免状态漂移", "身份侧解封失败")
	}
	if err := commands.users.UpdateStatus(ctx, target.ID, 1); err != nil {
		rollbackErr := commands.auth.BanUser(ctx, authUserID, "rollback after local unban failure", 0)
		if rollbackErr != nil {
			slog.Warn("unban compensation failed", "user_id", target.ID, "auth_user_id", authUserID, "rollback_err", rollbackErr)
			return reconciliationError("业务侧解封失败，且身份侧回滚失败，请按运维流程补偿处理", err, rollbackErr)
		}
		return identityAdminError(IdentityAdminBusinessFailure, "业务侧解封失败", err)
	}
	return nil
}

func (commands *identityAdminCommands) RevokeSessions(ctx context.Context, actor principal.Principal, targetUserID int64) error {
	_, authUserID, err := commands.loadManagedAuthUser(ctx, actor, targetUserID)
	if err != nil {
		return err
	}
	if err := commands.auth.RevokeUserSessions(ctx, authUserID); err != nil {
		return authMutationError(err, "目标用户在身份系统中不存在，无法吊销会话", "身份侧会话吊销失败")
	}
	return nil
}

func (commands *identityAdminCommands) Delete(ctx context.Context, actor principal.Principal, targetUserID int64) error {
	target, err := commands.loadTarget(ctx, actor, targetUserID, true)
	if err != nil {
		return err
	}
	if commands.auth == nil {
		return identityAdminError(IdentityAdminUnavailable, "认证服务未配置", errors.New("admin auth service client not configured"))
	}
	// The durable lifecycle owns Auth identity validation for a new deletion and
	// must also be able to resume after tombstoning has cleared auth_user_id.
	if err := commands.users.DeleteUser(ctx, target.ID, commands.auth); err != nil {
		if errors.Is(err, ErrUserDeletionBlockedByPaymentState) {
			return identityAdminError(IdentityAdminConflict, "存在未结算的 Payment、Refund 或 Dispute，暂不能注销用户", err)
		}
		return identityAdminError(IdentityAdminBusinessFailure, "用户删除流程已安全冻结，可重试以继续收敛", err)
	}
	return nil
}

func (commands *identityAdminCommands) promote(ctx context.Context, userID int64, authUserID, currentRole string) error {
	if err := commands.auth.SetUserRole(ctx, authUserID, principal.RoleAdmin.String()); err != nil {
		return authMutationError(err, "目标用户在身份系统中不存在，已拒绝改角色以避免状态漂移", "同步 Auth 管理员角色失败")
	}
	if err := commands.users.UpdateRole(ctx, userID, principal.RoleAdmin.String()); err != nil {
		rollbackErr := commands.auth.SetUserRole(ctx, authUserID, currentRole)
		if rollbackErr != nil {
			slog.Warn("role promotion compensation failed", "user_id", userID, "auth_user_id", authUserID, "rollback_role", currentRole, "rollback_err", rollbackErr)
			return reconciliationError("业务侧角色更新失败，且 Auth 角色回滚失败，请按运维流程补偿处理", err, rollbackErr)
		}
		return identityAdminError(IdentityAdminBusinessFailure, "业务侧角色更新失败", err)
	}
	return nil
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

func (commands *identityAdminCommands) loadManagedAuthUser(ctx context.Context, actor principal.Principal, targetUserID int64) (*User, string, error) {
	target, err := commands.loadTarget(ctx, actor, targetUserID, true)
	if err != nil {
		return nil, "", err
	}
	if commands.auth == nil {
		return nil, "", identityAdminError(IdentityAdminUnavailable, "认证服务未配置", errors.New("admin auth service client not configured"))
	}
	authUserID, err := commands.requireAuthIdentity(target, "目标用户缺少 auth_user_id，已拒绝操作以避免状态漂移")
	if err != nil {
		return nil, "", err
	}
	return target, authUserID, nil
}

func (commands *identityAdminCommands) loadTarget(ctx context.Context, actor principal.Principal, targetUserID int64, rejectAdmins bool) (*User, error) {
	if !actor.IsValid() {
		return nil, identityAdminError(IdentityAdminInvalid, "管理员身份无效", nil)
	}
	if !actor.IsAdmin() {
		return nil, identityAdminError(IdentityAdminForbidden, "需要管理员权限", nil)
	}
	if targetUserID <= 0 {
		return nil, identityAdminError(IdentityAdminInvalid, "无效的用户ID", nil)
	}
	if actor.UserID() == targetUserID {
		return nil, identityAdminError(IdentityAdminInvalid, "不能操作当前登录管理员自己", nil)
	}
	if commands == nil || commands.users == nil {
		return nil, identityAdminError(IdentityAdminUnavailable, "用户服务未配置", errors.New("identity admin user service not configured"))
	}
	target, err := commands.users.GetByID(ctx, targetUserID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, identityAdminError(IdentityAdminTargetNotFound, "用户不存在", err)
		}
		return nil, identityAdminError(IdentityAdminBusinessFailure, "读取用户失败", err)
	}
	if target == nil || target.ID <= 0 {
		return nil, identityAdminError(IdentityAdminTargetNotFound, "用户不存在", nil)
	}
	if rejectAdmins && normalizeAdminRole(target.Role) == principal.RoleAdmin.String() {
		return nil, identityAdminError(IdentityAdminInvalid, "不能操作管理员账号", nil)
	}
	return target, nil
}

func (commands *identityAdminCommands) requireAuthIdentity(target *User, message string) (string, error) {
	authUserID := strings.TrimSpace(derefString(target.AuthUserID))
	if authUserID == "" {
		return "", identityAdminError(IdentityAdminInvalid, message, nil)
	}
	return authUserID, nil
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
