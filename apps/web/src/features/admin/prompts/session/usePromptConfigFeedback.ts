import { defineFeedback } from '../../shared'

export type AdminPromptConfigFeedback = {
  loadPromptsFailed: (error: unknown) => void
  promptDraftInvalid: (message: string) => void
  promptSaved: (featureTitle: string) => void
  savePromptFailed: (error: unknown) => void
}

const ADMIN_PROMPT_CONFIG_COPY = {
  loadPromptsFailed: { error: '加载提示词失败' },
  promptDraftInvalid: { warning: (message: string) => message },
  promptSaved: { success: (featureTitle: string) => `「${featureTitle}」提示词已保存` },
  savePromptFailed: { error: '保存提示词失败' },
}

export function usePromptConfigFeedback(): AdminPromptConfigFeedback {
  return defineFeedback<AdminPromptConfigFeedback>(ADMIN_PROMPT_CONFIG_COPY)
}
