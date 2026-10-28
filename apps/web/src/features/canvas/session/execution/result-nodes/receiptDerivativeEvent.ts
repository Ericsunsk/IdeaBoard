// Workspace hints prompt authoritative metadata reads; polling remains the bounded fallback.

import { getWorkspaceMediaAssetMetadata } from '../../../../../api/mediaAssetDelivery'
import { subscribeWorkspaceMediaInvalidation } from '../../../../../api/workspaceEventStream'
import { awaitReceiptDerivativeReady, RECEIPT_DERIVATIVE_BUDGET_MS } from './resultDisplay'

// Bypass cached pending metadata when confirming a receipt or a hint.
async function confirmReceiptDerivativeReady(
  workspaceId: string,
  assetId: string,
  signal?: AbortSignal,
): Promise<boolean> {
  const metadata = await getWorkspaceMediaAssetMetadata(workspaceId, assetId, {
    signal,
    force: true,
  })
  const status = metadata.derivatives.preview?.status ?? metadata.status
  return status === 'ready' || status === 'failed'
}

// Consume the shared latch; timeout, transport failure and abort wake the poll fallback.
function waitForLatchedHint(
  state: { isHinted: boolean; isTransportFailed: boolean; notify: (() => void) | null },
  timeoutMs: number,
  signal?: AbortSignal,
): Promise<boolean> {
  return new Promise((resolve) => {
    if (state.isTransportFailed) {
      resolve(false)
      return
    }
    if (state.isHinted) {
      state.isHinted = false
      resolve(true)
      return
    }
    const timer = globalThis.setTimeout(() => settle(false), Math.max(0, timeoutMs))
    const settle = (isHinted: boolean) => {
      globalThis.clearTimeout(timer)
      signal?.removeEventListener('abort', onAbort)
      state.notify = null
      if (isHinted) state.isHinted = false
      resolve(isHinted)
    }
    const onAbort = () => settle(false)
    if (signal?.aborted === true) {
      settle(false)
      return
    }
    signal?.addEventListener('abort', onAbort, { once: true })
    state.notify = () => settle(!state.isTransportFailed)
  })
}

/**
 * Subscribe before confirming so hints racing a GET are latched. Every hint
 * re-confirms over REST, including when cache invalidation aborts that GET.
 * Hint waiting and polling share one budget; abort propagates to the caller.
 */
export async function waitForReceiptDerivativeReady(
  workspaceId: string,
  assetId: string,
  signal?: AbortSignal,
  timeoutMs: number = RECEIPT_DERIVATIVE_BUDGET_MS,
): Promise<void> {
  signal?.throwIfAborted()
  const startedAt = Date.now()
  const remainingMs = () => Math.max(0, timeoutMs) - (Date.now() - startedAt)
  const hintState = {
    isHinted: false,
    isTransportFailed: false,
    notify: null as (() => void) | null,
  }
  let stopHint: () => void
  try {
    stopHint = subscribeWorkspaceMediaInvalidation(
      workspaceId,
      (invalidatedId) => {
        if (invalidatedId !== assetId) return
        hintState.isHinted = true
        hintState.notify?.()
      },
      // Use polling rather than waiting through stream reconnection.
      () => {
        hintState.isTransportFailed = true
        hintState.notify?.()
      },
    )
  } catch {
    // No event transport: use the existing bounded poll.
    await awaitReceiptDerivativeReady(workspaceId, assetId, signal, remainingMs())
    return
  }
  try {
    for (;;) {
      try {
        if (await confirmReceiptDerivativeReady(workspaceId, assetId, signal)) return
      } catch (error) {
        signal?.throwIfAborted()
        if (hintState.isHinted) {
          hintState.isHinted = false
          continue
        }
        console.warn('[canvas] failed to confirm receipt derivative readiness', {
          assetId,
          operation: 'confirmReceiptDerivativeReady',
          error,
        })
        break
      }
      const remaining = remainingMs()
      if (remaining <= 0) break
      if (!(await waitForLatchedHint(hintState, remaining, signal))) break
    }
  } finally {
    try {
      stopHint()
    } catch {
      // Unsubscribe is best-effort; the bounded poll fallback owns readiness.
    }
  }
  await awaitReceiptDerivativeReady(workspaceId, assetId, signal, remainingMs())
}
