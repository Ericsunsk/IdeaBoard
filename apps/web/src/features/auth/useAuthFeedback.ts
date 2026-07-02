import type { AsyncActionFeedback } from '../../shared/composables/useAsyncAction'
import { extractBetterAuthErrorMessage } from '../../shared/auth/client'
import { useAppToast } from '../../shared/composables/useAppToast'

type AuthToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'error'>

export type AuthFeedback = {
  login: AsyncActionFeedback<boolean>
  register: AsyncActionFeedback<boolean>
  stopImpersonating: AsyncActionFeedback<boolean>
}

export function createAuthFeedback(appToast: AuthToast): AuthFeedback {
  return {
    login: {
      success() {
        appToast.success('登录成功')
      },
      error(error) {
        appToast.error(
          error instanceof Error && error.message ? error.message : '登录失败，请稍后重试',
        )
      },
    },
    register: {
      success() {
        appToast.success('注册成功')
      },
      error(error) {
        appToast.error(
          error instanceof Error && error.message ? error.message : '注册失败，请稍后重试',
        )
      },
    },
    stopImpersonating: {
      success() {
        appToast.success('已退出伪装身份')
      },
      error(error) {
        appToast.error(extractBetterAuthErrorMessage(error, '退出伪装失败，请重新登录管理员账号'))
      },
    },
  }
}

export function useAuthFeedback() {
  return createAuthFeedback(useAppToast())
}
