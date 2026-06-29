import {
  getListAdminFeaturePromptsQueryKey,
  setAdminFeaturePrompt,
  useListAdminFeaturePrompts,
} from '../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type {
  FeaturePrompt,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
  SetAdminFeaturePromptBody,
} from '../../../shared/api/generated-query/model'
import { requireApiData } from '../shared/queries/adminApiResponse'

export { getListAdminFeaturePromptsQueryKey, useListAdminFeaturePrompts }
export type { FeaturePrompt, FeaturePromptVariable, FeaturePromptVariableTone }

type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function saveFeaturePrompt(input: SaveFeaturePromptInput): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  const res = await setAdminFeaturePrompt(slug, payload)
  return requireApiData(res, '保存提示词失败')
}
