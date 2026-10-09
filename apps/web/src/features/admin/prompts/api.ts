import { requireContractData } from '../../../shared/api/contractKernel'
import { buildRoutineControlMutationHeaders } from '../shared/lib/controlMutation'
import {
  listControlFeaturePrompts,
  saveControlFeaturePrompt,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import { withControlCopy } from '../shared/lib/controlCopy'

// Admin feature prompt boundary (W2 transports). The list read rides the
// Control Plane (`GET /api/control/v1/platform/feature-prompts`); the save
// moved to the Control Plane (`saveControlFeaturePrompt`) as a routine
// config write (backend ReasonOptional): no confirm dialog, the audit
// records an empty reason, while the header-only Idempotency-Key stays
// required. The unified contract envelope (`{ data }` success / `{ error:
// { code, message, request_id } }` errors) applies. The deleted
// `/api/admin/feature-prompts/*` routes are gone; this module is the single
// transport.
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

export type SaveFeaturePromptOptions = {
  operatorReason: string
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
  options: SaveFeaturePromptOptions,
): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  try {
    const body = await saveControlFeaturePrompt(
      slug,
      { ...payload },
      {
        headers: buildRoutineControlMutationHeaders(options),
      },
    )
    return requireContractData<FeaturePrompt>(body, '保存提示词失败')
  } catch (error) {
    throw withControlCopy(error, '保存提示词失败')
  }
}

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.
