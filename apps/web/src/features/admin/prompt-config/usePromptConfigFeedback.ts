import {
  formatAdminDisplayError,
  type AdminToastPort,
  useAdminToast,
} from '../shared/lib/toast'

export type AdminPromptConfigFeedback = {
  loadPromptsFailed: (error: unknown) => void
  promptDraftInvalid: (message: string) => void
  promptSaved: (featureTitle: string) => void
  savePromptFailed: (error: unknown) => void
}

function createAdminPromptConfigFeedback(toast: AdminToastPort): AdminPromptConfigFeedback {
  return {
    loadPromptsFailed: (error) => toast.error(formatAdminDisplayError(error, '加载提示词失败')),
    promptDraftInvalid: (message) => toast.warning(message),
    promptSaved: (featureTitle) => toast.success(`「${featureTitle}」提示词已保存`),
    savePromptFailed: (error) => toast.error(formatAdminDisplayError(error, '保存提示词失败')),
  }
}

export function usePromptConfigFeedback() {
  return createAdminPromptConfigFeedback(useAdminToast())
}
