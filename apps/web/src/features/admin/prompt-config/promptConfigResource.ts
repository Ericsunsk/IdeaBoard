import type {
  FeaturePrompt,
  FeaturePromptEvalFixtureResult,
  FeaturePromptEvalReport,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from '../../../shared/api/generated-query/model'
import {
  evaluateAdminFeaturePromptDraft,
  fetchAdminFeaturePrompts,
  fetchLatestAdminFeaturePromptEvalReport,
  publishAdminFeaturePrompt,
} from './promptConfigApi'

export type {
  FeaturePrompt,
  FeaturePromptEvalFixtureResult,
  FeaturePromptEvalReport,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
}

export type AdminPromptConfigResource = {
  fetchPrompts: typeof fetchAdminFeaturePrompts
  fetchLatestEvalReport: typeof fetchLatestAdminFeaturePromptEvalReport
  publishPrompt: typeof publishAdminFeaturePrompt
  evaluatePrompt: typeof evaluateAdminFeaturePromptDraft
}

export function createAdminPromptConfigResource(): AdminPromptConfigResource {
  return {
    evaluatePrompt: evaluateAdminFeaturePromptDraft,
    fetchPrompts: fetchAdminFeaturePrompts,
    fetchLatestEvalReport: fetchLatestAdminFeaturePromptEvalReport,
    publishPrompt: publishAdminFeaturePrompt,
  }
}
