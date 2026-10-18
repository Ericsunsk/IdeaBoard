import type { QueryClient } from '@tanstack/vue-query'
import { queryClient } from '../../../../core/query/queryClient'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { boardOperationIdempotencyKey } from '../../../../api/idempotencyKey'
import { getBoardScope, isBoardScopeCurrent } from '../catalog/scope'
import { boardQueryKeys } from '../sync/boardQueryKeys'
import { fetchWorkspaceMembers } from '../../../workspace'
import {
  fetchBoardAccessFact,
  fetchBoardCreatorIdentity,
  fetchWorkspaceBoardMeta,
  replaceBoardSharing,
  type ShareAccessInput,
  type WorkspaceBoardAccessFact,
} from '../adapters/boardsResource'

// Captured residency prevents late responses from updating another session.
// Unknown-outcome retries reuse the original idempotency key and access version.

/** User + Workspace + Board identity captured at intent time. */
export type ShareActionScope = {
  userScope: string
  workspaceId: string
  boardId: string
  residency: number
}

/** One user attempt: idempotency key plus the observed access version. */
export type ShareAttempt = {
  key: string
  version: number
}

export type ShareFailureReason =
  | 'board-not-saved'
  | 'not-found'
  | 'forbidden'
  | 'access-conflict'
  | 'precondition-required'
  | 'archived'
  | 'member-unknown'
  | 'invalid'
  | 'sync-failed'

type ShareActionFailure = { isOk: false; reason: ShareFailureReason; error: unknown }
type ShareScopeChanged = { isOk: false; reason: 'scope-changed' }

export type ShareActionResult =
  { isOk: true; access: WorkspaceBoardAccessFact } | ShareActionFailure | ShareScopeChanged

export type ShareTransport = {
  fetchMeta: typeof fetchWorkspaceBoardMeta
  fetchAccess: typeof fetchBoardAccessFact
  fetchRoster: typeof fetchWorkspaceMembers
  fetchBoardCreatorIdentity: typeof fetchBoardCreatorIdentity
  replaceAccess: typeof replaceBoardSharing
}

export const shareTransport: ShareTransport = {
  fetchMeta: fetchWorkspaceBoardMeta,
  fetchAccess: fetchBoardAccessFact,
  fetchRoster: fetchWorkspaceMembers,
  fetchBoardCreatorIdentity,
  replaceAccess: replaceBoardSharing,
}

export type ShareCommandDeps = {
  transport?: ShareTransport
  client?: QueryClient
}

/** Captures the current User + Workspace scope plus the target board. */
export function captureShareScope(boardId: string): ShareActionScope | null {
  const scope = getBoardScope()
  const normalizedBoardId = boardId.trim()
  if (!scope.userScope || !scope.workspaceId || !normalizedBoardId) return null
  return {
    ...scope,
    userScope: scope.userScope,
    workspaceId: scope.workspaceId,
    boardId: normalizedBoardId,
  }
}

/** Late-response guard: the captured scope must still be selected. */
export function isShareScopeCurrent(scope: ShareActionScope): boolean {
  return isBoardScopeCurrent(scope)
}

/** Mints one idempotency key for a user attempt at the observed version. */
export function mintShareAttempt(version: number): ShareAttempt {
  return { key: boardOperationIdempotencyKey('share'), version }
}

/** Maps stable backend codes to Share failure reasons (never prose). */
export function classifyShareError(error: unknown): ShareFailureReason {
  switch (userErrorCode(error)) {
    case 'board_access_conflict':
    case 'board_idempotency_conflict':
      return 'access-conflict'
    case 'board_precondition_required':
      return 'precondition-required'
    case 'board_forbidden':
      return 'forbidden'
    case 'board_not_found':
      return 'not-found'
    case 'board_member_not_found':
      return 'member-unknown'
    case 'board_archived':
      return 'archived'
    case 'bad_request':
    case 'idempotency_key_required':
      return 'invalid'
    default:
      return 'sync-failed'
  }
}

/** Refreshes the catalog and sharing resource after an accepted or rejected attempt. */
export function invalidateShareQueries(
  client: QueryClient,
  workspaceId: string,
  boardId: string,
): void {
  void client.invalidateQueries({ queryKey: boardQueryKeys.list(workspaceId), exact: true })
  void client.invalidateQueries({
    queryKey: boardQueryKeys.access(workspaceId, boardId),
    exact: true,
  })
}

function scopeChanged(): ShareScopeChanged {
  return { isOk: false, reason: 'scope-changed' }
}

function shareFailed(reason: ShareFailureReason, error: unknown): ShareActionFailure {
  return { isOk: false, reason, error }
}

export async function replaceShareAccess(
  deps: ShareCommandDeps,
  scope: ShareActionScope,
  attempt: ShareAttempt,
  input: ShareAccessInput,
): Promise<ShareActionResult> {
  const transport = deps.transport ?? shareTransport
  const client = deps.client ?? queryClient
  if (!isShareScopeCurrent(scope)) return scopeChanged()
  try {
    await transport.fetchMeta(scope.workspaceId, scope.boardId)
  } catch (error) {
    if (!isShareScopeCurrent(scope)) return scopeChanged()
    invalidateShareQueries(client, scope.workspaceId, scope.boardId)
    const reason = classifyShareError(error)
    return shareFailed(reason === 'not-found' ? 'board-not-saved' : reason, error)
  }
  if (!isShareScopeCurrent(scope)) return scopeChanged()
  try {
    const access = await transport.replaceAccess(scope.workspaceId, scope.boardId, input, {
      expectedVersion: attempt.version,
      idempotencyKey: attempt.key,
    })
    if (!isShareScopeCurrent(scope)) return scopeChanged()
    client.setQueryData(boardQueryKeys.access(scope.workspaceId, scope.boardId), access)
    invalidateShareQueries(client, scope.workspaceId, scope.boardId)
    return { isOk: true, access }
  } catch (error) {
    if (!isShareScopeCurrent(scope)) return scopeChanged()
    invalidateShareQueries(client, scope.workspaceId, scope.boardId)
    return shareFailed(classifyShareError(error), error)
  }
}
