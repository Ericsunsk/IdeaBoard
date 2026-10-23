import { requireContractData } from '@/api/contractKernel'
import { buildControlMutationHeaders } from '../../shared'
import {
  listControlFeaturePrompts,
  saveControlFeaturePrompt,
} from '@/api/generated-query/control/control-platform/control-platform'
import { withControlCopy } from '../../shared'

export type FeaturePrompt = {
  feature_slug: string
  prompt: string
  updated_at?: string | null
}

type SetAdminFeaturePromptBody = {
  prompt: string
}

export type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export type SaveFeaturePromptOptions = {
  idempotencyKey?: string | undefined
}

export async function fetchAdminFeaturePrompts(signal?: AbortSignal): Promise<FeaturePrompt[]> {
  try {
    const body = await listControlFeaturePrompts(signal == null ? undefined : { signal })
    const data = requireContractData<{ prompts: FeaturePrompt[] }>(body, '加载提示词失败')
    return data.prompts ?? []
  } catch (error) {
    throw withControlCopy(error, '加载提示词失败')
  }
}

export async function saveAdminFeaturePrompt(
  input: SaveFeaturePromptInput,
  options?: SaveFeaturePromptOptions,
): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  try {
    const body = await saveControlFeaturePrompt(
      slug,
      { ...payload },
      {
        headers: buildControlMutationHeaders(options),
      },
    )
    return requireContractData<FeaturePrompt>(body, '保存提示词失败')
  } catch (error) {
    throw withControlCopy(error, '保存提示词失败')
  }
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.
