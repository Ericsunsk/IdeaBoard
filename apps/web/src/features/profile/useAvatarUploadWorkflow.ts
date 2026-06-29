import { ref, type Ref } from 'vue'
import { useUserStore } from '../../app/stores/user'
import { uploadApi, type UploadAvatarResponse } from '../../shared/api/upload'
import { useAsyncAction } from '../../shared/composables/useAsyncAction'
import { useAppToast } from '../../shared/composables/useAppToast'
import type { ApiResponse } from '../../shared/types'

type RunAsyncAction = <T>(
  action: () => T | Promise<T>,
  options?: {
    loading?: Ref<boolean>
    errorMessage?: string | ((error: unknown) => string)
  },
) => Promise<T | undefined>

type AvatarUploadUserStore = {
  setAvatarLocal: (avatar: string) => void
}

type AvatarUploadToast = {
  success: (message: string) => void
  warning: (message: string) => void
}

export type AvatarUploadWorkflowDependencies = {
  uploadAvatar?: (file: File) => Promise<ApiResponse<UploadAvatarResponse>>
  userStore?: AvatarUploadUserStore
  appToast?: AvatarUploadToast
  runAsyncAction?: RunAsyncAction
}

export function useAvatarUploadWorkflow(deps: AvatarUploadWorkflowDependencies = {}) {
  const uploadAvatar = deps.uploadAvatar ?? uploadApi.uploadAvatar
  const userStore = deps.userStore ?? useUserStore()
  const appToast = deps.appToast ?? useAppToast()
  const runAsyncAction = deps.runAsyncAction ?? useAsyncAction().runAsyncAction

  const uploading = ref(false)

  async function selectFile(file?: File | null) {
    if (!file) return
    if (!file.type.startsWith('image/')) {
      appToast.warning('仅支持上传图片文件')
      return
    }

    await runAsyncAction(
      async () => {
        const uploadRes = await uploadAvatar(file)
        const avatarUrl = uploadRes.data?.avatar?.trim()
        if (!avatarUrl) {
          throw new Error('头像上传失败，请稍后重试')
        }
        userStore.setAvatarLocal(avatarUrl)
        appToast.success('头像已更新')
      },
      {
        loading: uploading,
        errorMessage: '头像更新失败',
      },
    )
  }

  async function handleFileChange(event: Event) {
    const input = event.target as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    await selectFile(file)
  }

  return {
    uploading,
    selectFile,
    handleFileChange,
  }
}
