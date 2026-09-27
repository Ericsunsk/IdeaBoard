import {
  listControlFeaturePrompts,
  saveControlFeaturePrompt,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'
import { withControlCopy } from '../shared/lib/controlCopy'

// Admin feature prompt boundary (W2 transports). The list read rides the
// Control Plane (`GET /api/control/v1/platform/feature-prompts`); the save
// moved to the Control Plane (`saveControlFeaturePrompt`, operator reason
// required) with the unified contract envelope (`{ data }` success /
// `{ error: { code, message, request_id } }` errors). The deleted
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

export async function fetchAdminFeaturePrompts(): Promise<FeaturePrompt[]> {
  try {
    const body = await listControlFeaturePrompts()
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
  const operatorReason = requireOperatorReason(options.operatorReason)
  const { slug, ...payload } = input
  try {
    const body = await saveControlFeaturePrompt(
      slug,
      { ...payload },
      {
        headers: buildContractRequestHeaders({
          requestId: newRequestId(),
          idempotencyKey: options.idempotencyKey ?? newRequestId(),
          operatorReason,
        }),
      },
    )
    return requireContractData<FeaturePrompt>(body, '保存提示词失败')
  } catch (error) {
    throw withControlCopy(error, '保存提示词失败')
  }
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(
  code: string,
  message: string,
  requestId?: string | undefined,
): Error & { code?: string | undefined; requestId?: string | undefined } {
  const error = new Error(message) as Error & {
    code?: string | undefined
    requestId?: string | undefined
  }
  error.code = code
  if (requestId) error.requestId = requestId
  return error
}

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.
