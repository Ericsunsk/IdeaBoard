import type { BoardState } from '../../core/document/nodes'
import { getBoardState } from '../persistence/state'
import { serializeBoardState } from '../persistence/boardStateSerialization'
import { getBoardScope, isCompleteBoardScope, subscribeBoardScopeChange } from '../catalog/scope'

// Runtime-only evidence of the last observed server content. Cold storage
// without a baseline remains dirty; timestamps and preview caches cannot
// prove that an edit was saved.
const serverContent = new Map<string, string>()
const liveDraftGuards = new Set<(boardId: string) => boolean>()
subscribeBoardScopeChange(() => serverContent.clear())

function documentContent(state: BoardState): string {
  const envelope = serializeBoardState(state, 'remote-envelope')
  return JSON.stringify({
    schemaVersion: envelope.schemaVersion,
    imageNodes: envelope.imageNodes,
    promptNodes: envelope.promptNodes,
    fusionNodes: envelope.fusionNodes,
    connections: envelope.connections,
    groups: envelope.groups,
    mediaRefs: envelope.mediaRefs ?? [],
  })
}

export function rememberBoardServerContent(boardId: string, state: BoardState): void {
  serverContent.set(boardId.trim(), documentContent(state))
}

/** Drops the server-content baseline for one board (access-loss cleanup). */
export function forgetBoardServerContent(boardId: string): void {
  serverContent.delete(boardId.trim())
}

export function hasBoardServerContent(boardId: string): boolean {
  return serverContent.has(boardId.trim())
}

export function isBoardDraftDirty(boardId: string): boolean {
  if ([...liveDraftGuards].some((guard) => guard(boardId))) return true
  const state = getBoardState(boardId)
  if (!state) return false
  return serverContent.get(boardId.trim()) !== documentContent(state)
}

export function registerBoardDraftGuard(guard: (boardId: string) => boolean): () => void {
  let scope = getBoardScope()
  const stopScope = subscribeBoardScopeChange((next) => {
    scope = next
  })
  const scopedGuard = (boardId: string) => {
    const current = getBoardScope()
    return (
      isCompleteBoardScope(scope) &&
      current.userScope === scope.userScope &&
      current.workspaceId === scope.workspaceId &&
      guard(boardId)
    )
  }
  liveDraftGuards.add(scopedGuard)
  return () => {
    liveDraftGuards.delete(scopedGuard)
    stopScope()
  }
}
