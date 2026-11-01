import {
  getAuthStorageUserScope,
  getRecoverableAuthStorageUserScope,
  subscribeAuthStorageScopeChange,
} from '../../../../core/utils/auth'
import {
  getWorkspaceSelectionStore as getDefaultWorkspaceSelectionStore,
  subscribeWorkspaceSelectionChange as subscribeDefaultWorkspaceSelectionChange,
} from '../../../workspace'

// Active User + Workspace scope gates repository views and async mutations.
// Local cache keys may use the remembered user during transient auth loss to
// preserve drafts. Explicit logout clears that recovery scope; neither path
// infers a missing workspace from legacy user-only caches.
//
// Ports seam: the workspace selection is an injected port (default: the
// `features/workspace` selection store). Lower layers resolve through
// `getBoardScope`/subscriptions, never by importing the store directly.

export type BoardScope = {
  userScope: string | null
  workspaceId: string | null
  residency: number
}

export type CompleteBoardScope = BoardScope & {
  userScope: string
  workspaceId: string
}

export type BoardScopeWorkspacePort = {
  getSelectedId: () => string | null
  subscribeSelectionChange: (listener: (id: string | null) => void) => () => void
}

function defaultWorkspacePort(): BoardScopeWorkspacePort {
  return {
    getSelectedId: () => {
      try {
        return getDefaultWorkspaceSelectionStore().getSelectedId()
      } catch {
        return null
      }
    },
    subscribeSelectionChange: (listener) => subscribeDefaultWorkspaceSelectionChange(listener),
  }
}

let workspacePort: BoardScopeWorkspacePort = defaultWorkspacePort()
let stopWorkspaceResidency: (() => void) | null = null

export function setBoardScopeWorkspacePortForTests(port: Partial<BoardScopeWorkspacePort>): void {
  stopWorkspaceResidency?.()
  workspacePort = { ...workspacePort, ...port }
  stopWorkspaceResidency = workspacePort.subscribeSelectionChange(advanceResidency)
}

export function resetBoardScopeWorkspacePortForTests(): void {
  stopWorkspaceResidency?.()
  workspacePort = defaultWorkspacePort()
  stopWorkspaceResidency = workspacePort.subscribeSelectionChange(advanceResidency)
}

let residency = 0
const advanceResidency = () => {
  residency += 1
}
subscribeAuthStorageScopeChange(advanceResidency)
stopWorkspaceResidency = workspacePort.subscribeSelectionChange(advanceResidency)

export function getBoardScope(): BoardScope {
  return {
    userScope: getAuthStorageUserScope(),
    workspaceId: workspacePort.getSelectedId(),
    residency,
  }
}

export function getRecoverableBoardScope(): BoardScope {
  return {
    userScope: getRecoverableAuthStorageUserScope(),
    workspaceId: workspacePort.getSelectedId(),
    residency,
  }
}

export function isCompleteBoardScope(scope: BoardScope): scope is CompleteBoardScope {
  return !!scope.userScope && !!scope.workspaceId
}

export function isBoardScopeCurrent(captured: BoardScope): boolean {
  const current = getBoardScope()
  return (
    current.userScope === captured.userScope &&
    current.workspaceId === captured.workspaceId &&
    current.residency === captured.residency
  )
}

export function boardScopeKey(
  scope: Pick<CompleteBoardScope, 'userScope' | 'workspaceId'>,
): string {
  return `${scope.userScope}@${scope.workspaceId}`
}

export function subscribeBoardScopeChange(listener: (scope: BoardScope) => void): () => void {
  const emit = () => listener(getBoardScope())
  const unsubscribeAuth = subscribeAuthStorageScopeChange(emit)
  const unsubscribeWorkspace = workspacePort.subscribeSelectionChange(emit)
  return () => {
    unsubscribeAuth()
    unsubscribeWorkspace()
  }
}

/** Local cache key; transient auth loss can retain the remembered user scope. */
export function scopedBoardStorageKey(prefix: string, scope?: BoardScope): string | null {
  const resolved = scope ?? getRecoverableBoardScope()
  if (!isCompleteBoardScope(resolved)) return null
  return `${prefix}.${resolved.userScope}.${resolved.workspaceId}`
}
