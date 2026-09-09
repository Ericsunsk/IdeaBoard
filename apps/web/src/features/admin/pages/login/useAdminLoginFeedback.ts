import {
  formatAdminDisplayError,
  type AdminToastPort,
  useAdminToast,
} from '../../shared/lib/toast'

export type AdminLoginFeedback = {
  succeeded: () => void
  sessionRevokeFailed: () => void
  nonAdminAccount: () => void
  failed: (error: unknown) => void
}

function createAdminLoginFeedback(toast: AdminToastPort): AdminLoginFeedback {
  return {
    succeeded: () => toast.success('管理员登录成功'),
    sessionRevokeFailed: () => toast.warning('会话撤销失败，请稍后重试'),
    nonAdminAccount: () => toast.error('该账号不是管理员'),
    failed: (error) => toast.error(formatAdminDisplayError(error, '管理员登录失败')),
  }
}

export function useAdminLoginFeedback() {
  return createAdminLoginFeedback(useAdminToast())
}
