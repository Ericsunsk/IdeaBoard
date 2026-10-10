// Executable P2.5 design spike. This private test module is deliberately not
// wired into the production repository; P3 must bind these scenarios to the
// production Query boundary before claiming production red/green evidence.
import { QueryClient, type QueryFunctionContext } from '@tanstack/vue-query'
import { queryClient } from '../../../core/query/queryClient'
import { accountScoped } from '../../../core/query/queryDefaults'
import { subscribeWorkspaceEvent } from '../../../api/workspaceEventStream'
import {
  fetchWorkspaceBoards,
  fetchWorkspaceBoardDocument,
  type WorkspaceBoardDocument,
} from '../infrastructure/api/boardsResource'

export const designBoardKeys = {
  list: (workspaceId: string) => ['boards', workspaceId, 'list'] as const,
  document: (workspaceId: string, boardId: string) =>
    ['boards', workspaceId, 'board', boardId, 'document'] as const,
}

export function newDesignQueryClient() {
  return new QueryClient({ defaultOptions: queryClient.getDefaultOptions() })
}

export function designListOptions(workspaceId: string) {
  return {
    queryKey: designBoardKeys.list(workspaceId),
    queryFn: ({ signal }: QueryFunctionContext) => fetchWorkspaceBoards(workspaceId, { signal }),
    ...accountScoped(),
  }
}

export function designDocumentOptions(workspaceId: string, boardId: string) {
  return {
    queryKey: designBoardKeys.document(workspaceId, boardId),
    queryFn: ({ signal }: QueryFunctionContext) =>
      fetchWorkspaceBoardDocument(workspaceId, boardId, { signal }),
    // A successful older read must not roll the observed server revision back.
    structuralSharing: (previous: unknown, next: unknown) => {
      const oldDocument = previous as WorkspaceBoardDocument | undefined
      const newDocument = next as WorkspaceBoardDocument
      return oldDocument && newDocument.revision < oldDocument.revision ? oldDocument : next
    },
  }
}

export function subscribeDesignBoardInvalidation(
  client: QueryClient,
  workspaceId: string,
  isDraftDirty: (boardId: string) => boolean = () => false,
) {
  const pending = new Map<string, number>()
  const observedRevisions = new Map<string, number>()
  let timer: ReturnType<typeof setTimeout> | undefined
  const flush = () => {
    timer = undefined
    for (const [boardId, revision] of pending) {
      observedRevisions.set(boardId, revision)
      const queryKey = designBoardKeys.document(workspaceId, boardId)
      if (isDraftDirty(boardId)) {
        // Query.invalidate marks the real cache stale without refetching an
        // active observer and replacing the draft under the user's cursor.
        client.getQueryCache().find({ queryKey, exact: true })?.invalidate()
      } else {
        void client.invalidateQueries({ queryKey, exact: true })
      }
    }
    pending.clear()
  }
  const stop = subscribeWorkspaceEvent(
    workspaceId,
    (envelope) => {
      const boardId = envelope.resource.id
      if (envelope.type === 'board.document_replaced') {
        const revision = Number(envelope.data.revision)
        const cached = client.getQueryData<WorkspaceBoardDocument>(
          designBoardKeys.document(workspaceId, boardId),
        )
        const knownRevision = Math.max(
          cached?.revision ?? 0,
          observedRevisions.get(boardId) ?? 0,
          pending.get(boardId) ?? 0,
        )
        if (!Number.isFinite(revision) || revision <= knownRevision) return
        pending.set(boardId, revision)
        if (timer === undefined) timer = setTimeout(flush, 500)
      } else if (envelope.type === 'board.member_changed') {
        void client.invalidateQueries({ queryKey: designBoardKeys.list(workspaceId), exact: true })
      } else if (envelope.type === 'board.share_revoked') {
        pending.delete(boardId)
        void client.invalidateQueries({ queryKey: designBoardKeys.list(workspaceId), exact: true })
        client.removeQueries({
          queryKey: designBoardKeys.document(workspaceId, boardId),
          exact: true,
        })
      }
    },
    {
      types: ['board.document_replaced', 'board.member_changed', 'board.share_revoked'],
      onGap: () => {
        void client.invalidateQueries({ queryKey: ['boards', workspaceId] })
      },
    },
  )
  return () => {
    stop()
    if (timer !== undefined) clearTimeout(timer)
    pending.clear()
  }
}
