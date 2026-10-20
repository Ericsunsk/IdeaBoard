import { ref } from 'vue'
import { useUserStore } from '../../../auth'
import { useAsyncAction } from '../../../../core/composables/useAsyncAction'
import { normalizeDisplayName, renameDisplayName, validateDisplayName } from './resource'
import { type ProfileFeedback, useProfileFeedback } from './useProfileFeedback'

type DisplayNameRenameUserStore = Pick<
  ReturnType<typeof useUserStore>,
  'user' | 'captureSession' | 'setAuth'
>

export type DisplayNameRenameDependencies = {
  rename?: typeof renameDisplayName
  userStore?: DisplayNameRenameUserStore
  profileFeedback?: Pick<ProfileFeedback, 'invalidDisplayName' | 'displayNameRename'>
}

export function useDisplayNameRename(deps: DisplayNameRenameDependencies = {}) {
  const rename = deps.rename ?? renameDisplayName
  const userStore = deps.userStore ?? useUserStore()
  const profileFeedback = deps.profileFeedback ?? useProfileFeedback()
  const { runAsyncAction } = useAsyncAction()

  const isEditing = ref(false)
  const isRenaming = ref(false)
  const draftName = ref('')

  function currentName(): string {
    return userStore.user?.display_name ?? ''
  }

  function startRename() {
    if (isRenaming.value) return
    draftName.value = currentName()
    isEditing.value = true
  }

  function cancelRename() {
    if (isRenaming.value) return
    draftName.value = currentName()
    isEditing.value = false
  }

  async function commitRename() {
    if (isRenaming.value || !isEditing.value) return
    const isCurrentSession = userStore.captureSession()
    if (!isCurrentSession()) return
    const previous = normalizeDisplayName(currentName())
    const violation = validateDisplayName(draftName.value)
    if (violation) {
      profileFeedback.invalidDisplayName(violation)
      return
    }
    const next = normalizeDisplayName(draftName.value)
    if (next === previous) {
      cancelRename()
      return
    }
    const updated = await runAsyncAction(
      async () => {
        const renamed = await rename(next)
        if (!isCurrentSession()) return null
        const current = userStore.user
        if (!current || current.public_id !== renamed.public_id) return null
        userStore.setAuth({ ...current, ...renamed })
        return renamed
      },
      {
        isLoading: isRenaming,
        feedback: {
          success: (renamed) => {
            if (renamed) profileFeedback.displayNameRename.success?.(renamed)
          },
          error: (error) => {
            if (isCurrentSession()) profileFeedback.displayNameRename.error?.(error)
          },
        },
      },
    )
    if (updated == null) {
      if (!isCurrentSession()) {
        draftName.value = currentName()
        isEditing.value = false
        return
      }
      draftName.value = previous
      return
    }
    isEditing.value = false
  }

  return {
    isEditing,
    isRenaming,
    draftName,
    startRename,
    cancelRename,
    commitRename,
  }
}
