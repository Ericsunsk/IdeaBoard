import { api } from '../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'

// Admin feature prompt boundary (issue #809). The generated
// `admin-feature-prompts` client is gone but the backend admin plane still
// serves `/api/admin/feature-prompts*` with the shared envelope, so this
// boundary transports through the central `api.request` with local wire
// types.
export type FeaturePrompt = {
  feature_slug: string
  prompt: string
  updated_at?: string | null
}

export type SetAdminFeaturePromptBody = {
  prompt: string
}

export type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function fetchAdminFeaturePrompts(): Promise<FeaturePrompt[]> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ prompts: FeaturePrompt[] }>>('/admin/feature-prompts', {
      method: 'GET',
    }),
    '加载提示词失败',
  )
  return data.prompts ?? []
}

export async function saveAdminFeaturePrompt(
  input: SaveFeaturePromptInput,
): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  return resolveApiData(
    api.request<ApiResponseWithData<FeaturePrompt>>(
      `/admin/feature-prompts/${encodeURIComponent(slug)}`,
      {
        method: 'PUT',
        body: payload as unknown as Record<string, unknown>,
      },
    ),
    '保存提示词失败',
  )
}
