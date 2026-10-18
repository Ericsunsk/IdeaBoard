import {
  getAuthStorageUserScope,
  getRecoverableAuthStorageUserScope,
  subscribeAuthStorageScopeChange,
} from '../../../../core/utils/auth'
import { getWorkspaceSelectionStore, subscribeWorkspaceSelectionChange } from '../../../workspace'

// Active User + Workspace scope gates repository views and async mutations.
// Local cache keys may use the remembered user during transient auth loss to
// preserve drafts. Explicit logout clears that recovery scope; neither path
// infers a missing workspace from legacy user-only caches.

export type BoardScope = {
  userScope: string | null
  workspaceId: string | null
  residency: number
}

export type CompleteBoardScope = BoardScope & {
  userScope: string
  workspaceId: string
}

let residency = 0
const advanceResidency = () => {
  residency += 1
}
subscribeAuthStorageScopeChange(advanceResidency)
subscribeWorkspaceSelectionChange(advanceResidency)

export function getBoardScope(): BoardScope {
  return {
    userScope: getAuthStorageUserScope(),
    workspaceId: getWorkspaceSelectionStore().getSelectedId(),
    residency,
  }
}

export function getRecoverableBoardScope(): BoardScope {
  return {
    userScope: getRecoverableAuthStorageUserScope(),
    workspaceId: getWorkspaceSelectionStore().getSelectedId(),
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
  const unsubscribeWorkspace = subscribeWorkspaceSelectionChange(emit)
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
