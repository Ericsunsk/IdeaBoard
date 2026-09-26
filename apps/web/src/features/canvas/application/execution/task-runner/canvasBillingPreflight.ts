import type { ApiClientError } from '../../../../../shared/api/client'
import { i18n } from '../../../../../shared/i18n'

export type CanvasBillingPreflightFailureKind =
  'insufficient-credits' | 'network' | 'pricing-unavailable'

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

export function mapCanvasBillingRequestError(error: unknown): Error {
  const apiError = error as ApiClientError
  // Legacy user-plane codes (`insufficient_credits`, `pricing_unavailable`)
  // plus workspace v1 codes (`task_insufficient_credits` 402,
  // `billing_quote_unavailable` 503). Both transports converge here so the
  // feedback boundary keeps one copy per kind.
  if (apiError?.code === 'insufficient_credits' || apiError?.code === 'task_insufficient_credits')
    return insufficientCreditsPreflightError()
  if (apiError?.code === 'pricing_unavailable' || apiError?.code === 'billing_quote_unavailable') {
    return new CanvasBillingPreflightError(
      'pricing-unavailable',
      i18n.global.t('errorCodes.pricing_unavailable'),
    )
  }
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
