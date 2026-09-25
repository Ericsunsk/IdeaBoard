import { api } from '../../../shared/api/client'
import { CONTROL_API_PREFIX } from '../../../shared/api/controlMutator'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import { getListControlFeaturePromptsUrl } from '../../../shared/api/generated-query/control/control-platform/control-platform'

// Admin feature prompt boundary (final cut, partial). The list read moved
// to the Control Plane (`GET /api/control/v1/platform/feature-prompts`, a
// guarded proxy keeping the legacy `{code, msg, data}` envelope), so
// `fetchAdminFeaturePrompts` repoints at the generated Control path with
// wire types untouched. `saveAdminFeaturePrompt` stays on the legacy route
// until the admin UI collects the operator reason the Control mutation
// guard requires — see the blocked list.
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
    api.request<ApiResponseWithData<{ prompts: FeaturePrompt[] }>>(
      `${CONTROL_API_PREFIX}${getListControlFeaturePromptsUrl()}`,
      {
        method: 'GET',
      },
    ),
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
