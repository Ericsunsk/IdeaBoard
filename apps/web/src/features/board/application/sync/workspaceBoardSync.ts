// Workspace board sync boundary (issue #802, frontend slice).
//
// Owns the snake_case transport -> camelCase domain mapping for the
// workspace-rooted board endpoints (the resource stays thin), the
// ETag/If-Match revision storage, revision/conflict UI states, and the
// workspace-scoped query keys.
//
// Query keys depend on the workspace selection from the `workspace` feature
// entry and reuse its key root: this module never duplicates the workspace
// store. Conflict handling reuses the rollback convention from `boardSync`
// (preserve-local-newer + explicit rollback ids); only the branch
// classification is board-specific.

import {
  workspaceQueryKeys,
} from '../../../workspace/index'
import {
  isOpaqueId,
  newRequestId,
  opaqueIdPrefix,
  parseContractEnvelope,
} from '../../../../shared/api/contractKernel'
import { boardOperationIdempotencyKey } from '../../../../shared/api/idempotencyKey'
import type {
  WorkspaceBoardDocument,
  WorkspaceBoardMember,
  WorkspaceBoardShare,
} from '../../infrastructure/api/workspaceBoardsResource'

export type WorkspaceBoardSummary = {
  boardId: string
  workspaceId: string
  name: string
  revision: number
  schemaVersion: number
  archivedAt: string | null
  createdAt: string
  updatedAt: string
}

export type WorkspaceBoardPage = {
  items: WorkspaceBoardSummary[]
  nextCursor: string | null
  hasMore: boolean
}

export type WorkspaceBoardSnapshot = {
  boardId: string
  revision: number
  schemaVersion: number
  document: unknown
  updatedAt: string
}

export type WorkspaceBoardMembership = {
  userId: string
  role: 'owner' | 'editor' | 'viewer'
  createdAt?: string | undefined
}

export type WorkspaceBoardShareSummary = {
  tokenId: string
  bearer?: string | undefined
  tokenPrefix: string
  role: string
  revoked: boolean
  usageCount: number
  createdAt: string
  revokedAt?: string | null | undefined
}

// Hidden (unknown board, grantless caller, malformed board_ shape) surfaces
// as 404; visible-but-forbidden surfaces as 403. Branch on `kind`, not copy.
export type WorkspaceBoardAccessKind = 'hidden' | 'forbidden'

export const WORKSPACE_BOARD_PAGE_SIZE_DEFAULT = 20
export const WORKSPACE_BOARD_PAGE_SIZE_MAX = 100
export const WORKSPACE_BOARD_NAME_MAX_LENGTH = 120

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requireBoardId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'board') {
    throw new Error('Invalid board opaque ID')
  }
  return value
}

function requireWorkspaceId(value: unknown): string {
  if (typeof value !== 'string' || !isOpaqueId(value) || opaqueIdPrefix(value) !== 'ws') {
    throw new Error('Invalid workspace opaque ID')
  }
  return value
}

function asRevision(value: unknown): number {
  const revision = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(revision) || revision < 0) throw new Error('Invalid board revision')
  return revision
}

export function parseWorkspaceBoardItem(raw: unknown): WorkspaceBoardSummary {
  if (!isRecord(raw)) throw new Error('Invalid board payload')
  const boardId = requireBoardId(raw.id)
  const workspaceId = requireWorkspaceId(raw.workspace_id)
  const name = typeof raw.name === 'string' ? raw.name.trim() : ''
  if (!name) throw new Error('Invalid board payload')
  return {
    boardId,
    workspaceId,
    name,
    revision: asRevision(raw.revision),
    schemaVersion: typeof raw.schema_version === 'number' ? raw.schema_version : 0,
    archivedAt: typeof raw.archived_at === 'string' ? raw.archived_at : null,
    createdAt: typeof raw.created_at === 'string' ? raw.created_at : '',
    updatedAt: typeof raw.updated_at === 'string' ? raw.updated_at : '',
  }
}

export function parseWorkspaceBoardPage(payload: unknown): WorkspaceBoardPage {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || 'Invalid board list response') as Error & {
      code?: string | undefined
      requestId?: string | undefined
    }
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    throw error
  }
  if (!Array.isArray(parsed.data)) throw new Error('Invalid board list response')
  const items = parsed.data.map(parseWorkspaceBoardItem)
  const meta = isRecord(parsed.meta) ? parsed.meta : null
  const nextCursor = typeof meta?.next_cursor === 'string' ? meta.next_cursor : null
  const hasMore = meta?.has_more === true
  return { items, nextCursor, hasMore }
}

export function parseWorkspaceBoardSnapshot(payload: unknown): WorkspaceBoardSnapshot {
  const parsed = parseContractEnvelope<unknown>(payload)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || 'Invalid board document response') as Error & {
      code?: string | undefined
      requestId?: string | undefined
    }
    error.code = parsed.error.code
    error.requestId = parsed.error.request_id
    throw error
  }
  const data = parsed.data as WorkspaceBoardDocument
  if (!isRecord(data)) throw new Error('Invalid board document response')
  return {
    boardId: requireBoardId(data.board_id),
    revision: asRevision(data.revision),
    schemaVersion: typeof data.schema_version === 'number' ? data.schema_version : 0,
    document: data.document,
    updatedAt: typeof data.updated_at === 'string' ? data.updated_at : '',
  }
}

const BOARD_ROLES = new Set(['owner', 'editor', 'viewer'])

export function parseWorkspaceBoardMembership(raw: unknown): WorkspaceBoardMembership {
  const item = raw as WorkspaceBoardMember
  if (!isRecord(raw) || typeof item.user_id !== 'string' || !BOARD_ROLES.has(item.role)) {
    throw new Error('Invalid board member payload')
  }
  return {
    userId: item.user_id,
    role: item.role as WorkspaceBoardMembership['role'],
    ...(typeof item.created_at === 'string' ? { createdAt: item.created_at } : {}),
  }
}

export function parseWorkspaceBoardShare(raw: unknown): WorkspaceBoardShareSummary {
  const item = raw as WorkspaceBoardShare
  if (!isRecord(raw) || typeof item.token_id !== 'string' || !item.token_id.trim()) {
    throw new Error('Invalid board share payload')
  }
  return {
    tokenId: item.token_id,
    ...(typeof item.token === 'string' && item.token ? { bearer: item.token } : {}),
    tokenPrefix: typeof item.token_prefix === 'string' ? item.token_prefix : '',
    role: typeof item.role === 'string' ? item.role : '',
    revoked: item.revoked === true,
    usageCount: typeof item.usage_count === 'number' ? item.usage_count : 0,
    createdAt: typeof item.created_at === 'string' ? item.created_at : '',
    ...(typeof item.revoked_at === 'string' ? { revokedAt: item.revoked_at } : {}),
  }
}

// ETag/If-Match storage: the revision counter doubles as the validator.
// `api.request` returns parsed bodies only, so the revision observed in the
// mapped body is the stored validator (the response ETag header carries the
// same value). Writers send it back as If-Match; a 409 means refresh-then-
// retry with the same idempotency key.
const storedBoardRevisions = new Map<string, number>()

export function getStoredBoardRevision(boardId: string): number | null {
  return storedBoardRevisions.get(boardId.trim()) ?? null
}

export function setStoredBoardRevision(boardId: string, revision: number): void {
  const normalized = boardId.trim()
  if (!normalized || !Number.isFinite(revision) || revision < 0) return
  storedBoardRevisions.set(normalized, revision)
}

export function clearStoredBoardRevision(boardId: string): void {
  storedBoardRevisions.delete(boardId.trim())
}

export function trackBoardSummary(summary: WorkspaceBoardSummary): WorkspaceBoardSummary {
  setStoredBoardRevision(summary.boardId, summary.revision)
  return summary
}

function readErrorStatus(error: unknown): number | null {
  if (!isRecord(error)) return null
  const status = error.status
  return typeof status === 'number' && Number.isFinite(status) ? status : null
}

function readErrorCode(error: unknown): string | null {
  if (!isRecord(error)) return null
  const code = (error as { code?: unknown }).code
  if (typeof code === 'string' && code) return code.toLowerCase()
  const data = isRecord(error.data) ? (error.data as Record<string, unknown>) : null
  const nested = data?.code ?? data?.error_code
  if (typeof nested === 'string' && nested) return nested.toLowerCase()
  const contractError = isRecord(data?.error) ? (data.error as Record<string, unknown>) : null
  const contractCode = contractError?.code
  return typeof contractCode === 'string' && contractCode ? contractCode.toLowerCase() : null
}

export function isBoardRevisionConflictError(error: unknown): boolean {
  if (readErrorStatus(error) === 409) {
    const code = readErrorCode(error)
    return code == null || code.includes('revision_conflict') || code.includes('conflict')
  }
  const code = readErrorCode(error)
  return code != null && code.includes('revision_conflict')
}

export function isBoardIdempotencyConflictError(error: unknown): boolean {
  if (readErrorStatus(error) !== 409) return false
  const code = readErrorCode(error)
  return code != null && code.includes('idempotency_conflict')
}

export function classifyBoardAccessError(error: unknown): WorkspaceBoardAccessKind | null {
  const status = readErrorStatus(error)
  if (status === 404) return 'hidden'
  if (status === 403) return 'forbidden'
  const code = readErrorCode(error)
  if (!code) return null
  if (code.includes('not_found') || code === 'board_hidden') return 'hidden'
  if (code.includes('forbidden') || code.includes('denied')) return 'forbidden'
  return null
}

export type BoardWriteHeadersOptions = {
  idempotencyKey?: string | undefined
  requestId?: string | undefined
  ifMatch?: string | number | null | undefined
  operation?: string | undefined
}

export function buildBoardWriteHeaders(options?: BoardWriteHeadersOptions): Record<string, string> {
  const headers: Record<string, string> = {}
  const requestId = options?.requestId?.trim()
  headers['Request-Id'] = requestId ? requestId : newRequestId()
  const key = options?.idempotencyKey?.trim()
  headers['Idempotency-Key'] = key ? key : boardOperationIdempotencyKey(options?.operation ?? 'write')
  if (options?.ifMatch != null && options.ifMatch !== '') {
    headers['If-Match'] = String(options.ifMatch)
  }
  return headers
}

// Single source of truth for workspace board server state: TanStack Query
// caches under these keys, rooted at the workspace feature key root so
// workspace switches invalidate board caches without duplicating selection.
export function workspaceBoardQueryKeys(workspaceId: string): {
  all: readonly [string, string, string]
  list: readonly [string, string, string, string]
  detail: (boardId: string) => readonly [string, string, string, string, string]
  document: (boardId: string) => readonly [string, string, string, string, string, string]
  revisions: (boardId: string) => readonly [string, string, string, string, string, string]
  members: (boardId: string) => readonly [string, string, string, string, string, string]
  shares: (boardId: string) => readonly [string, string, string, string, string, string]
} {
  const root: string = workspaceQueryKeys.all[0]
  return {
    all: [root, workspaceId, 'boards'],
    list: [root, workspaceId, 'boards', 'list'],
    detail: (boardId: string) => [root, workspaceId, 'boards', 'detail', boardId],
    document: (boardId: string) => [root, workspaceId, 'boards', 'document', boardId, 'snapshot'],
    revisions: (boardId: string) => [root, workspaceId, 'boards', 'revisions', boardId, 'list'],
    members: (boardId: string) => [root, workspaceId, 'boards', 'members', boardId, 'list'],
    shares: (boardId: string) => [root, workspaceId, 'boards', 'shares', boardId, 'list'],
  }
}
