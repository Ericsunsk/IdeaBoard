// Production Board SSE-to-Query mapping (ADR-0014 §4).
//
// SSE carries invalidation signals only, never snapshot transport: event
// frames may contain an untrusted `document` field and it is never used as
// snapshot data. This module never calls `setQueryData` on document
// content — not even on `document_replaced`.
//
// Mapping:
// - `board.document_replaced` → invalidate the document, coalesced to one
//   refetch per 500ms window. Frames at or below the observed revision are
//   dropped (reorder/duplicate protection). Draft-dirty boards are marked
//   stale without refetching, so the draft under the cursor is never
//   overwritten.
// - `board.member_changed` → invalidate the list only.
// - `board.share_revoked` → invalidate the list, remove the document, and
//   when the revoked board is still in the local catalog (the gallery shows
//   a stale entry until the next hydrate), request exactly one hydrate
//   convergence. The convergence reads over HTTP and never emits SSE frames,
//   so it cannot re-trigger this handler; duplicate frames while a
//   convergence is in flight coalesce into that single run.
// - stream gap → invalidate only `['boards', workspaceId]`; other
//   workspaces and task caches stay untouched.

import type { QueryClient } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { subscribeWorkspaceEvent } from '../../../../api/workspaceEventStream'
import { getBoardById } from '../catalog/boards'
import { getBoardScope, subscribeBoardScopeChange } from '../catalog/scope'
import type { WorkspaceBoardDocument } from '../adapters/boardsResource'
import { boardQueryKeys } from './boardQueryKeys'
import { isBoardDraftDirty } from './boardDraftState'

export const BOARD_DOCUMENT_REPLACE_COALESCE_MS = 500

export type BoardShareRevokedConvergence = {
  /** Whether the revoked board is currently shown (defaults to the local catalog). */
  isBoardVisible?: ((boardId: string) => boolean) | undefined
  /**
   * Caller-owned hydrate convergence for a visible revoked board. The
   * production observer supplies its catalog hydrate; without it, ordinary
   * list invalidation converges active Query observers. A promise coalesces
   * the in-flight run instead of stacking.
   */
  requestCatalogHydrate?: (() => void | Promise<unknown>) | undefined
}

function isThenable(value: unknown): value is Promise<unknown> {
  return (
    !!value &&
    (typeof value === 'object' || typeof value === 'function') &&
    typeof (value as { then?: unknown }).then === 'function'
  )
}

export function subscribeBoardQueryInvalidation(
  client: QueryClient,
  workspaceId: string,
  isDraftDirty: (boardId: string) => boolean = () => false,
  convergence: BoardShareRevokedConvergence = {},
): () => void {
  const pending = new Map<string, number>()
  const observedRevisions = new Map<string, number>()
  const hydrateInflight = new Set<string>()
  const isBoardVisible =
    convergence.isBoardVisible ?? ((boardId: string) => getBoardById(boardId) !== null)
  const requestCatalogHydrate = convergence.requestCatalogHydrate
  let timer: ReturnType<typeof setTimeout> | undefined
  const flush = () => {
    timer = undefined
    for (const [boardId, revision] of pending) {
      observedRevisions.set(boardId, revision)
      const queryKey = boardQueryKeys.document(workspaceId, boardId)
      if (isDraftDirty(boardId)) {
        // Mark stale without refetching: an active observer keeps
        // rendering the draft instead of replacing it mid-edit.
        client.getQueryCache().find({ queryKey, exact: true })?.invalidate()
      } else {
        void client.invalidateQueries({ queryKey, exact: true })
      }
    }
    pending.clear()
  }
  function requestShareRevokedConvergence(boardId: string): void {
    if (!requestCatalogHydrate) return
    if (hydrateInflight.has(boardId)) return
    let isShown: boolean
    try {
      isShown = isBoardVisible(boardId)
    } catch {
      return
    }
    if (!isShown) return
    hydrateInflight.add(boardId)
    let result: void | Promise<unknown>
    try {
      result = requestCatalogHydrate()
    } catch {
      hydrateInflight.delete(boardId)
      return
    }
    if (isThenable(result)) {
      result.then(
        () => hydrateInflight.delete(boardId),
        () => hydrateInflight.delete(boardId),
      )
    } else {
      hydrateInflight.delete(boardId)
    }
  }
  const stop = subscribeWorkspaceEvent(
    workspaceId,
    (envelope) => {
      const boardId = envelope.resource.id
      if (envelope.type === 'board.document_replaced') {
        const revision = Number(envelope.data.revision)
        const cached = client.getQueryData<WorkspaceBoardDocument>(
          boardQueryKeys.document(workspaceId, boardId),
        )
        const knownRevision = Math.max(
          cached?.revision ?? 0,
          observedRevisions.get(boardId) ?? 0,
          pending.get(boardId) ?? 0,
        )
        if (!Number.isFinite(revision) || revision <= knownRevision) return
        pending.set(boardId, revision)
        if (timer === undefined) {
          timer = setTimeout(flush, BOARD_DOCUMENT_REPLACE_COALESCE_MS)
        }
      } else if (envelope.type === 'board.member_changed') {
        void client.invalidateQueries({ queryKey: boardQueryKeys.list(workspaceId), exact: true })
      } else if (envelope.type === 'board.share_revoked') {
        pending.delete(boardId)
        void client.invalidateQueries({ queryKey: boardQueryKeys.list(workspaceId), exact: true })
        client.removeQueries({
          queryKey: boardQueryKeys.document(workspaceId, boardId),
          exact: true,
        })
        requestShareRevokedConvergence(boardId)
      }
    },
    {
      types: ['board.document_replaced', 'board.member_changed', 'board.share_revoked'],
      onGap: () => {
        void client.invalidateQueries({ queryKey: boardQueryKeys.scopeRoot(workspaceId) })
      },
    },
  )
  return () => {
    stop()
    if (timer !== undefined) clearTimeout(timer)
    pending.clear()
    hydrateInflight.clear()
  }
}

/**
 * Production subscription: follows the workspace selection, resubscribing
 * on scope change and staying silent without a selected workspace. Boards
 * with unsaved content or a live session edit are draft-dirty. Unknown
 * cold-storage baselines remain conservative until a server read proves
 * their content; hydrated and successfully saved documents can refetch.
 */
export function subscribeBoardQueryScopeInvalidation(
  options: {
    client?: QueryClient
    isDraftDirty?: (boardId: string) => boolean
    convergence?: BoardShareRevokedConvergence
  } = {},
): () => void {
  const client = options.client ?? queryClient
  const isDraftDirty = options.isDraftDirty ?? isBoardDraftDirty
  const convergence = options.convergence
  let stop = () => {}
  const resubscribe = () => {
    stop()
    const scope = getBoardScope()
    if (!scope.workspaceId) {
      stop = () => {}
      return
    }
    stop = subscribeBoardQueryInvalidation(client, scope.workspaceId, isDraftDirty, convergence)
  }
  const stopScope = subscribeBoardScopeChange(resubscribe)
  resubscribe()
  return () => {
    stopScope()
    stop()
  }
}
