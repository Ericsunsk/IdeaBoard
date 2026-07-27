import {
  listAdminFeaturePrompts,
  setAdminFeaturePrompt,
} from '../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type {
  FeaturePrompt,
  SetAdminFeaturePromptBody,
} from '../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../shared/api/apiResponse'

export type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function fetchAdminFeaturePrompts(): Promise<FeaturePrompt[]> {
  const data = await resolveApiData(listAdminFeaturePrompts(), '加载提示词失败')
  return data.prompts ?? []
}

export async function saveAdminFeaturePrompt(
  input: SaveFeaturePromptInput,
): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  return resolveApiData(setAdminFeaturePrompt(slug, payload), '保存提示词失败')
}
