import type {
  FeaturePrompt,
  FeaturePromptEvalFixtureResult,
  FeaturePromptEvalReport,
  FeaturePromptRevision,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from '../../../shared/api/generated-query/model'
import {
  evaluateAdminFeaturePromptDraft,
  fetchAdminFeaturePrompts,
  fetchLatestAdminFeaturePromptEvalReport,
  publishAdminFeaturePrompt,
  rollbackAdminFeaturePromptRevision,
} from './promptConfigApi'

export type {
  FeaturePrompt,
  FeaturePromptEvalFixtureResult,
  FeaturePromptEvalReport,
  FeaturePromptRevision,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
}

export type AdminPromptConfigResource = {
  fetchPrompts: typeof fetchAdminFeaturePrompts
  fetchLatestEvalReport: typeof fetchLatestAdminFeaturePromptEvalReport
  publishPrompt: typeof publishAdminFeaturePrompt
  rollbackPrompt: typeof rollbackAdminFeaturePromptRevision
  evaluatePrompt: typeof evaluateAdminFeaturePromptDraft
}

export function createAdminPromptConfigResource(): AdminPromptConfigResource {
  return {
    evaluatePrompt: evaluateAdminFeaturePromptDraft,
    fetchPrompts: fetchAdminFeaturePrompts,
    fetchLatestEvalReport: fetchLatestAdminFeaturePromptEvalReport,
    publishPrompt: publishAdminFeaturePrompt,
    rollbackPrompt: rollbackAdminFeaturePromptRevision,
  }
}
