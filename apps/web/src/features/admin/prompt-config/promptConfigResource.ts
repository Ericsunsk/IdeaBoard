import type {
  FeaturePrompt,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from '../../../shared/api/generated-query/model'
import { fetchAdminFeaturePrompts, publishAdminFeaturePrompt } from './promptConfigApi'

export type {
  FeaturePrompt,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
}

export type AdminPromptConfigResource = {
  fetchPrompts: typeof fetchAdminFeaturePrompts
  publishPrompt: typeof publishAdminFeaturePrompt
}

export function createAdminPromptConfigResource(): AdminPromptConfigResource {
  return {
    fetchPrompts: fetchAdminFeaturePrompts,
    publishPrompt: publishAdminFeaturePrompt,
  }
}
