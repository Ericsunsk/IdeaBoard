import type { FeaturePrompt } from './api'
import { fetchAdminFeaturePrompts, saveAdminFeaturePrompt } from './api'

export type { FeaturePrompt }

export type AdminPromptConfigResource = {
  fetchPrompts: typeof fetchAdminFeaturePrompts
  savePrompt: typeof saveAdminFeaturePrompt
}

export function createAdminPromptConfigResource(): AdminPromptConfigResource {
  return {
    fetchPrompts: fetchAdminFeaturePrompts,
    savePrompt: saveAdminFeaturePrompt,
  }
}
