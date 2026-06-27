import { setAdminFeaturePrompt } from '../../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type {
  FeaturePrompt,
  SetAdminFeaturePromptBody,
} from '../../../../shared/api/generated-query/model'
import { requireApiData } from './adminApiResponse'

type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function saveFeaturePrompt(input: SaveFeaturePromptInput): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  const res = await setAdminFeaturePrompt(slug, payload)
  return requireApiData(res, '保存提示词失败')
}
