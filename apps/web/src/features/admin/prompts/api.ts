import {
  listControlFeaturePrompts,
  saveControlFeaturePrompt,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'

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

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text. request_id flows through for
// correlation wherever the app surfaces the error.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  const requestId = readRequestId(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError('idempotency_conflict', '操作已提交，请勿重复操作', requestId)
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因', requestId)
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback, requestId)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (typeof status === 'number') return status
  }
  return undefined
}

function readRequestId(error: unknown): string | undefined {
  if (error && typeof error === 'object') {
    const candidate = error as { requestId?: unknown; request_id?: unknown }
    if (typeof candidate.requestId === 'string' && candidate.requestId) return candidate.requestId
    if (typeof candidate.request_id === 'string' && candidate.request_id) {
      return candidate.request_id
    }
  }
  return undefined
}
