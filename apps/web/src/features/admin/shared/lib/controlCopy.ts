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
import {
  createControlError,
  mapControlError,
  readControlErrorCode,
  readControlRequestId,
  type ControlErrorMessages,
} from './controlMutation'

export type { ControlErrorMessages } from './controlMutation'

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
  if (code === 'control_reason_required') {
    return createControlError(code, '需要填写操作原因', readControlRequestId(error))
  }
  return mapControlError(error, fallback, domainMessages)
}

export function withSimpleControlCopy(error: unknown, fallback: string): Error {
  const code = readSimpleControlErrorCode(error)
  if (code === 'idempotency_conflict' || code === 409) {
    return createControlError(
      code === 409 ? 'idempotency_conflict' : code,
      '操作已提交，请勿重复操作',
      readControlRequestId(error),
    )
  }
  if (code === 'control_reason_required') {
    return createControlError(code, '需要填写操作原因', readControlRequestId(error))
  }
  return mapControlError(error, fallback)
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
