import type { AsyncActionFeedback } from '../../../../core/composables/useAsyncAction'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../workspaceBilling'
import { formatIdeaAmount } from '../../../../core/i18n/userIdeaCurrency'
import { i18n } from '../../../../core/i18n'
import type { User } from '../../../../api/sessionUserType'

type ProfileToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'info' | 'warning' | 'error'>

export type ProfileFeedback = {
  invalidAvatarFile: () => void
  invalidDisplayName: (copy: string) => void
  logoutFailed: () => void
  rewardRedemptionSucceeded: (creditedAmount: number, replayed: boolean) => void
  rewardRedemptionFailed: (error: unknown) => void
  avatarUpload: AsyncActionFeedback<void>
  displayNameRename: AsyncActionFeedback<User>
  passwordChange: AsyncActionFeedback<boolean>
}

function createProfileFeedback(appToast: ProfileToast): ProfileFeedback {
  return {
    invalidAvatarFile() {
      appToast.warning(i18n.global.t('account.toast.avatarFileUnsupported'))
    },
    invalidDisplayName(copy) {
      appToast.error(copy)
    },
    logoutFailed() {
      appToast.error(i18n.global.t('account.toast.signOutFailed'))
    },
    rewardRedemptionSucceeded(creditedAmount, isReplayed) {
      appToast.success(
        isReplayed
          ? i18n.global.t('account.toast.rewardAlreadyRedeemed')
          : i18n.global.t('account.toast.rewardCredited', {
              amount: formatIdeaAmount(creditedAmount),
            }),
      )
    },
    rewardRedemptionFailed(error) {
      appToast.error(
        userFacingErrorCopy(
          error,
          i18n.global.t('account.errors.networkRetry'),
          ACCOUNT_GUIDANCE_COPY_KEYS,
        ),
      )
    },
    avatarUpload: {
      success() {
        appToast.success(i18n.global.t('account.toast.avatarUpdated'))
      },
      error(error) {
        appToast.error(
          userFacingErrorCopy(
            error,
            i18n.global.t('account.toast.avatarUpdateFailed'),
            ACCOUNT_GUIDANCE_COPY_KEYS,
          ),
        )
      },
    },
    displayNameRename: {
      success() {
        appToast.success(i18n.global.t('account.toast.displayNameUpdated'))
      },
      error(error) {
        appToast.error(
          userFacingErrorCopy(
            error,
            i18n.global.t('account.toast.displayNameUpdateFailed'),
            ACCOUNT_GUIDANCE_COPY_KEYS,
          ),
        )
      },
    },
    passwordChange: {
      success() {
        appToast.success(i18n.global.t('account.toast.passwordUpdated'))
      },
      error(error) {
        appToast.error(
          userFacingErrorCopy(
            error,
            i18n.global.t('account.toast.passwordUpdateFailed'),
            ACCOUNT_GUIDANCE_COPY_KEYS,
          ),
        )
      },
    },
  }
}

export function useProfileFeedback() {
  return createProfileFeedback(useAppToast())
}
