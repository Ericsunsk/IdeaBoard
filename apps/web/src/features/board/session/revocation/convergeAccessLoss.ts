import type { QueryClient } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { invalidateWorkspaceMediaAssetMetadata } from '@/api/media/mediaAssetMetadata'
import { deleteBoard } from '../catalog/boards'
import { getBoardScope, isBoardScopeCurrent, subscribeBoardScopeChange } from '../catalog/scope'
import {
  subscribeWorkspaceEvent,
  type WorkspaceEventEnvelope,
} from '@/api/realtime/workspaceEventStream'
import { deleteBoardState } from '../persistence/state'
import { clearPendingBoardSyncRollback } from '../sync/boardSync'
import { forgetBoardServerContent } from '../sync/boardDraftState'
import { boardQueryKeys } from '../sync/boardQueryKeys'
import {
  markBoardAccessLost,
  markBoardAccessStale,
  revalidateBoardAccess,
  type RevalidateBoardAccessOptions,
  type BoardAccessRevalidation,
} from './accessLoss'

// Confirmed access-loss convergence (#941): what happens AFTER
// `revalidateBoardAccess` (or a directed `board.access_changed` verdict)
// proves the loss. Unconfirmed errors NEVER reach this module — ordinary
// document GET/PUT 404s, network failures, and incomplete catalogs keep
// the draft and the retry loops (see `accessLoss.ts`).
//
// Convergence drops, never copies: the unsaved draft is discarded with
// the protected queries, the in-memory catalog entry, the persisted local
// board state, the server-content baseline, pending rollback markers, and
// the workspace display-lease cache. Surfaces subscribe through
// `subscribeBoardAccessLost` for UI follow-ups (close Share, toast,
// navigate to an authorized Gallery); this module performs no navigation
// itself so Gallery and Canvas share one path.

export type BoardAccessLostListener = (boardId: string) => void

const accessLostListeners = new Set<BoardAccessLostListener>()

/** Surfaces observe confirmed losses (close Share, toast, navigate). */
export function subscribeBoardAccessLost(listener: BoardAccessLostListener): () => void {
  accessLostListeners.add(listener)
  return () => {
    accessLostListeners.delete(listener)
  }
}

function publishBoardAccessLost(boardId: string): void {
  for (const listener of [...accessLostListeners]) {
    try {
      listener(boardId)
    } catch {
      // One surface must never break another's convergence.
    }
  }
}

type ConvergeAccessLossDeps = {
  client?: QueryClient | undefined
}

/**
 * Converges one confirmed board loss under the current scope: marks the
 * registry (autosave/generation/retry gates stop immediately), removes
 * the protected document/access queries, invalidates the catalog
 * list, drops the local catalog entry + persisted board state +
 * server-content baseline + rollback markers, releases the workspace
 * display-lease cache, and publishes to subscribed surfaces.
 */
function convergeConfirmedAccessLoss(
  boardId: string,
  options: { workspaceId?: string | null | undefined } & ConvergeAccessLossDeps = {},
): void {
  const normalized = boardId.trim()
  if (!normalized) return
  const scope = getBoardScope()
  const workspaceId = (options.workspaceId ?? scope.workspaceId ?? '').trim()
  if (!scope.userScope || !workspaceId || workspaceId !== scope.workspaceId) return
  markBoardAccessLost(normalized)
  const client = options.client ?? queryClient
  client.removeQueries({ queryKey: boardQueryKeys.document(workspaceId, normalized), exact: true })
  client.removeQueries({ queryKey: boardQueryKeys.access(workspaceId, normalized), exact: true })
  void client.invalidateQueries({ queryKey: boardQueryKeys.list(workspaceId), exact: true })
  deleteBoard(normalized)
  deleteBoardState(normalized)
  forgetBoardServerContent(normalized)
  clearPendingBoardSyncRollback(normalized)
  invalidateWorkspaceMediaAssetMetadata(workspaceId)
  publishBoardAccessLost(normalized)
}

export type RevalidateAndConvergeBoardAccessOptions = RevalidateBoardAccessOptions &
  ConvergeAccessLossDeps & {
    isCurrent?: (() => boolean) | undefined
    revalidate?: typeof revalidateBoardAccess | undefined
  }

/** Revalidation may discard protected state only while its captured residency is current. */
export async function revalidateAndConvergeBoardAccess(
  boardId: string,
  options: RevalidateAndConvergeBoardAccessOptions = {},
): Promise<BoardAccessRevalidation> {
  const scope = getBoardScope()
  if (options.isCurrent && !options.isCurrent()) return 'preserved'
  const revalidate = options.revalidate ?? revalidateBoardAccess
  const verdict = await revalidate(boardId, {
    workspaceId: options.workspaceId,
    fetchMeta: options.fetchMeta,
  })
  if (!isBoardScopeCurrent(scope) || (options.isCurrent && !options.isCurrent())) return 'preserved'
  if (verdict === 'revoked') convergeConfirmedAccessLoss(boardId, options)
  return verdict
}

export type OpenBoardAccessLossObserver = {
  /** Currently open board id, or null when no board is open. */
  getBoardId: () => string | null
  /** Revalidation seam (specs only). */
  revalidate?: typeof revalidateBoardAccess | undefined
}

/**
 * Open-board loss observation (#941): directed `board.access_changed`
 * frames for the open board, SSE gap notices, reconnects, and hidden-tab recoveries all
 * revalidate the open board — never converge blindly. Stale frames for
 * other boards are ignored; a scope switch resubscribes and drops
 * in-flight verdicts (late responses never converge the new scope).
 */
export function observeOpenBoardAccessLoss(observer: OpenBoardAccessLossObserver): () => void {
  let isDisposed = false
  let revalidating: string | null = null
  const scopeAtSubscribe = getBoardScope()

  function isCurrent(): boolean {
    if (isDisposed) return false
    return isBoardScopeCurrent(scopeAtSubscribe)
  }

  async function revalidateOpenBoard(): Promise<void> {
    const boardId = observer.getBoardId()?.trim()
    if (!boardId || revalidating === boardId) return
    revalidating = boardId
    try {
      await revalidateAndConvergeBoardAccess(boardId, {
        revalidate: observer.revalidate,
        isCurrent: () => isCurrent() && observer.getBoardId()?.trim() === boardId,
      })
    } catch {
      // Revalidation never throws, but a custom seam might: preserve.
    } finally {
      if (revalidating === boardId) revalidating = null
    }
  }

  function handleEnvelope(envelope: WorkspaceEventEnvelope): void {
    if (!isCurrent()) return
    const boardId = observer.getBoardId()?.trim()
    if (!boardId) return
    if (envelope.resource.id !== boardId) return
    if (envelope.type === 'board.access_changed') {
      void revalidateOpenBoard()
    }
  }

  function handleVisible(): void {
    if (typeof document === 'undefined') return
    if (document.visibilityState !== 'visible') return
    const boardId = observer.getBoardId()?.trim()
    if (!boardId) return
    markBoardAccessStale(boardId)
    void revalidateOpenBoard()
  }

  const workspaceId = scopeAtSubscribe.workspaceId
  let stopEvents = () => {}
  let stopScope = () => {}
  if (workspaceId) {
    stopEvents = subscribeWorkspaceEvent(workspaceId, handleEnvelope, {
      types: ['board.access_changed'],
      onGap: () => {
        const boardId = observer.getBoardId()?.trim()
        if (!boardId) return
        markBoardAccessStale(boardId)
        void revalidateOpenBoard()
      },
      onError: () => {
        // Transport errors reconnect with backoff behind this seam; the
        // resumed stream replays missed frames, and the gap notice above
        // covers evicted cursors. Flag stale so the next sensitive op
        // revalidates even when no gap arrives.
        const boardId = observer.getBoardId()?.trim()
        if (boardId) markBoardAccessStale(boardId)
      },
    })
    stopScope = subscribeBoardScopeChange(() => {
      stopEvents()
      stopEvents = () => {}
    })
  }
  if (typeof document !== 'undefined' && typeof document.addEventListener === 'function') {
    document.addEventListener('visibilitychange', handleVisible)
  }
  return () => {
    isDisposed = true
    stopEvents()
    stopScope()
    if (typeof document !== 'undefined' && typeof document.removeEventListener === 'function') {
      document.removeEventListener('visibilitychange', handleVisible)
    }
  }
}
