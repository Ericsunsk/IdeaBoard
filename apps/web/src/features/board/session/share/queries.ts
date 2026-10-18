import type { QueryFunctionContext } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { accountScoped } from '../../../../core/query/queryDefaults'
import { fetchBoardAccessFact } from '../adapters/boardsResource'
import { boardQueryKeys } from '../sync/boardQueryKeys'
import type { WorkspaceBoardMeta } from '../adapters/boardsResource'
import type { BoardAccessCapabilities } from '../catalog/boards'

// TanStack Query owns the sharing resource and Workspace roster.
// SSE invalidates facts; accepted replacements seed only the access resource.

/** Board facts the Share panel renders without a second permission fact. */
export type ShareBoardFacts = {
  name: string
  capabilities: BoardAccessCapabilities
  accessState: 'private' | 'workspace'
  workspaceAccess: 'none' | 'viewer' | 'editor'
  isCreator: boolean
  creatorUserId: string
}

/** Reads Share board facts from the catalog list cache (no network). */
export function readShareBoardFacts(workspaceId: string, boardId: string): ShareBoardFacts | null {
  const cached = queryClient.getQueryData<WorkspaceBoardMeta[]>(boardQueryKeys.list(workspaceId))
  const meta = cached?.find((entry) => entry.id === boardId)
  return meta ? toShareBoardFacts(meta) : null
}

/** Maps a server board meta to Share board facts (cold-cache fallback). */
export function toShareBoardFacts(meta: WorkspaceBoardMeta): ShareBoardFacts | null {
  if (!meta.capabilities) return null
  return {
    name: meta.name,
    capabilities: { ...meta.capabilities },
    accessState: meta.accessState,
    workspaceAccess: meta.workspaceAccess,
    isCreator: meta.isCreator,
    creatorUserId: meta.creatorUserId,
  }
}

/** Creator-only sharing resource query. */
export function shareAccessOptions(
  workspaceId: string,
  boardId: string,
  fetch: typeof fetchBoardAccessFact = fetchBoardAccessFact,
) {
  const queryFn = ({ signal }: QueryFunctionContext) => fetch(workspaceId, boardId, { signal })
  return {
    queryKey: boardQueryKeys.access(workspaceId, boardId),
    queryFn,
    ...accountScoped(),
  }
}
