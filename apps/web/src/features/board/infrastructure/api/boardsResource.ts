import {
  archiveBoard,
  createBoard,
  getBoard,
  getBoardDocument,
  joinBoardByToken,
  listBoards,
  replaceBoardDocument,
  restoreBoard,
  updateBoard,
} from '../../../../shared/api/generated-query/public/boards/boards'
import type { Board } from '../../../../shared/api/generated-query/public/model/board'
import type { BoardDocument } from '../../../../shared/api/generated-query/public/model/boardDocument'
import { boardOperationIdempotencyKey } from '../../../../shared/api/idempotencyKey'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../../../../shared/utils/workspaceId'
import { PAGE_SIZES } from '../../../../shared/config/pagination'
import { getWorkspaceSelectionStore } from '../../../workspace'
import type { BoardState } from '../../domain'

// Workspace board transport (Public Data Plane, `/api/v1`).
//
// Sole importer of the generated `public/boards` boundary; application and
// presentation consume boards through this adapter, never generated code
// directly. Every call is workspace-scoped: the workspace id resolves from
// the `features/workspace` selection store (personal workspace default) and
// fail-closed before any network call when no workspace is selected — the
// backend never sees an inferred workspace.
//
// v1 semantics (NOT the deleted legacy `/user/boards` shapes):
// - list: cursor-paginated board metas (no documents inline).
// - document: separate GET/PUT snapshot with `If-Match: <revision>`
//   optimistic concurrency; stale revisions render 409
//   `board_revision_conflict`.
// - rename: PATCH name (no document write); save: PUT document.
// - delete: POST archive (already-archived reads 404); restore reverts it.
//   There is no hard-delete endpoint.
// - join: POST share bearer under the target workspace; the bearer grants
//   workspace membership alongside the board grant.
// - writes requiring `Idempotency-Key` (create/replace/archive/restore/
//   join) mint a fresh per-attempt key unless the caller passes one, so
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
export function requireBoardWorkspaceId(explicit?: string | null): string {
  return requireSharedWorkspaceId(
    explicit?.trim() ? explicit : getWorkspaceSelectionStore().getSelectedId(),
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
}

export type WorkspaceBoardDocument = {
  boardId: string
  revision: number
  schemaVersion: number
  document: unknown
  updatedAt: number
}

export type WorkspaceBoardRecord = {
  meta: WorkspaceBoardMeta
  state: BoardState
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
  headerOptions?: { idempotencyKey?: string | undefined; ifMatch?: number | undefined },
): { headers?: Record<string, string>; signal?: AbortSignal } {
  const headers: Record<string, string> = {}
  const idempotencyKey = options?.idempotencyKey ?? headerOptions?.idempotencyKey
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey
  if (options?.requestId) headers['Request-Id'] = options.requestId
  if (headerOptions?.ifMatch != null) headers['If-Match'] = String(headerOptions.ifMatch)
  const signal = options?.signal ?? undefined
  return {
    ...(Object.keys(headers).length > 0 ? { headers } : {}),
    ...(signal == null ? {} : { signal }),
  }
}

function toTimestampMs(value: string, field: string): number {
  const parsed = Date.parse(value)
  if (!Number.isFinite(parsed)) throw new BoardPayloadError(field)
  return parsed
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
  }
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

/** Lists every accessible board meta in the workspace (cursor loop, bounded). */
export async function fetchWorkspaceBoards(
  workspaceId?: string | null,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta[]> {
  const workspace = requireBoardWorkspaceId(workspaceId ?? null)
  const items: WorkspaceBoardMeta[] = []
  let cursor: string | undefined
  for (let page = 0; page < BOARD_LIST_MAX_PAGES; page += 1) {
    // The per-page request already carries the signal; stop the loop itself
    // promptly when the caller aborts between pages.
    options.signal?.throwIfAborted()
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
    if (!meta?.has_more || !nextCursor) return items
    cursor = nextCursor
  }
  return items
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

/**
 * Joins a board with a share bearer under the target workspace. The caller
 * needs no prior membership; success grants workspace membership alongside
 * the board grant. Unknown/revoked/foreign bearers converge to 404.
 */
export async function joinWorkspaceBoardByShareToken(
  workspaceId: string | null | undefined,
  token: string,
  options: BoardsRequestOptions = {},
): Promise<WorkspaceBoardMeta> {
  const workspace = requireBoardWorkspaceId(workspaceId)
  const envelope = await joinBoardByToken(
    workspace,
    { data: { token } },
    requestConfig(options, {
      idempotencyKey: boardOperationIdempotencyKey('join'),
    }),
  )
  return toWorkspaceBoardMeta(envelope.data)
}
