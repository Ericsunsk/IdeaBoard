import {
  archiveBoard,
  createBoard,
  getBoard,
  getBoardAccess,
  getBoardDocument,
  getWorkspaceBoardBundle,
  listBoards,
  issueBoardRealtimeTicket,
  replaceBoardDocument,
  restoreBoard,
  updateBoard,
  updateBoardAccess,
} from '../../../../api/generated-query/public/boards/boards'
import type { Board } from '../../../../api/generated-query/public/model/board'
import type { BoardAccessState } from '../../../../api/generated-query/public/model/boardAccessState'
import type { BoardCapabilities } from '../../../../api/generated-query/public/model/boardCapabilities'
import type { WorkspaceAccess } from '../../../../api/generated-query/public/model/workspaceAccess'
import type { BoardDocument } from '../../../../api/generated-query/public/model/boardDocument'
import { boardOperationIdempotencyKey } from '@/api/http/idempotencyKey'
import { getBoardScope, isBoardScopeCurrent, type BoardScope } from '../catalog/scope'
import { withSignal } from '../../../../core/utils/request'
import { isOpaqueIdWithPrefix } from '../../../../core/utils/opaqueIds'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../../../../core/utils/workspaceId'
import { PAGE_SIZES } from '../../../../core/config/pagination'
import { getWorkspaceSelectionStore } from '../../../workspace'
import { parsePreviewScene, type PreviewScene } from '../../core/preview/scene'
import type { BoardState } from '../../core/document/nodes'

// Workspace board transport (Public Data Plane, `/api/v1`).
//
// Sole importer of the generated `public/boards` boundary; application and
// presentation consume boards through this adapter, never generated code
// directly. Every call is workspace-scoped: the workspace id resolves from
// the `features/workspace` selection store (personal workspace default) and
// fail-closed before any network call when no workspace is selected — the
// backend never sees an inferred workspace.
//
// Public Data Plane semantics:
// - list: cursor-paginated board metas (no documents inline).
// - document: separate GET/PUT snapshot with `If-Match: <revision>`
//   optimistic concurrency; stale revisions render 409
//   `board_revision_conflict`.
// - rename: PATCH name (no document write); save: PUT document.
// - delete: POST archive (already-archived reads 404); restore reverts it.
//   There is no hard-delete endpoint.
// - access: boards carry server-computed `capabilities` (read/write/
//   manage/generate), `access_state`, `workspace_access`, and
//   `relationship` (is_creator/has_direct_grant).
//   This adapter consumes them as-is and never recomputes roles: no local
//   RBAC copy exists. Missing capability facts fail closed to read-only.
// - writes requiring `Idempotency-Key` (create/replace/archive/restore)
//   mint a fresh per-attempt key unless the caller passes one, so
//   retries of the same attempt replay instead of duplicating.
//
// The board document payload shape is owned by the board domain
// (`BoardState`); this module only moves it across the wire.

export class BoardWorkspaceUnavailableError extends Error {
  constructor() {
    super('Board workspace is unavailable. Select a workspace before syncing boards.')
    this.name = 'BoardWorkspaceUnavailableError'
  }
}

/** The server answered with a board envelope this client cannot use. */
export class BoardPayloadError extends Error {
  constructor(field: string) {
    super(`Invalid board payload: ${field}`)
    this.name = 'BoardPayloadError'
  }
}

/** Fail-closed workspace resolution: throws before any network call when unset. */
export type BoardWorkspaceIdResolver = () => string | null

function defaultBoardWorkspaceIdResolver(): string | null {
  try {
    return getWorkspaceSelectionStore().getSelectedId()
  } catch {
    return null
  }
}

export function requireBoardWorkspaceId(
  explicit?: string | null,
  resolve?: BoardWorkspaceIdResolver,
): string {
  return requireSharedWorkspaceId(
    explicit?.trim() ? explicit : (resolve ?? defaultBoardWorkspaceIdResolver)(),
    () => new BoardWorkspaceUnavailableError(),
  )
}

export type WorkspaceBoardMeta = {
  id: string
  name: string
  revision: number
  schemaVersion: number
  archivedAt?: string | undefined
  createdAt: number
  updatedAt: number
  /**
   * Opaque content identity for the accepted persisted snapshot. Gallery
   * skips re-render when it is unchanged; empty when the server omits it.
   */
  contentVersion: string
  /**
   * Strict-parsed server Scene projection (contract v1). Null when the
   * server omits the scene or when it fails closed validation — the card
   * then renders the empty layout state, never a guessed projection.
   */
  previewScene: PreviewScene | null
  /**
   * Server-computed capabilities for the caller, consumed as-is. The
   * adapter never recomputes roles from membership or grants (no local
   * RBAC copy): Canvas renders editable vs read-only from these flags.
   */
  capabilities: BoardCapabilities
  /** Server-derived visibility: private | workspace. */
  accessState: BoardAccessState
  /** Sharing gate and common role for selected members. */
  workspaceAccess: WorkspaceAccess
  /** Whether the caller is the board creator (implicit owner). */
  isCreator: boolean
  /**
   * Whether the caller is explicitly selected for sharing, consumed from
   * `relationship.has_direct_grant`.
   */
  hasDirectGrant: boolean
  creatorUserId: string
  /**
   * Workspace-visible owner avatar public URL (server-enriched under the
   * member-list rule, omitted when unavailable). Display-only: never used
   * as an identity or permission fact.
   */
  ownerAvatarUrl?: string | undefined
}

export type WorkspaceBoardDocument = {
  boardId: string
  revision: number
  schemaVersion: number
  document: unknown
  updatedAt: number
}

export type SaveWorkspaceBoardResult = {
  board: WorkspaceBoardMeta
  document: WorkspaceBoardDocument
  newRevision: number
}

export type BoardsRequestOptions = {
  idempotencyKey?: string | undefined
  requestId?: string | undefined
  signal?: AbortSignal | null | undefined
}

function requestConfig(
  options: BoardsRequestOptions | undefined,
  headerOptions?: {
    idempotencyKey?: string | undefined
    ifMatch?: number | undefined
    accessIfMatch?: string | undefined
  },
): { headers?: Record<string, string>; signal?: AbortSignal } {
  const headers: Record<string, string> = {}
  const idempotencyKey = options?.idempotencyKey ?? headerOptions?.idempotencyKey
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
  if (options?.requestId) headers['Request-Id'] = options.requestId
  if (headerOptions?.accessIfMatch != null) headers['If-Match'] = headerOptions.accessIfMatch
  else if (headerOptions?.ifMatch != null) headers['If-Match'] = String(headerOptions.ifMatch)
  const signal = options?.signal ?? undefined
  return {
    ...(Object.keys(headers).length > 0 ? { headers } : {}),
    ...withSignal(signal),
  }
}

function toTimestampMs(value: string, field: string): number {
  const parsed = Date.parse(value)
  if (!Number.isFinite(parsed)) throw new BoardPayloadError(field)
  return parsed
}

function toWorkspaceBoardCapabilities(board: Board): BoardCapabilities {
  const caps = board.capabilities
  if (!caps || typeof caps !== 'object') {
    // Fail closed to read-only when the server omits the capability fact:
    // viewers still read, nothing renders an editable affordance.
    return { read: true, write: false, manage: false, generate: false }
  }
  return {
    read: caps.read === true,
    write: caps.write === true,
    manage: caps.manage === true,
    generate: caps.generate === true,
  }
}

function toWorkspaceBoardAccessState(board: Board): BoardAccessState {
  return board.access_state === 'workspace' ? board.access_state : 'private'
}

function toWorkspaceAccess(board: Board): WorkspaceAccess {
  return board.workspace_access === 'viewer' || board.workspace_access === 'editor'
    ? board.workspace_access
    : 'none'
}

function toWorkspaceBoardMeta(board: Board): WorkspaceBoardMeta {
  const id = board.id?.trim()
  if (!id) throw new BoardPayloadError('id')
  const revision = Number(board.revision)
  if (!Number.isFinite(revision) || revision < 1) throw new BoardPayloadError('revision')
  return {
    id,
    name: board.name,
    revision,
    schemaVersion: Number(board.schema_version),
    ...(board.archived_at == null || board.archived_at === ''
      ? {}
      : { archivedAt: board.archived_at }),
    createdAt: toTimestampMs(board.created_at, 'created_at'),
    updatedAt: toTimestampMs(board.updated_at, 'updated_at'),
    contentVersion: typeof board.content_version === 'string' ? board.content_version : '',
    previewScene: parsePreviewScene(board.preview_scene ?? null),
    capabilities: toWorkspaceBoardCapabilities(board),
    accessState: toWorkspaceBoardAccessState(board),
    workspaceAccess: toWorkspaceAccess(board),
    isCreator: board.relationship?.is_creator === true,
    hasDirectGrant: board.relationship?.has_direct_grant === true,
    creatorUserId: toUserPublicId(board.creator_user_id, 'creator_user_id'),
    ...toOwnerAvatarUrl(board.owner_avatar),
  }
}

function toOwnerAvatarUrl(value: unknown): { ownerAvatarUrl: string } | Record<string, never> {
  if (typeof value !== 'string') return {}
  const trimmed = value.trim()
  return trimmed ? { ownerAvatarUrl: trimmed } : {}
}

function toWorkspaceBoardDocument(document: BoardDocument): WorkspaceBoardDocument {
  const boardId = document.board_id?.trim()
  if (!boardId) throw new BoardPayloadError('board_id')
  const revision = Number(document.revision)
  if (!Number.isFinite(revision) || revision < 1) {
    throw new BoardPayloadError('revision')
  }
  return {
    boardId,
    revision,
    schemaVersion: Number(document.schema_version),
    document: document.document,
    updatedAt: toTimestampMs(document.updated_at, 'updated_at'),
  }
}

const BOARD_LIST_PAGE_SIZE = PAGE_SIZES.dense
const BOARD_LIST_MAX_PAGES = 50

/**
 * The catalog walk hit the bounded page budget with more pages pending:
 * an explicit incomplete signal, never a silent truncation. Callers must
 * preserve local drafts and never treat absence as deletion on this path.
 */
export class BoardCatalogIncompleteError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'BoardCatalogIncompleteError'
  }
}

/** The User + Workspace scope moved mid-walk: the partial walk is dropped. */
export class BoardCatalogScopeChangedError extends Error {
  constructor() {
    super('BOARD_CATALOG_SCOPE_CHANGED')
    this.name = 'BoardCatalogScopeChangedError'
  }
}

/**
 * Binds one walk page to the Workspace + caller scope captured before the
 * first page: a User or Workspace switch mid-walk aborts the walk so a
 * mixed-scope snapshot can never seed the catalog.
 */
function assertCatalogWalkScope(scope: BoardScope): void {
  if (!isBoardScopeCurrent(scope)) throw new BoardCatalogScopeChangedError()
}

/**
 * Lists every accessible board meta in the workspace (cursor loop, bounded).
 *
 * Complete-or-throw: only a walk that traverses every page with no
 * truncation is complete. The walk binds to the Workspace + caller scope
 * captured before the first page — a User or Workspace switch mid-walk
 * aborts with `BoardCatalogScopeChangedError` so a mixed-scope snapshot
 * can never seed the catalog. When the scope already moved before the
 * first page, the walk fails fast without issuing any request (callers
 * must not assume a request fires). Offset cursors are not stable snapshots
 * under concurrent directory/authorization writes; a completed walk is a
 * point-in-time view and live convergence rides SSE invalidation plus the
 * next hydrate. Exhausting `BOARD_LIST_MAX_PAGES` with `has_more` pending
 * throws `BoardCatalogIncompleteError`, and any page failure propagates —
 * both keep absence from ever reading as deletion or revocation downstream.
 */
export async function fetchWorkspaceBoards(
  workspaceId?: string | null,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta[]> {
  const workspace = requireBoardWorkspaceId(workspaceId ?? null)
  const scope = { ...getBoardScope(), workspaceId: workspace }
  const items: WorkspaceBoardMeta[] = []
  let cursor: string | undefined
  for (let page = 0; page < BOARD_LIST_MAX_PAGES; page += 1) {
    // The per-page request already carries the signal; stop the loop itself
    // promptly when the caller aborts between pages.
    options.signal?.throwIfAborted()
    assertCatalogWalkScope(scope)
    const response = await listBoards(
      workspace,
      {
        ...(cursor == null ? {} : { cursor }),
        page_size: BOARD_LIST_PAGE_SIZE,
      },
      requestConfig(options),
    )
    for (const board of response.data ?? []) items.push(toWorkspaceBoardMeta(board))
    const meta = response.meta
    const nextCursor = meta?.next_cursor?.trim()
    if (!meta?.has_more || !nextCursor) {
      assertCatalogWalkScope(scope)
      return items
    }
    cursor = nextCursor
  }
  throw new BoardCatalogIncompleteError(
    `Board catalog walk truncated after ${BOARD_LIST_MAX_PAGES} pages`,
  )
}

/** Reads one board's canonical document snapshot (revision + payload). */
export async function fetchWorkspaceBoardDocument(
  workspaceId: string | null | undefined,
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardDocument> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new BoardPayloadError('id')
  const envelope = await getBoardDocument(workspace, normalizedBoardId, requestConfig(options))
  return toWorkspaceBoardDocument(envelope.data)
}

/** Creates a board at revision 1 with the empty document snapshot. */
export async function createWorkspaceBoard(
  workspaceId: string | null | undefined,
  name: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const envelope = await createBoard(
    workspace,
    { data: { name } },
    requestConfig(options, {
      idempotencyKey: boardOperationIdempotencyKey('create'),
    }),
  )
  return toWorkspaceBoardMeta(envelope.data)
}

export type SaveWorkspaceBoardInput = {
  state: BoardState
  baseRevision: number
}

/**
 * Saves a board document only: replaces the document snapshot via PUT with
 * `If-Match: <baseRevision>`, then reads back the board meta. Ordinary
 * Canvas document saves (autosave included) never rename: no PATCH is sent,
 * so a stale local name can never overwrite a collaborator rename. Stale
 * revisions render 409 `board_revision_conflict` for the sync rollback
 * policy.
 */
export async function saveWorkspaceBoard(
  workspaceId: string | null | undefined,
  boardId: string,
  input: SaveWorkspaceBoardInput,
  options: BoardsRequestOptions = {},
): Promise<SaveWorkspaceBoardResult> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new BoardPayloadError('id')
  const { schemaVersion, ...payload } = input.state
  const envelope = await replaceBoardDocument(
    workspace,
    normalizedBoardId,
    {
      data: {
        schema_version: schemaVersion,
        payload: payload as unknown as Record<string, unknown>,
      },
    },
    requestConfig(options, {
      idempotencyKey: boardOperationIdempotencyKey('write'),
      ifMatch: input.baseRevision,
    }),
  )
  const document = toWorkspaceBoardDocument(envelope.data)
  const boardEnvelope = await getBoard(workspace, normalizedBoardId, requestConfig(options))
  const board = toWorkspaceBoardMeta(boardEnvelope.data)
  return { board, document, newRevision: document.revision }
}

/**
 * Renames a board via PATCH (no document write). Sole name-write path for
 * explicit rename commands (canvas header, gallery); ordinary document
 * saves never call this. Blank or overlong names render 422; writes
 * against an archived board render 409 `board_archived`.
 */
export async function renameWorkspaceBoard(
  workspaceId: string | null | undefined,
  boardId: string,
  name: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new BoardPayloadError('id')
  const envelope = await updateBoard(
    workspace,
    normalizedBoardId,
    { data: { name } },
    requestConfig(options),
  )
  return toWorkspaceBoardMeta(envelope.data)
}

/** Archives a board (the v1 delete semantic; already-archived reads 404). */
export async function archiveWorkspaceBoard(
  workspaceId: string | null | undefined,
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new BoardPayloadError('id')
  const envelope = await archiveBoard(
    workspace,
    normalizedBoardId,
    requestConfig(options, {
      idempotencyKey: boardOperationIdempotencyKey('archive'),
    }),
  )
  return toWorkspaceBoardMeta(envelope.data)
}

/** Restores an archived board (live boards read 404). */
export async function restoreWorkspaceBoard(
  workspaceId: string | null | undefined,
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new BoardPayloadError('id')
  const envelope = await restoreBoard(
    workspace,
    normalizedBoardId,
    requestConfig(options, {
      idempotencyKey: boardOperationIdempotencyKey('restore'),
    }),
  )
  return toWorkspaceBoardMeta(envelope.data)
}

/** Reads one board meta (carries `archivedAt` for the canvas archived banner). */
export async function fetchWorkspaceBoardMeta(
  workspaceId: string | null | undefined,
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const normalizedBoardId = boardId.trim()
  if (!normalizedBoardId) throw new BoardPayloadError('id')
  const envelope = await getBoard(workspace, normalizedBoardId, requestConfig(options))
  return toWorkspaceBoardMeta(envelope.data)
}

export type ShareWorkspaceAccess = 'none' | 'viewer' | 'editor'

export type WorkspaceBoardAccessFact = {
  workspaceAccess: ShareWorkspaceAccess
  memberUserIds: string[]
  accessVersion: number
  accessState: BoardAccessState
  archivedAt?: string | undefined
}

export type ShareAccessWrite = {
  expectedVersion: number
  idempotencyKey: string
}

export type ShareAccessInput = Pick<WorkspaceBoardAccessFact, 'workspaceAccess' | 'memberUserIds'>

function toUserPublicId(value: unknown, field: string): string {
  if (typeof value === 'string' && isOpaqueIdWithPrefix(value.trim(), 'usr')) return value.trim()
  throw new BoardPayloadError(field)
}

function toAccessVersion(value: unknown, field: string): number {
  const version = typeof value === 'number' ? value : Number(value)
  if (!Number.isInteger(version) || version < 1) throw new BoardPayloadError(field)
  return version
}

function toShareAccessFact(data: unknown): WorkspaceBoardAccessFact {
  if (!data || typeof data !== 'object') throw new BoardPayloadError('access')
  const record = data as Record<string, unknown>
  if (
    record.workspace_access !== 'none' &&
    record.workspace_access !== 'viewer' &&
    record.workspace_access !== 'editor'
  )
    throw new BoardPayloadError('workspace_access')
  if (record.access_state !== 'private' && record.access_state !== 'workspace')
    throw new BoardPayloadError('access_state')
  if (!Array.isArray(record.member_user_ids)) throw new BoardPayloadError('member_user_ids')
  return {
    workspaceAccess: record.workspace_access,
    memberUserIds: record.member_user_ids.map((value) => toUserPublicId(value, 'member_user_ids')),
    accessVersion: toAccessVersion(record.access_version, 'access_version'),
    accessState: record.access_state,
    ...(typeof record.archived_at === 'string' && record.archived_at
      ? { archivedAt: record.archived_at }
      : {}),
  }
}

export async function fetchBoardAccessFact(
  workspaceId: string | null | undefined,
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardAccessFact> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const id = boardId.trim()
  if (!id) throw new BoardPayloadError('id')
  const envelope = await getBoardAccess(workspace, id, requestConfig(options))
  return toShareAccessFact(envelope.data)
}

export async function replaceBoardSharing(
  workspaceId: string | null | undefined,
  boardId: string,
  input: ShareAccessInput,
  write: ShareAccessWrite,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardAccessFact> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const id = boardId.trim()
  if (!id) throw new BoardPayloadError('id')
  if (!write.idempotencyKey.trim()) throw new BoardPayloadError('idempotencyKey')
  toAccessVersion(write.expectedVersion, 'expectedVersion')
  const envelope = await updateBoardAccess(
    workspace,
    id,
    {
      data: {
        workspace_access: input.workspaceAccess,
        member_user_ids: input.memberUserIds.map((value) =>
          toUserPublicId(value, 'member_user_ids'),
        ),
      },
    },
    requestConfig(options, {
      idempotencyKey: write.idempotencyKey,
      accessIfMatch: `access-${write.expectedVersion}`,
    }),
  )
  return toShareAccessFact(envelope.data)
}

type CachedRealtimeTicket = {
  ticket: string
  expiresAtMs: number
}
const realtimeTicketCache = new Map<string, CachedRealtimeTicket>()

function ticketKey(workspaceId: string, boardId: string): string {
  return `${workspaceId}:${boardId}`
}

function cacheBoardRealtimeTicket(
  workspaceId: string,
  boardId: string,
  ticket: string,
  expiresAt: string,
): void {
  const expiresAtMs = Date.parse(expiresAt)
  if (!Number.isFinite(expiresAtMs) || expiresAtMs <= Date.now()) return
  realtimeTicketCache.set(ticketKey(workspaceId, boardId), { ticket, expiresAtMs })
}

function getCachedBoardRealtimeTicket(workspaceId: string, boardId: string): string | null {
  const key = ticketKey(workspaceId, boardId)
  const cached = realtimeTicketCache.get(key)
  if (!cached) return null
  if (Date.now() >= cached.expiresAtMs - 5_000) {
    realtimeTicketCache.delete(key)
    return null
  }
  return cached.ticket
}

/** Fetches the consolidated board bundle: meta, document snapshot, and realtime ticket in 1 RTT. */
export async function fetchWorkspaceBoardBundle(
  workspaceId: string,
  boardId: string,
  options: BoardsRequestOptions = {},
): Promise<{
  meta: WorkspaceBoardMeta
  document: WorkspaceBoardDocument
}> {
  const workspace = workspaceId.trim()
  const board = boardId.trim()
  if (!workspace || !board) throw new BoardPayloadError('board bundle scope')
  const envelope = await getWorkspaceBoardBundle(workspace, board, requestConfig(options))
  const data = envelope?.data
  if (!data?.meta || !data?.document) throw new BoardPayloadError('board bundle')
  const meta = toWorkspaceBoardMeta(data.meta)
  const document = toWorkspaceBoardDocument(data.document)
  if (data.realtime?.ticket && data.realtime?.expires_at) {
    cacheBoardRealtimeTicket(workspace, board, data.realtime.ticket, data.realtime.expires_at)
  }
  return {
    meta,
    document,
  }
}

/** Mints an opaque room ticket for the explicitly selected workspace Board. */
export async function requestBoardRealtimeTicket(
  workspaceId: string,
  boardId: string,
  options: Pick<BoardsRequestOptions, 'signal'> = {},
): Promise<string> {
  const workspace = workspaceId.trim()
  const board = boardId.trim()
  if (!workspace || !board) throw new BoardPayloadError('realtime ticket scope')
  const cached = getCachedBoardRealtimeTicket(workspace, board)
  if (cached) {
    return cached
  }
  const envelope = await issueBoardRealtimeTicket(workspace, board, requestConfig(options))
  const ticket = envelope?.data?.ticket
  if (typeof ticket !== 'string' || !ticket.trim()) throw new BoardPayloadError('realtime ticket')
  return ticket.trim()
}
