// Canvas task quote/readiness seam (issue #859).
//
// Readiness mapping split out of the workspace task port:
// `CanvasTaskRequestData` -> task-quotes -> `SubmitTaskReadinessData`.
// Quotes are price-only: no wallet balances ride the snapshot, so
// `available_credits`/`reserved_credits` map to 0 and sufficiency is
// inferred from the absence of 402. There is no v1 matrix endpoint;
// matrix fans out to one quote per request. No SSE, no cancel, no Vue.

import type { ApiClientError } from '../../../../../shared/api/client'
import { quoteWorkspaceTask } from '../../../../../shared/api/workspaceTaskTransport'
import type { Service } from '../../../../../shared/domain/imageServices'
import { i18n } from '../../../../../shared/i18n'
import { readinessErrorCopy } from '../../../../../shared/errors/errorCatalog'
import type {
  CanvasTaskRequestData,
  SubmitTaskReadinessData,
} from '../../../domain/execution-contracts/canvasTaskRequest'
import { insufficientCreditsPreflightError } from '../task-runner/canvasBillingPreflight'
import { errorStatus, toWorkspaceQuoteInput } from './canvasTaskContractMapping'
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

export async function fetchCanvasWorkspaceTaskReadiness(
  request: CanvasTaskRequestData,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<SubmitTaskReadinessData> {
  const workspaceId = requireCanvasWorkspaceId()
  const input = toWorkspaceQuoteInput(request)
  try {
    const snapshot = await quoteWorkspaceTask(
      workspaceId,
      input,
      options?.signal == null ? undefined : { signal: options.signal },
    )
    return toSubmitTaskReadiness(snapshot, input.service)
  } catch (error) {
    const apiError = error as ApiClientError
    const code = apiError?.code
    if (code === 'task_insufficient_credits') throw insufficientCreditsPreflightError()
    if (code === 'service_resolution_unavailable') {
      return toNotReady(input.service, readinessErrorCopy(code), code)
    }
    if (errorStatus(error) === 422 || code === 'validation_failed') {
      const message =
        error instanceof Error && error.message
          ? error.message
          : i18n.global.t('common.errors.billingInfoUnavailable')
      return toNotReady(input.service, message, 'validation_failed')
    }
    throw error
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
