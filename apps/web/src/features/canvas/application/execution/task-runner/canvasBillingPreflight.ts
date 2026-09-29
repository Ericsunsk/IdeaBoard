import type { ApiClientError } from '../../../../../shared/api/client'
import { i18n } from '../../../../../shared/i18n'

export type CanvasBillingPreflightFailureKind =
  'insufficient-credits' | 'network' | 'pricing-unavailable' | 'resolution-unavailable'

export class CanvasBillingPreflightError extends Error {
  readonly kind: CanvasBillingPreflightFailureKind

  constructor(kind: CanvasBillingPreflightFailureKind, message: string) {
    super(message)
    this.name = 'CanvasBillingPreflightError'
    this.kind = kind
  }
}

export function insufficientCreditsPreflightError(): CanvasBillingPreflightError {
  return new CanvasBillingPreflightError(
    'insufficient-credits',
    i18n.global.t('errorCodes.insufficient_credits'),
  )
}

function preflightErrorForCode(code: unknown): CanvasBillingPreflightError | null {
  // Legacy user-plane codes (`insufficient_credits`, `pricing_unavailable`)
  // plus workspace v1 codes (`task_insufficient_credits` 402,
  // `billing_quote_unavailable` 503, `service_resolution_unavailable` 422).
  // Both transports converge here so the feedback boundary keeps one copy
  // per kind.
  if (code === 'insufficient_credits' || code === 'task_insufficient_credits')
    return insufficientCreditsPreflightError()
  if (code === 'pricing_unavailable' || code === 'billing_quote_unavailable') {
    return new CanvasBillingPreflightError(
      'pricing-unavailable',
      i18n.global.t('errorCodes.pricing_unavailable'),
    )
  }
  // Empty Service Model Pool at submit (HTTP 422): the backend guarantees
  // no task and no billing hold on this path, so the client maps it to a
  // clear user toast and performs no retry (only `network` retries).
  if (code === 'service_resolution_unavailable') {
    return new CanvasBillingPreflightError(
      'resolution-unavailable',
      i18n.global.t('errorCodes.service_resolution_unavailable'),
    )
  }
  return null
}

export function mapCanvasBillingRequestError(error: unknown): Error {
  const mapped = preflightErrorForCode((error as ApiClientError)?.code)
  if (mapped) return mapped
  const apiError = error as ApiClientError
  if (apiError?.status == null && apiError?.code == null) {
    return new CanvasBillingPreflightError(
      'network',
      i18n.global.t('common.errors.requestIncomplete'),
    )
  }
  return error instanceof Error
    ? error
    : new Error(i18n.global.t('common.errors.requestIncomplete'))
}

export function isCanvasBillingPreflightError(
  error: unknown,
  kind?: CanvasBillingPreflightFailureKind,
): error is CanvasBillingPreflightError {
  return error instanceof CanvasBillingPreflightError && (kind === undefined || error.kind === kind)
}
