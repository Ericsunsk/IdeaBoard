import type { FeaturePrompt } from './api'
import { fetchAdminFeaturePrompts, saveAdminFeaturePrompt } from './api'
import type { SaveFeaturePromptOptions } from './api'

export type { FeaturePrompt }

export type AdminPromptConfigResource = {
  fetchPrompts: typeof fetchAdminFeaturePrompts
  savePrompt: (
    input: Parameters<typeof saveAdminFeaturePrompt>[0],
    options?: SaveFeaturePromptOptions,
  ) => ReturnType<typeof saveAdminFeaturePrompt>
}

export function createAdminPromptConfigResource(): AdminPromptConfigResource {
  return {
    fetchPrompts: fetchAdminFeaturePrompts,
    savePrompt: saveAdminFeaturePrompt,
  }
}
