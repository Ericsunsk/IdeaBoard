// Production Board Query ownership (ADR-0014 §§1-2, 5).
//
// The catalog list rides `accountScoped()` (stale 15s); per-board documents
// ride the global singleton default (stale 5s). `gcTime` 10m, `retry 1` and
// `refetchOnWindowFocus: false` stay on the global `queryClient` defaults —
// this module never overrides them and never creates a second QueryClient.
//
// Write-back discipline: SSE never calls `setQueryData` on document content
// (it only invalidates; see `boardSseInvalidation.ts`). `setQueryData` here
// carries server-authored snapshots only — hydrate seeds from completed
// HTTP reads, save seeds from the PUT response, and create/rename apply
// local write-back of the minted/renamed meta. No optimistic document
// content is ever written.

import { QueryObserver, type QueryFunctionContext } from '@tanstack/vue-query'
import { computed, onScopeDispose, shallowRef, watch, type Ref } from 'vue'
import { queryClient } from '../../../../core/query/queryClient'
import { accountScoped } from '../../../../core/query/queryDefaults'
import {
  fetchWorkspaceBoardDocument,
  fetchWorkspaceBoards,
  type WorkspaceBoardDocument,
  type WorkspaceBoardMeta,
} from '../adapters/boardsResource'
import type { PreviewScene } from '../../core/preview/scene'
import { boardQueryKeys } from './boardQueryKeys'
import { getBoardScope, isCompleteBoardScope, subscribeBoardScopeChange } from '../catalog/scope'

/** Gallery scene projection carried inline on one catalog list item. */
export type BoardListSceneEntry = {
  contentVersion: string
  scene: PreviewScene | null
}

/** Reactive, list-only Gallery projection; Query owns every server snapshot. */
export function useBoardCatalogScenes(boards: Readonly<Ref<readonly { id: string }[]>>) {
  const scope = shallowRef(getBoardScope())
  const snapshot = shallowRef<WorkspaceBoardMeta[]>([])
  const stopScope = subscribeBoardScopeChange((next) => {
    scope.value = next
  })
  const stopQuery = watch(
    scope,
    (captured, _, onCleanup) => {
      snapshot.value = []
      if (!isCompleteBoardScope(captured)) return
      const observer = new QueryObserver(queryClient, {
        ...boardListOptions(captured.workspaceId),
        enabled: false,
      })
      onCleanup(observer.subscribe((result) => (snapshot.value = result.data ?? [])))
      snapshot.value = observer.getCurrentResult().data ?? []
    },
    { immediate: true, flush: 'sync' },
  )
  onScopeDispose(() => {
    stopScope()
    stopQuery()
  })
  const projection = computed(
    (previous?: {
      userScope: string | null
      workspaceId: string | null
      sceneMap: Record<string, BoardListSceneEntry>
    }) => {
      const captured = scope.value
      const workspaceId = isCompleteBoardScope(captured) ? captured.workspaceId : null
      const sceneMap: Record<string, BoardListSceneEntry> = {}
      if (workspaceId) {
        const visibleIds = new Set(boards.value.map((board) => board.id))
        const canReuse =
          previous?.userScope === captured.userScope && previous?.workspaceId === workspaceId
        for (const meta of snapshot.value) {
          if (!visibleIds.delete(meta.id)) continue
          const cached = canReuse ? previous.sceneMap[meta.id] : undefined
          sceneMap[meta.id] =
            cached?.contentVersion === meta.contentVersion &&
            (cached.scene === null) === (meta.previewScene === null)
              ? cached
              : { contentVersion: meta.contentVersion, scene: meta.previewScene }
        }
        for (const id of visibleIds) sceneMap[id] = { contentVersion: '', scene: null }
      }
      return { userScope: captured.userScope, workspaceId, sceneMap }
    },
  )
  return {
    sceneMap: computed(() => projection.value.sceneMap),
    workspaceId: computed(() => projection.value.workspaceId),
  }
}

/** Catalog list: server board metas, 15s freshness across navigation. */
export function boardListOptions(workspaceId: string) {
  return {
    queryKey: boardQueryKeys.list(workspaceId),
    queryFn: ({ signal }: QueryFunctionContext) => fetchWorkspaceBoards(workspaceId, { signal }),
    ...accountScoped(),
  }
}

/** Per-board server document snapshot; global default freshness (5s). */
export function boardDocumentOptions(workspaceId: string, boardId: string) {
  return {
    queryKey: boardQueryKeys.document(workspaceId, boardId),
    queryFn: ({ signal }: QueryFunctionContext) =>
      fetchWorkspaceBoardDocument(workspaceId, boardId, { signal }),
    // A successful older read must not roll the observed server revision
    // back: keep the previous snapshot when the response regresses.
    structuralSharing: (previous: unknown, next: unknown) => {
      const oldDocument = previous as WorkspaceBoardDocument | undefined
      const newDocument = next as WorkspaceBoardDocument
      return oldDocument && newDocument.revision < oldDocument.revision ? oldDocument : next
    },
  }
}

/** Observed server revision for `If-Match`; undefined when never read. */
export function getBoardDocumentRevision(workspaceId: string, boardId: string): number | undefined {
  const cached = queryClient.getQueryData<WorkspaceBoardDocument>(
    boardQueryKeys.document(workspaceId, boardId),
  )
  const revision = Number(cached?.revision)
  return Number.isFinite(revision) ? revision : undefined
}

/** Seed the catalog list from a completed cursor walk (hydrate only). */
export function seedBoardListQuery(workspaceId: string, metas: WorkspaceBoardMeta[]): void {
  queryClient.setQueryData(boardQueryKeys.list(workspaceId), metas)
}

/**
 * Seed one document from a server-authored snapshot (hydrate or the PUT
 * response). Revision-guarded: an older snapshot never regresses the
 * cached revision.
 */
export function seedBoardDocumentQuery(
  workspaceId: string,
  document: WorkspaceBoardDocument,
): void {
  queryClient.setQueryData<WorkspaceBoardDocument>(
    boardQueryKeys.document(workspaceId, document.boardId),
    (previous) => (previous && document.revision < previous.revision ? previous : document),
  )
}

/**
 * Create/rename/save local write-back of one list meta. Carries the
 * server-minted or server-advanced identity; never invents revisions.
 * A single board cannot establish a complete catalog on a cold cache.
 */
export function writeBoardListMeta(workspaceId: string, meta: WorkspaceBoardMeta): void {
  queryClient.setQueryData<WorkspaceBoardMeta[]>(boardQueryKeys.list(workspaceId), (previous) => {
    if (!previous) return undefined
    if (previous.some((entry) => entry.id === meta.id)) {
      return previous.map((entry) => (entry.id === meta.id ? meta : entry))
    }
    return [...previous, meta]
  })
}

/**
 * Adopt the server-advanced revision after an explicit rename (PATCH-only,
 * no document write): the list meta and, when cached, the document revision
 * advance together so the next document PUT keeps a fresh `If-Match`.
 * Document content is preserved, never overwritten.
 */
export function adoptBoardRenameRevision(
  workspaceId: string,
  boardId: string,
  meta: WorkspaceBoardMeta,
): void {
  writeBoardListMeta(workspaceId, meta)
  queryClient.setQueryData<WorkspaceBoardDocument>(
    boardQueryKeys.document(workspaceId, boardId),
    (previous) =>
      previous && meta.revision > previous.revision
        ? { ...previous, revision: meta.revision }
        : previous,
  )
}

/** Delete/archive convergence: invalidate the list, drop the document. */
export function removeBoardDocumentQuery(workspaceId: string, boardId: string): void {
  void queryClient.invalidateQueries({ queryKey: boardQueryKeys.list(workspaceId), exact: true })
  queryClient.removeQueries({
    queryKey: boardQueryKeys.document(workspaceId, boardId),
    exact: true,
  })
}
