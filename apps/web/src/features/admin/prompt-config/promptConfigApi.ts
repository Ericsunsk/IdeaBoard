import {
  listAdminFeaturePrompts,
  setAdminFeaturePrompt,
} from '../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type { FeaturePrompt, SetAdminFeaturePromptBody } from '../../../shared/api/generated-query/model'
import { requireApiData } from '../shared/queries/adminApiResponse'

export type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function fetchAdminFeaturePrompts(): Promise<FeaturePrompt[]> {
  const res = await listAdminFeaturePrompts()
  const data = requireApiData(res, '加载提示词失败')
  return data.prompts ?? []
}

export async function publishAdminFeaturePrompt(
  input: SaveFeaturePromptInput,
): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  const res = await setAdminFeaturePrompt(slug, payload)
  return requireApiData(res, '保存提示词失败')
}
