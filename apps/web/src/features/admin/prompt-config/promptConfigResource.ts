import type { FeaturePrompt } from '../../../shared/api/generated-query/model'
import { fetchAdminFeaturePrompts, saveAdminFeaturePrompt } from './promptConfigApi'

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
