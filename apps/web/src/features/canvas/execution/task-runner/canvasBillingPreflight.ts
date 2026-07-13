import type { ApiClientError } from '../../../../shared/api/client'

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
  return new CanvasBillingPreflightError('insufficient-credits', 'Idea不足')
}

export function mapCanvasBillingRequestError(error: unknown): Error {
  const apiError = error as ApiClientError
  if (apiError?.code === 'insufficient_credits') return insufficientCreditsPreflightError()
  if (apiError?.code === 'pricing_unavailable') {
    return new CanvasBillingPreflightError('pricing-unavailable', '暂时无法获取报价，请重试')
  }
  if (apiError?.status == null && apiError?.code == null) {
    return new CanvasBillingPreflightError('network', '网络错误，请重试')
  }
  return error instanceof Error ? error : new Error('请求失败')
}

export function isCanvasBillingPreflightError(
  error: unknown,
  kind?: CanvasBillingPreflightFailureKind,
): error is CanvasBillingPreflightError {
  return error instanceof CanvasBillingPreflightError && (kind === undefined || error.kind === kind)
}
