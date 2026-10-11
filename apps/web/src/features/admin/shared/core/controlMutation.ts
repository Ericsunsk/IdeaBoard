import { buildContractRequestHeaders, newRequestId } from '../../../../api/contractKernel'

/** Common options carried by every Admin Control mutation. */
export type ControlMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export type ControlError = Error & {
  code?: string | undefined
  requestId?: string | undefined
}

export type ControlErrorMessages = Readonly<Record<string, string>>

/** Normalize and validate the operator note before a request is sent. */
export function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw createControlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

/** Build standard Control headers while preserving caller-owned retry keys. */
export function buildControlMutationHeaders(
  options: ControlMutationOptions,
  defaultIdempotencyKey?: string,
): Record<string, string> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? defaultIdempotencyKey ?? newRequestId(),
    operatorReason,
  })
}

/** Routine writes allow an empty reason but preserve caller-owned retry keys. */
export function buildRoutineControlMutationHeaders(
  options: Partial<ControlMutationOptions> = {},
  defaultIdempotencyKey?: string,
): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? defaultIdempotencyKey ?? newRequestId(),
    operatorReason: options.operatorReason?.trim() || undefined,
  })
}

/** Read the machine code from either an envelope error or an HTTP-like error. */
export function readControlErrorCode(error: unknown): string | number | undefined {
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

export function readControlRequestId(error: unknown): string | undefined {
  if (error && typeof error === 'object') {
    const candidate = error as { requestId?: unknown; request_id?: unknown }
    if (typeof candidate.requestId === 'string' && candidate.requestId) return candidate.requestId
    if (typeof candidate.request_id === 'string' && candidate.request_id) {
      return candidate.request_id
    }
  }
  return undefined
}

export function createControlError(
  code: string,
  message: string,
  requestId?: string | undefined,
): ControlError {
  const error = new Error(message) as ControlError
  error.code = code
  if (requestId) error.requestId = requestId
  return error
}

/** Map machine codes to domain-owned copy without coupling this helper to a resource. */
export function mapControlError(
  error: unknown,
  fallback: string,
  messages: ControlErrorMessages = {},
): ControlError {
  const code = readControlErrorCode(error)
  const message = code === undefined ? undefined : messages[String(code)]
  if (message) {
    return createControlError(String(code), message, readControlRequestId(error))
  }
  if (error instanceof Error) return error as ControlError
  return createControlError('control_failed', fallback, readControlRequestId(error))
}
