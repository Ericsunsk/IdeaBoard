import { ref, type Ref } from 'vue'
import { useUserStore } from '../../../auth'
import { uploadApi } from '../../../../api/upload'
import {
  type AsyncActionFeedback,
  useAsyncAction,
} from '../../../../core/composables/useAsyncAction'
import { type ProfileFeedback, useProfileFeedback } from './useProfileFeedback'

type RunAsyncAction = <T>(
  action: () => T | Promise<T>,
  options?: {
    loading?: Ref<boolean>
    feedback?: AsyncActionFeedback<T>
  },
) => Promise<T | undefined>

type AvatarUploadUserStore = {
  setAvatarLocal: (avatar: string) => void
}

export type AvatarUploadDependencies = {
  uploadAvatar?: typeof uploadApi.uploadAvatar
  userStore?: AvatarUploadUserStore
  profileFeedback?: Pick<ProfileFeedback, 'invalidAvatarFile' | 'avatarUpload'>
  runAsyncAction?: RunAsyncAction
}

export function useAvatarUpload(deps: AvatarUploadDependencies = {}) {
  const uploadAvatar = deps.uploadAvatar ?? uploadApi.uploadAvatar
  const userStore = deps.userStore ?? useUserStore()
  const profileFeedback = deps.profileFeedback ?? useProfileFeedback()
  const runAsyncAction = deps.runAsyncAction ?? useAsyncAction().runAsyncAction

  const isUploading = ref(false)

  async function selectFile(file?: File | null) {
    if (!file) return
    if (!file.type.startsWith('image/')) {
      profileFeedback.invalidAvatarFile()
      return
    }

    await runAsyncAction(
      async () => {
        const avatarUrl = (await uploadAvatar(file)).avatar.trim()
        userStore.setAvatarLocal(avatarUrl)
      },
      {
        isLoading: isUploading,
        feedback: profileFeedback.avatarUpload,
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
    isUploading,
    selectFile,
    handleFileChange,
  }
}
