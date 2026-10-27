// Canvas task quote/readiness seam (issue #859).
//
// Readiness mapping split out of the workspace task port:
// `CanvasTaskRequestData` -> task-quotes -> `SubmitTaskReadinessData`.
// Quotes are price-only: no wallet balances ride the snapshot, so
// `available_credits`/`reserved_credits` map to 0 and sufficiency is
// inferred from the absence of 402. There is no v1 matrix endpoint;
// matrix fans out to one quote per request. No SSE, no cancel, no Vue.
//
// #917: quote enforces the same stable account and consumption
// eligibility as submit (Board generate gate via the bound board_id,
// 402 insufficient vs 403 restricted). Readiness keeps them distinct:
// shortfall throws the insufficient-credits preflight (top-up call to
// action), restriction and Board denials read not-ready with their
// codes.

import type { ApiClientError } from '../../../../../api/client'
import { quoteWorkspaceTask } from '../../../../../api/workspaceTaskTransport'
import type { Service } from '../../../../../core/contracts/imageServices'
import { i18n } from '../../../../../core/i18n'
import { readinessErrorCopy } from '../../../../../core/errors/errorCatalog'
import type {
  CanvasTaskRequestData,
  SubmitTaskReadinessData,
} from '../../../core/execution/canvasTaskRequest'
import { insufficientCreditsPreflightError } from '../task-runner/canvasBillingPreflight'
import {
  errorStatus,
  isCanvasWorkspaceTaskUnmappableError,
  toWorkspaceCreateInput,
  toWorkspaceQuoteInput,
} from './canvasTaskContractMapping'
import { requireCanvasWorkspaceId } from './canvasTaskCancellation'

export function toSubmitTaskReadiness(
  snapshot: {
    unit_credits: number
    total_credits: number
    price_version_id: number
    price_version: number
    quantity: number
  },
  service: Service,
): SubmitTaskReadinessData {
  // Quote snapshots carry no wallet balances: sufficiency is inferred from
  // the absence of 402 (a 402 quote throws insufficient-credits instead of
  // returning data). available/reserved map to 0; quote consumers render
  // unit/total only.
  return {
    ready: true,
    service,
    billing: {
      sufficient: true,
      unit_credits: snapshot.unit_credits,
      total_credits: snapshot.total_credits,
      available_credits: 0,
      reserved_credits: 0,
      price_version_id: snapshot.price_version_id,
      price_version: snapshot.price_version,
      quantity: snapshot.quantity,
    },
  }
}

function toNotReady(service: Service, message: string, errorCode: string): SubmitTaskReadinessData {
  return { ready: false, message, error_code: errorCode, service }
}

/**
 * Submit/quote 口径预检 (#P1-1): quote 阶段即执行与 submit 相同的
 * `toWorkspaceCreateInput` fail-closed 校验（image_urls、numeric ids、
 * storage URL、data-URL mask），不通过直接标 not-ready，不等 submit 才
 * toast。inpaint 缺 mask 走服务端 422，但完整 submit 意图（携带
 * use_mask/mask 键或媒体输入）无 mask_asset_id 时同样在 quote 阶段以
 * 明确指引标 not-ready，避免闪现 pending 节点再清除。纯价格探针（无
 * mask 上下文的泛 service 询价）不受 mask 检查影响。
 */
function quoteMediaPreflightNotReady(
  request: CanvasTaskRequestData,
): SubmitTaskReadinessData | null {
  try {
    toWorkspaceCreateInput(request)
  } catch (error) {
    if (!isCanvasWorkspaceTaskUnmappableError(error)) throw error
    // 未知 service 是编程错误，仍由 `toWorkspaceQuoteInput` fail-closed
    // 抛出；其余 fail-closed（media-urls/media-ids/params/board）在
    // quote 阶段即标 not-ready（createInput 先校验 service，能走到
    // 这里的 service 必为合法 slug）。
    if (error.reason === 'service') throw error
    return toNotReady(request.service as Service, error.message, 'validation_failed')
  }
  const service = request.service as Service
  if (service === 'inpaint') {
    const params = request.params ?? {}
    const hasInpaintIntent =
      'mask_asset_id' in params ||
      'use_mask' in params ||
      'mask_image_url' in params ||
      'mask_image_urls' in params ||
      (Array.isArray(request.media_asset_ids) && request.media_asset_ids.length > 0)
    if (hasInpaintIntent && !`${params.mask_asset_id ?? ''}`.trim()) {
      return toNotReady(
        service,
        i18n.global.t('canvas.execution.markInpaintAreaFirst'),
        'validation_failed',
      )
    }
  }
  return null
}

/**
 * Quote 传输错误 -> readiness 映射（与 submit 门控同码表）：
 * 返回 not-ready 数据，或对非映射错误直接抛出。
 */
function mapQuoteTransportError(service: Service, error: unknown): SubmitTaskReadinessData {
  const apiError = error as ApiClientError
  const code = apiError?.code
  if (code === 'task_insufficient_credits') throw insufficientCreditsPreflightError()
  // Funds restricted vs balance shortfall stay distinct (#917): a
  // restricted Wallet is not-ready (no top-up call to action), while a
  // shortfall throws the insufficient-credits preflight above.
  if (code === 'billing_wallet_restricted') {
    return toNotReady(service, readinessErrorCopy('wallet_restricted'), 'wallet_restricted')
  }
  // Board generate gate mirrors submit (#917): Viewers read not-ready
  // instead of throwing, archived/invisible Boards keep their codes.
  if (
    code === 'task_forbidden' ||
    code === 'board_archived' ||
    code === 'board_not_found' ||
    code === 'service_resolution_unavailable'
  ) {
    return toNotReady(service, readinessErrorCopy(code), code)
  }
  if (errorStatus(error) === 422 || code === 'validation_failed') {
    const message =
      error instanceof Error && error.message
        ? error.message
        : i18n.global.t('common.errors.billingInfoUnavailable')
    return toNotReady(service, message, 'validation_failed')
  }
  throw error
}

export async function fetchCanvasWorkspaceTaskReadiness(
  request: CanvasTaskRequestData,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<SubmitTaskReadinessData> {
  const workspaceId = requireCanvasWorkspaceId()
  // 口径预检先于网络：与 submit 同规则的媒体 fail-closed 校验在 quote
  // 阶段即标 not-ready（无 task、无 hold、无网络调用）。
  const preflight = quoteMediaPreflightNotReady(request)
  if (preflight) return preflight
  const input = toWorkspaceQuoteInput(request)
  try {
    const snapshot = await quoteWorkspaceTask(
      workspaceId,
      input,
      options?.signal == null ? undefined : { signal: options.signal },
    )
    return toSubmitTaskReadiness(snapshot, input.service)
  } catch (error) {
    return mapQuoteTransportError(input.service, error)
  }
}

export async function fetchCanvasWorkspaceTaskReadinessMatrix(
  requests: readonly CanvasTaskRequestData[],
  options?: { signal?: AbortSignal | null | undefined },
): Promise<SubmitTaskReadinessData[]> {
  if (requests.length === 0) return []
  // No v1 matrix endpoint: one quote per request, order-preserved, first
  // rejection wins (matches the legacy matrix throw-on-transport-failure).
  return Promise.all(requests.map((request) => fetchCanvasWorkspaceTaskReadiness(request, options)))
}
