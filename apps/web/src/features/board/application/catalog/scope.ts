import {
  getAuthStorageUserScope,
  subscribeAuthStorageScopeChange,
} from '../../../../shared/utils/auth'
import {
  getWorkspaceSelectionStore,
  subscribeWorkspaceSelectionChange,
} from '../../../workspace'

// Board scope (issue #894): every catalog/state cache key plus every async
// catalog mutation is bound to User + Workspace. The workspace half resolves
// from the `features/workspace` selection store through its public entry;
// Board never duplicates the selection store. A missing half means "no
// scope": readers stay empty and writers no-op instead of inferring.

export type BoardScope = {
  userScope: string | null
  workspaceId: string | null
}

export type CompleteBoardScope = {
  userScope: string
  workspaceId: string
}

export function getBoardScope(): BoardScope {
  return {
    userScope: getAuthStorageUserScope(),
    workspaceId: getWorkspaceSelectionStore().getSelectedId(),
  }
}

export function isCompleteBoardScope(scope: BoardScope): scope is CompleteBoardScope {
  return !!scope.userScope && !!scope.workspaceId
}

export function isBoardScopeCurrent(captured: BoardScope): boolean {
  const current = getBoardScope()
  return current.userScope === captured.userScope && current.workspaceId === captured.workspaceId
}

export function boardScopeKey(scope: CompleteBoardScope): string {
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

/** Scoped cache key (`<prefix>.<userScope>.<workspaceId>`); null when scope is incomplete. */
export function scopedBoardStorageKey(prefix: string, scope?: BoardScope): string | null {
  const resolved = scope ?? getBoardScope()
  if (!isCompleteBoardScope(resolved)) return null
  return `${prefix}.${resolved.userScope}.${resolved.workspaceId}`
}
