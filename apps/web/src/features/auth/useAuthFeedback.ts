import type { AsyncActionFeedback } from '../../core/composables/useAsyncAction'
import { useAppToast } from '../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../core/errors/errorCatalog'
import { i18n } from '../../core/i18n'

type AuthToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'error'>

export type AuthFeedback = {
  login: AsyncActionFeedback<boolean>
  register: AsyncActionFeedback<boolean>
  stopImpersonating: AsyncActionFeedback<boolean>
}

function createAuthFeedback(appToast: AuthToast): AuthFeedback {
  return {
    login: {
      success() {
        appToast.success(i18n.global.t('auth.toasts.signedIn'))
      },
      error(error) {
        appToast.error(userFacingErrorCopy(error, i18n.global.t('auth.errors.signInFailed')))
      },
    },
    register: {
      success() {
        appToast.success(i18n.global.t('auth.toasts.signedUp'))
      },
      error(error) {
        appToast.error(userFacingErrorCopy(error, i18n.global.t('auth.errors.signUpFailed')))
      },
    },
    stopImpersonating: {
      success() {
        appToast.success(i18n.global.t('auth.toasts.impersonationEnded'))
      },
      error(error) {
        appToast.error(
          userFacingErrorCopy(error, i18n.global.t('auth.errors.stopImpersonatingRetryAsAdmin')),
        )
      },
    },
  }
}

export function useAuthFeedback() {
  return createAuthFeedback(useAppToast())
}
