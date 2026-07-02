import type { AsyncActionFeedback } from '../../shared/composables/useAsyncAction'
import { useAppToast } from '../../shared/composables/useAppToast'
import { formatDisplayErrorMessage } from '../../shared/utils/errorMessages'

type ProfileToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error'>

export type ProfileFeedback = {
  invalidAvatarFile: () => void
  avatarUpload: AsyncActionFeedback<void>
  passwordChange: AsyncActionFeedback<boolean>
}

export function createProfileFeedback(appToast: ProfileToast): ProfileFeedback {
  return {
    invalidAvatarFile() {
      appToast.warning('仅支持上传图片文件')
    },
    avatarUpload: {
      success() {
        appToast.success('头像已更新')
      },
      error(error) {
        appToast.error(formatDisplayErrorMessage(error, '头像更新失败'))
      },
    },
    passwordChange: {
      success() {
        appToast.success('密码更新成功，请重新登录')
      },
      error(error) {
        appToast.error(formatDisplayErrorMessage(error, '密码更新失败'))
      },
    },
  }
}

export function useProfileFeedback() {
  return createProfileFeedback(useAppToast())
}
