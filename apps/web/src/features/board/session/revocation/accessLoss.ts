import { userErrorCode } from '../../../../core/errors/errorCatalog'
import { getBoardScope, isBoardScopeCurrent, subscribeBoardScopeChange } from '../catalog/scope'
import { fetchWorkspaceBoardMeta } from '../adapters/boardsResource'

// Confirmed board access loss (#941): the single client-side vocabulary
// distinguishing a PROVEN revocation from errors that must preserve the
// draft (network failures, ordinary document GET/PUT 404s, incomplete
// catalogs, 5xx). Only a revalidation read (board meta or the access
// fact) feeds the classifier below: document reads never classify
// directly — a save/load failure with board codes triggers a revalidation
// first, and only the revalidation verdict converges.
//
// Revoked (converge: stop writes/autosave/generation, close Share, purge
// protected queries/memory/local copies, release display leases, return
// to the Gallery; never auto-copy unsaved content):
// - `board_forbidden` (403: visible but no capability)
// - `board_not_found` on a board that was present (404: invisible now)
// - `workspace_not_found` (workspace membership gone entirely)
// - bare 403/404 status on a revalidation read (transports that drop the
//   wire code; an authed meta read failing this way means no access)
// Preserved (keep the draft, keep retrying, never converge):
// - status-less errors (offline/timeout/abort)
// - 5xx and unknown codes
// - incomplete-catalog signals (absence is never deletion there)
// - ordinary document GET/PUT 404s (they only TRIGGER a revalidation)

export type BoardAccessLossScope = {
  userScope: string
  workspaceId: string
  residency: number
}

function currentLossScope(): BoardAccessLossScope | null {
  const scope = getBoardScope()
  if (!scope.userScope || !scope.workspaceId) return null
  return { userScope: scope.userScope, workspaceId: scope.workspaceId, residency: scope.residency }
}

function lossKey(scope: BoardAccessLossScope, boardId: string): string {
  return `${scope.userScope}\x00${scope.workspaceId}\x00${boardId.trim()}`
}

// Confirmed-loss registry: per User + Workspace + Board, cleared on every
// scope switch so a mark captured under one identity can never gate
// saves, submits, or retries under another (user/workspace switch
// isolation). Autosave, generation submit, and retry loops consult
// `isBoardAccessLost` before writing; convergence marks through
// `markBoardAccessLost`.
const revokedBoardIds = new Set<string>()
// Stale-access flags: set on SSE gaps, reconnects, and hidden-tab
// recovery for the open board; the next sensitive op (task submit)
// revalidates once and clears. Cleared on scope switches like the
// registry above.
const staleBoardIds = new Set<string>()
subscribeBoardScopeChange(() => {
  revokedBoardIds.clear()
  staleBoardIds.clear()
})

/** Records a confirmed loss for the board under the current scope. */
export function markBoardAccessLost(boardId: string, scope?: BoardAccessLossScope | null): void {
  const resolved = scope ?? currentLossScope()
  const normalized = boardId.trim()
  if (!resolved || !normalized || !isBoardScopeCurrent(resolved)) return
  revokedBoardIds.add(lossKey(resolved, normalized))
  staleBoardIds.delete(lossKey(resolved, normalized))
}

/** Whether the board is confirmed lost under the current scope. */
export function isBoardAccessLost(boardId: string): boolean {
  const scope = currentLossScope()
  const normalized = boardId.trim()
  if (!scope || !normalized) return false
  return revokedBoardIds.has(lossKey(scope, normalized))
}

/** Clears a loss mark (a later revalidation proved access again). */
function clearBoardAccessLost(boardId: string, scope?: BoardAccessLossScope | null): void {
  const resolved = scope ?? currentLossScope()
  const normalized = boardId.trim()
  if (!resolved || !normalized || !isBoardScopeCurrent(resolved)) return
  revokedBoardIds.delete(lossKey(resolved, normalized))
}

/** Flags the board for revalidation before the next sensitive op. */
export function markBoardAccessStale(boardId: string): void {
  const scope = currentLossScope()
  const normalized = boardId.trim()
  if (!scope || !normalized) return
  const key = lossKey(scope, normalized)
  if (!revokedBoardIds.has(key)) staleBoardIds.add(key)
}

/** Whether the board needs revalidation before the next sensitive op. */
export function isBoardAccessStale(boardId: string): boolean {
  const scope = currentLossScope()
  const normalized = boardId.trim()
  if (!scope || !normalized) return false
  return staleBoardIds.has(lossKey(scope, normalized))
}

const CONFIRMED_LOSS_CODES = new Set(['board_forbidden', 'board_not_found', 'workspace_not_found'])

function errorStatusInChain(error: unknown): number | null {
  const seen = new Set<unknown>()
  let current: unknown = error
  while (current && typeof current === 'object' && !seen.has(current)) {
    seen.add(current)
    const record = current as { status?: unknown; cause?: unknown }
    if (typeof record.status === 'number') return record.status
    current = record.cause
  }
  return null
}

/**
 * Revalidation-only loss classifier. Callers MUST only feed revalidation
 * outcomes (board meta or access-fact reads of a previously present
 * board): document GET/PUT outcomes always revalidate first instead.
 */
export function isConfirmedAccessLoss(error: unknown): boolean {
  const code = userErrorCode(error)
  if (code !== null) return CONFIRMED_LOSS_CODES.has(code)
  const status = errorStatusInChain(error)
  return status === 403 || status === 404
}

export type BoardAccessRevalidation = 'ok' | 'revoked' | 'preserved'

export type RevalidateBoardAccessOptions = {
  workspaceId?: string | null | undefined
  fetchMeta?: typeof fetchWorkspaceBoardMeta | undefined
}

/**
 * Re-reads one board's meta to disambiguate a suspicious failure.
 * `revoked` marks the registry; `ok` clears a previous mark (access
 * proved again, e.g. a re-grant); `preserved` leaves everything
 * untouched (draft and retries survive network errors, 5xx, and
 * incomplete reads). Never throws.
 */
export async function revalidateBoardAccess(
  boardId: string,
  options: RevalidateBoardAccessOptions = {},
): Promise<BoardAccessRevalidation> {
  const normalized = boardId.trim()
  if (!normalized) return 'preserved'
  // Captured at intent time: a User/Workspace switch mid-flight must not
  // attribute this verdict to the new scope (late responses never steer
  // another scope's registry).
  const scope = currentLossScope()
  if (!scope || (options.workspaceId && options.workspaceId !== scope.workspaceId))
    return 'preserved'
  const fetchMeta = options.fetchMeta ?? fetchWorkspaceBoardMeta
  try {
    await fetchMeta(scope.workspaceId, normalized)
  } catch (error) {
    if (!isConfirmedAccessLoss(error)) return 'preserved'
    // A scope move mid-flight drops the verdict instead of attributing
    // it anywhere: marking the departed scope would leave a stale gate
    // behind, and marking the new scope would pollute it. Returning to
    // the board re-derives through a fresh revalidation.
    if (!isBoardScopeCurrent(scope)) return 'preserved'
    markBoardAccessLost(normalized, scope)
    return 'revoked'
  }
  if (!isBoardScopeCurrent(scope)) return 'preserved'
  clearBoardAccessLost(normalized, scope)
  staleBoardIds.delete(lossKey(scope, normalized))
  return 'ok'
}
