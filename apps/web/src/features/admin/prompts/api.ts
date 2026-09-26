import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import {
  listControlFeaturePrompts,
  saveControlFeaturePrompt,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
} from '../../../shared/api/contractKernel'

// Admin feature prompt boundary (W2 transports). The list read rides the
// Control Plane (`GET /api/control/v1/platform/feature-prompts`, a guarded
// proxy keeping the legacy `{code, msg, data}` envelope); the save moved to
// the Control Plane (`saveControlFeaturePrompt`, operator reason required)
// with the same envelope. The deleted `/api/admin/feature-prompts/*`
// routes are gone; this module is the single transport.
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
  const data = await resolveApiData(
    listControlFeaturePrompts() as Promise<ApiResponseWithData<{ prompts: FeaturePrompt[] }>>,
    '加载提示词失败',
  )
  return data.prompts ?? []
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
    return readLegacyData<FeaturePrompt>(body, '保存提示词失败')
  } catch (error) {
    throw withControlCopy(error, '保存提示词失败')
  }
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(code: string, message: string): Error & { code?: string | undefined } {
  const error = new Error(message) as Error & { code?: string | undefined }
  error.code = code
  return error
}

// withControlCopy maps control error codes to operator-facing copy.
// Branch on code, never on message text.
function withControlCopy(error: unknown, fallback: string): Error {
  const code = readErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError(code === 409 ? 'idempotency_conflict' : code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
}

function readErrorCode(error: unknown): string | number | undefined {
  if (error && typeof error === 'object' && 'code' in error) {
    const code = (error as { code?: unknown }).code
    if (typeof code === 'string' || typeof code === 'number') return code
  }
  if (error && typeof error === 'object' && 'status' in error) {
    const status = (error as { status?: unknown }).status
    if (status === 409) return 409
  }
  return undefined
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

// Control feature-prompt routes keep the legacy `{ code, msg, data? }`
// shape: a numeric `code` of 0 means success, anything else is a failure.
function readLegacyData<T>(body: unknown, fallback: string): T {
  if (isRecord(body) && 'code' in body) {
    if (body.code !== 0) {
      const message = typeof body.msg === 'string' && body.msg ? body.msg : fallback
      throw controlError(typeof body.code === 'string' ? body.code : 'control_failed', message)
    }
    if (body.data == null) throw controlError('control_failed', fallback)
    return body.data as T
  }
  throw controlError('control_failed', fallback)
}
