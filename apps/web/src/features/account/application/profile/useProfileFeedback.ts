import type { AsyncActionFeedback } from '../../../../shared/composables/useAsyncAction'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../shared/errors/errorCatalog'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import { i18n } from '../../../../shared/i18n'

type ProfileToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'info' | 'warning' | 'error'>

export type ProfileFeedback = {
  invalidAvatarFile: () => void
  logoutFailed: () => void
  rewardRedemptionSucceeded: (creditedAmount: number, replayed: boolean) => void
  rewardRedemptionFailed: (error: unknown) => void
  avatarUpload: AsyncActionFeedback<void>
  passwordChange: AsyncActionFeedback<boolean>
}

function createProfileFeedback(appToast: ProfileToast): ProfileFeedback {
  return {
    invalidAvatarFile() {
      appToast.warning(i18n.global.t('account.toast.avatarFileUnsupported'))
    },
    logoutFailed() {
      appToast.error(i18n.global.t('account.toast.signOutFailed'))
    },
    rewardRedemptionSucceeded(creditedAmount, replayed) {
      appToast.success(
        replayed
          ? i18n.global.t('account.toast.rewardAlreadyRedeemed')
          : i18n.global.t('account.toast.rewardCredited', {
              amount: formatIdeaAmount(creditedAmount),
            }),
      )
    },
    rewardRedemptionFailed(error) {
      appToast.error(userFacingErrorCopy(error, i18n.global.t('account.errors.networkRetry')))
    },
    avatarUpload: {
      success() {
        appToast.success(i18n.global.t('account.toast.avatarUpdated'))
      },
      error(error) {
        appToast.error(
          userFacingErrorCopy(error, i18n.global.t('account.toast.avatarUpdateFailed')),
        )
      },
    },
    passwordChange: {
      success() {
        appToast.success(i18n.global.t('account.toast.passwordUpdated'))
      },
      error(error) {
        appToast.error(
          userFacingErrorCopy(error, i18n.global.t('account.toast.passwordUpdateFailed')),
        )
      },
    },
  }
}

export function useProfileFeedback() {
  return createProfileFeedback(useAppToast())
}
