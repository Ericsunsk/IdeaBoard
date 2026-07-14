import {
  formatAdminDisplayError,
  type AdminToastPort,
  useAdminToast,
} from '../shared/lib/adminToast'

export type AdminUsersFeedback = {
  loadFailed: (error: unknown) => void
  adminStatusChangeForbidden: () => void
  unbanSucceeded: () => void
  unbanFailed: (error: unknown) => void
  banSucceeded: (username: string) => void
  banFailed: (error: unknown) => void
  missingAuthUserForSessionRevoke: () => void
  revokeSessionsSucceeded: () => void
  revokeSessionsFailed: (error: unknown) => void
  missingAuthUserForImpersonation: () => void
  impersonationStarted: (username: string) => void
  impersonationFailed: (error: unknown) => void
  userDeleted: () => void
  userDeleteFailed: (error: unknown) => void
  roleUpdated: (nextRole: 'admin' | 'user') => void
  roleUpdateFailed: (error: unknown) => void
}

export function createAdminUsersFeedback(toast: AdminToastPort): AdminUsersFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '加载用户失败')),
    adminStatusChangeForbidden: () => toast.warning('管理员账号不可封禁或解封'),
    unbanSucceeded: () => toast.success('已解除封禁'),
    unbanFailed: (error) => toast.error(formatAdminDisplayError(error, '解封失败')),
    banSucceeded: (username) => toast.success(`已高级封禁用户 ${username}`),
    banFailed: (error) => toast.error(formatAdminDisplayError(error, '封禁失败')),
    missingAuthUserForSessionRevoke: () => toast.warning('目标用户缺少 Auth 标识，无法吊销会话'),
    revokeSessionsSucceeded: () => toast.success('已强制吊销所有会话'),
    revokeSessionsFailed: (error) => toast.error(formatAdminDisplayError(error, '操作失败')),
    missingAuthUserForImpersonation: () => toast.error('目标用户缺少 Auth 标识，无法伪装登录'),
    impersonationStarted: (username) => toast.success(`正在以 ${username} 的身份登录...`),
    impersonationFailed: (error) => toast.error(formatAdminDisplayError(error, '伪装失败')),
    userDeleted: () => toast.success('用户已受控彻底注销'),
    userDeleteFailed: (error) => toast.error(formatAdminDisplayError(error, '注销失败')),
    roleUpdated: (nextRole) =>
      toast.success(nextRole === 'admin' ? '已设为管理员' : '已取消管理员'),
    roleUpdateFailed: (error) => toast.error(formatAdminDisplayError(error, '角色更新失败')),
  }
}

export function useAdminUsersFeedback() {
  return createAdminUsersFeedback(useAdminToast())
}
