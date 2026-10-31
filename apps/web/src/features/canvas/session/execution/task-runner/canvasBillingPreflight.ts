import type { ApiClientError } from '@/api/http/client'
import { i18n } from '../../../../../core/i18n'

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
  if (code === 'task_insufficient_credits') return insufficientCreditsPreflightError()
  if (code === 'billing_quote_unavailable') {
    return new CanvasBillingPreflightError(
      'pricing-unavailable',
      i18n.global.t('errorCodes.pricing_unavailable'),
    )
  }
  // Empty Service Model Pool at submit (HTTP 422): the backend guarantees
  // no task and no billing hold on this path, so the client maps it to a
  // clear user toast; this deterministic failure is not retryable.
  if (code === 'service_resolution_unavailable') {
    return new CanvasBillingPreflightError(
      'resolution-unavailable',
      i18n.global.t('errorCodes.service_resolution_unavailable'),
    )
  }
  return null
}

export function mapCanvasBillingRequestError(error: unknown): Error {
  if (error instanceof CanvasBillingPreflightError) return error
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

/**
 * 可重试判定 (#P1-4): 与 submit 共用重试预算（单次重试）。
 * - `network`（传输未完成）与 `pricing-unavailable`（503
 *   billing_quote_unavailable 瞬态）可重试；
 * - `resolution-unavailable`（空 Service Model Pool，422 确定性无 hold）
 *   与 `insufficient-credits` 不可重试；
 * - 未映射的原始传输错误仅 503/429 可重试；422 属于确定性校验失败，
 *   其余错误直接冒泡（调用方保留手动重提入口）。
 */
export function isRetryableCanvasTaskRequestError(error: unknown): boolean {
  if (error instanceof CanvasBillingPreflightError) {
    return error.kind === 'network' || error.kind === 'pricing-unavailable'
  }
  const status = (error as { status?: unknown } | null)?.status
  return status === 503 || status === 429
}
