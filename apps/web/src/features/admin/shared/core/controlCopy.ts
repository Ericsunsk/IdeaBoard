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
  return mapControlError(error, fallback, domainMessages)
}
