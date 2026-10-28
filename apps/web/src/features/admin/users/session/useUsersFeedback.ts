import { defineFeedback } from '../../shared'

export type AdminUsersFeedback = {
  loadFailed: (error: unknown) => void
  adminStatusChangeForbidden: () => void
  unbanSucceeded: () => void
  unbanFailed: (error: unknown) => void
  banSucceeded: (displayName: string) => void
  banFailed: (error: unknown) => void
  missingAuthUserForSessionRevoke: () => void
  revokeSessionsSucceeded: () => void
  revokeSessionsFailed: (error: unknown) => void
  missingAuthUserForImpersonation: () => void
  impersonationStarted: (displayName: string) => void
  impersonationFailed: (error: unknown) => void
  userDeleted: () => void
  userDeleteFailed: (error: unknown) => void
  roleUpdated: (nextRole: 'admin' | 'user') => void
  roleUpdateFailed: (error: unknown) => void
}

const ADMIN_USERS_COPY = {
  loadFailed: { error: '加载用户失败' },
  adminStatusChangeForbidden: { warning: '管理员账号不可封禁或解封' },
  unbanSucceeded: { success: '已解除封禁' },
  unbanFailed: { error: '解封失败' },
  banSucceeded: { success: (displayName: string) => `已高级封禁用户 ${displayName}` },
  banFailed: { error: '封禁失败' },
  missingAuthUserForSessionRevoke: { warning: '目标用户缺少 Auth 标识，无法吊销会话' },
  revokeSessionsSucceeded: { success: '已强制吊销所有会话' },
  revokeSessionsFailed: { error: '操作失败' },
  missingAuthUserForImpersonation: { errorText: '目标用户缺少 Auth 标识，无法伪装登录' },
  impersonationStarted: { success: (displayName: string) => `正在以 ${displayName} 的身份登录...` },
  impersonationFailed: { error: '伪装失败' },
  userDeleted: { success: '用户已受控彻底注销' },
  userDeleteFailed: { error: '注销失败' },
  roleUpdated: {
    success: (nextRole: 'admin' | 'user') =>
      nextRole === 'admin' ? '已设为管理员' : '已取消管理员',
  },
  roleUpdateFailed: { error: '角色更新失败' },
}

export function useUsersFeedback(): AdminUsersFeedback {
  return defineFeedback<AdminUsersFeedback>(ADMIN_USERS_COPY)
}
