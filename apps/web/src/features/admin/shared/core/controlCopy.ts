// Canonical Control Plane error copy for admin resources.
//
// `withControlCopy` is the converged form of the byte-identical locals
// previously copy-pasted across rewards, prompts, search-provider, and
// gateway resources: branch on stable control error codes, never on message
// text; `request_id` flows through for correlation.
//
// Resources with their own `control_not_found` / domain copy (billable
// catalog, payment orders, mail config, users, storage) keep a local variant
// on purpose — do not force them through these two.

export type ControlError = Error & {
  code?: string | undefined
  requestId?: string | undefined
}

export type ControlErrorMessages = Readonly<Record<string, string>>

/** Read the machine code from either an envelope error or an HTTP-like error. */
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
function mapControlError(
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

export function withControlCopy(
  error: unknown,
  fallback: string,
  domainMessages: ControlErrorMessages = {},
): Error {
  const code = readControlErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return createControlError(
      'idempotency_conflict',
      '操作已提交，请勿重复操作',
      readControlRequestId(error),
    )
  }
  return mapControlError(error, fallback, domainMessages)
}
