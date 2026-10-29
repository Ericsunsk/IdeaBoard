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
//
// Quote mapping validates the service and leaves submit-specific media checks
// to submission. Transport validation errors propagate to the caller; only
// billing and access gates become readiness results.

import type { ApiClientError } from '../../../../../api/client'
import { quoteWorkspaceTask } from '../../../../../api/workspaceTaskTransport'
import type { Service } from '../../../../../core/contracts/imageServices'
import { readinessErrorCopy } from '../../../../../core/errors/errorCatalog'
import type {
  CanvasTaskRequestData,
  SubmitTaskReadinessData,
} from '../../../core/execution/canvasTaskRequest'
import { insufficientCreditsPreflightError } from '../task-runner/canvasBillingPreflight'
import { toWorkspaceQuoteInput } from './canvasTaskContractMapping'
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
  throw error
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
