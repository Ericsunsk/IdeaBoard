// Canonical Control Plane error copy for admin resources.
//
// `withControlCopy` is the converged form of the byte-identical locals
// previously copy-pasted across rewards, prompts, search-provider, and
// gateway resources: branch on stable control error codes, never on message
// text; `request_id` flows through for correlation.
//
// `withSimpleControlCopy` is the converged form of the byte-identical locals
// in outbound-policy, branding, stripe-settings, media-upload, and access
// settings: same copy, but those surfaces never carried `request_id`.
//
// Resources with their own `control_not_found` / domain copy (billable
// catalog, payment orders, mail config, users, storage) keep a local variant
// on purpose — do not force them through these two.
export function withControlCopy(error: unknown, fallback: string): Error {
  const code = readControlErrorCode(error)
  const requestId = readControlRequestId(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError('idempotency_conflict', '操作已提交，请勿重复操作', requestId)
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因', requestId)
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback, requestId)
}

export function withSimpleControlCopy(error: unknown, fallback: string): Error {
  const code = readSimpleControlErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return controlError(code === 409 ? 'idempotency_conflict' : code, '操作已提交，请勿重复操作')
  }
  if (code === 'control_reason_required') {
    return controlError(code, '需要填写操作原因')
  }
  if (error instanceof Error) return error
  return controlError('control_failed', fallback)
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

function readControlErrorCode(error: unknown): string | number | undefined {
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

function readSimpleControlErrorCode(error: unknown): string | number | undefined {
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

function readControlRequestId(error: unknown): string | undefined {
  if (error && typeof error === 'object') {
    const candidate = error as { requestId?: unknown; request_id?: unknown }
    if (typeof candidate.requestId === 'string' && candidate.requestId) return candidate.requestId
    if (typeof candidate.request_id === 'string' && candidate.request_id) {
      return candidate.request_id
    }
  }
  return undefined
}
