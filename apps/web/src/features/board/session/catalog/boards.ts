import { createPrefixedId } from '../../../../core/utils/ids'
import { readLocalStorageItem, writeLocalStorageJson } from '../../../../core/utils/storage'
import { getRecoverableBoardScope, scopedBoardStorageKey } from './scope'

/**
 * Server-computed board capabilities, mirrored from the Public API
 * `BoardCapabilities` schema. Consumed verbatim from board payloads;
 * never recomputed locally. (Structural mirror — not a generated import:
 * only `session/adapters/boardsResource.ts` may import the
 * generated-query boundary.)
 * @externalContract Public API BoardCapabilities: consumed verbatim.
 */
export interface BoardAccessCapabilities {
  read: boolean
  write: boolean
  manage: boolean
  generate: boolean
}

/** Derived sharing posture, mirrored from the Public API schema. */
export type BoardAccessPosture = 'private' | 'workspace'

/** Common sharing gate and selected-member role from the Public API. */
export type BoardWorkspaceAccess = 'none' | 'viewer' | 'editor'

export interface BoardMeta {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number
  archivedAt?: string | undefined
  /** Owning workspace: board identity is User + Workspace scoped. */
  workspaceId?: string | undefined
  /**
   * Server-computed capabilities, consumed as-is (absent on pre-hydration
   * local boards). Never recomputed locally: no RBAC copy lives here.
   */
  capabilities?: BoardAccessCapabilities | undefined
  /** Derived sharing posture, server-provided alongside capabilities. */
  accessState?: BoardAccessPosture | undefined
  /** Common sharing gate and role, server-provided. */
  workspaceAccess?: BoardWorkspaceAccess | undefined
  /** Whether the caller is the board creator (implicit owner). */
  isCreator?: boolean | undefined
  /**
   * Whether the caller is explicitly selected for Board sharing
   * (`relationship.has_direct_grant`); absent on unsynced local Boards.
   */
  hasDirectGrant?: boolean | undefined
  /**
   * Creator User public identity (usr_ opaque), carried verbatim from the
   * board payload for owner display; absent on unsynced local Boards.
   * Never used as a permission fact.
   */
  creatorUserId?: string | undefined
  /**
   * Workspace-visible owner avatar public URL (display-only); absent when
   * the server omits it or on unsynced local Boards.
   */
  ownerAvatarUrl?: string | undefined
}

const STORAGE_KEY_PREFIX = 'ideaboard.canvas.boards.v2'
const MAX_BOARD_NAME_LEN = 60
const boardChangeListeners = new Set<() => void>()

export function publishCatalogChange() {
  boardChangeListeners.forEach((listener) => listener())
}

function readStorageValue(key: string): string | null {
  return readLocalStorageItem(key)
}

function createBoardId(): string {
  return createPrefixedId('board')
}

export function normalizeBoardName(name: string): string {
  const trimmed = name.trim().replace(/\s+/g, ' ')
  if (!trimmed) return ''
  return trimmed.slice(0, MAX_BOARD_NAME_LEN)
}

function toValidWorkspaceId(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return normalized ? normalized : undefined
}

function toValidRevision(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return undefined
  return value
}

function toValidArchivedAt(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

function toValidCapabilities(value: unknown): BoardAccessCapabilities | undefined {
  if (!value || typeof value !== 'object') return undefined
  const record = value as Record<string, unknown>
  for (const key of ['read', 'write', 'manage', 'generate']) {
    if (typeof record[key] !== 'boolean') return undefined
  }
  return {
    read: record.read as boolean,
    write: record.write as boolean,
    manage: record.manage as boolean,
    generate: record.generate as boolean,
  }
}

function toValidAccessState(value: unknown): BoardAccessPosture | undefined {
  return value === 'private' || value === 'workspace' ? value : undefined
}

function toValidWorkspaceAccess(value: unknown): BoardWorkspaceAccess | undefined {
  return value === 'none' || value === 'viewer' || value === 'editor' ? value : undefined
}

function toValidHasDirectGrant(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}

function toValidCreatorUserId(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return /^usr_[A-Za-z0-9_-]+$/.test(normalized) ? normalized : undefined
}

function toValidOwnerAvatarUrl(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const normalized = value.trim()
  return normalized ? normalized : undefined
}

function toValidAccessFacts(record: Record<string, unknown>): Partial<BoardMeta> {
  const capabilities = toValidCapabilities(record.capabilities)
  const accessState = toValidAccessState(record.accessState)
  const workspaceAccess = toValidWorkspaceAccess(record.workspaceAccess)
  const isCreator = typeof record.isCreator === 'boolean' ? record.isCreator : undefined
  const hasDirectGrant = toValidHasDirectGrant(record.hasDirectGrant)
  const creatorUserId = toValidCreatorUserId(record.creatorUserId)
  const ownerAvatarUrl = toValidOwnerAvatarUrl(record.ownerAvatarUrl)
  return {
    ...(capabilities === undefined ? {} : { capabilities }),
    ...(accessState === undefined ? {} : { accessState }),
    ...(workspaceAccess === undefined ? {} : { workspaceAccess }),
    ...(isCreator === undefined ? {} : { isCreator }),
    ...(hasDirectGrant === undefined ? {} : { hasDirectGrant }),
    ...(creatorUserId === undefined ? {} : { creatorUserId }),
    ...(ownerAvatarUrl === undefined ? {} : { ownerAvatarUrl }),
  }
}

function toValidBoardMeta(item: unknown): BoardMeta | null {
  if (!item || typeof item !== 'object') return null
  const record = item as Record<string, unknown>
  if (typeof record.id !== 'string') return null
  if (typeof record.name !== 'string') return null
  if (typeof record.createdAt !== 'number') return null
  if (typeof record.updatedAt !== 'number') return null
  const revision = toValidRevision(record.revision)
  const archivedAt = toValidArchivedAt(record.archivedAt)
  const workspaceId = toValidWorkspaceId(record.workspaceId)
  return {
    id: record.id,
    name: normalizeBoardName(record.name) || 'Untitled',
    createdAt: record.createdAt,
    updatedAt: record.updatedAt,
    ...(revision === undefined ? {} : { revision }),
    ...(archivedAt === undefined ? {} : { archivedAt }),
    ...(workspaceId === undefined ? {} : { workspaceId }),
    ...toValidAccessFacts(record),
  }
}

function getScopedStorageKey(): string | null {
  return scopedBoardStorageKey(STORAGE_KEY_PREFIX)
}

/** Stamps the current workspace onto entries missing ownership (never rewrites explicit owners). */
function withCurrentWorkspace(board: BoardMeta): BoardMeta {
  if (board.workspaceId) return board
  const workspaceId = getRecoverableBoardScope().workspaceId
  return workspaceId ? { ...board, workspaceId } : board
}

function readRawBoards(): BoardMeta[] {
  if (typeof window === 'undefined') return []
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return []
  const raw = readStorageValue(scopedKey)
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map(toValidBoardMeta).filter((item): item is BoardMeta => !!item)
  } catch {
    return []
  }
}

function writeRawBoards(boards: BoardMeta[]) {
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return
  writeLocalStorageJson(scopedKey, boards)
}

export function subscribeBoardChanges(listener: () => void): () => void {
  boardChangeListeners.add(listener)
  return () => boardChangeListeners.delete(listener)
}

function compareByRecentDesc(a: BoardMeta, b: BoardMeta): number {
  return b.updatedAt - a.updatedAt
}

export function listBoards(): BoardMeta[] {
  return readRawBoards().sort(compareByRecentDesc)
}

export function replaceBoards(boards: BoardMeta[], options: { publish?: boolean } = {}) {
  const normalized = boards
    .map((board) => withCurrentWorkspace(board))
    .map(toValidBoardMeta)
    .filter((item): item is BoardMeta => !!item)
    .sort(compareByRecentDesc)
  writeRawBoards(normalized)
  if (options.publish !== false) publishCatalogChange()
}

export function getBoardById(boardId: string): BoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  return boards.find((board) => board.id === id) || null
}

export function isBoardNameTaken(name: string, excludeBoardId?: string): boolean {
  const normalized = normalizeBoardName(name)
  if (!normalized) return false
  const excludeId = excludeBoardId?.trim() || ''
  const normalizedLower = normalized.toLocaleLowerCase()
  return readRawBoards().some((board) => {
    if (excludeId && board.id === excludeId) return false
    return normalizeBoardName(board.name).toLocaleLowerCase() === normalizedLower
  })
}

function makeUniqueBoardName(baseName: string, excludeBoardId?: string): string {
  const normalizedBase = normalizeBoardName(baseName) || 'Untitled'
  if (!isBoardNameTaken(normalizedBase, excludeBoardId)) {
    return normalizedBase
  }
  const buildCandidateWithIndex = (index: number): string => {
    const suffix = ` ${index}`
    const maxBaseLen = Math.max(1, MAX_BOARD_NAME_LEN - suffix.length)
    const base = normalizedBase.slice(0, maxBaseLen).trimEnd()
    return normalizeBoardName(`${base}${suffix}`)
  }
  let index = 2
  while (index < 10000) {
    const candidate = buildCandidateWithIndex(index)
    if (candidate && !isBoardNameTaken(candidate, excludeBoardId)) {
      return candidate
    }
    index += 1
  }
  const timestampSuffix = `${Date.now()}`.slice(-6)
  const maxBaseLen = Math.max(1, MAX_BOARD_NAME_LEN - timestampSuffix.length - 1)
  const base = normalizedBase.slice(0, maxBaseLen).trimEnd()
  return normalizeBoardName(`${base} ${timestampSuffix}`)
}

export function createBoard(name = 'Untitled'): BoardMeta {
  const now = Date.now()
  const boards = readRawBoards()
  const board: BoardMeta = withCurrentWorkspace({
    id: createBoardId(),
    name: makeUniqueBoardName(name),
    createdAt: now,
    updatedAt: now,
  })
  boards.unshift(board)
  writeRawBoards(boards)
  publishCatalogChange()
  return board
}

type RegisterBoardMetaInput = {
  id: string
  name: string
  createdAt?: number
  updatedAt?: number
  revision?: number
  archivedAt?: string | null | undefined
  capabilities?: BoardAccessCapabilities | undefined
  accessState?: BoardAccessPosture | undefined
  workspaceAccess?: BoardWorkspaceAccess | undefined
  isCreator?: boolean | undefined
  hasDirectGrant?: boolean | undefined
  creatorUserId?: string | undefined
  ownerAvatarUrl?: string | undefined
}

function toRegisterBoardFacts(input: RegisterBoardMetaInput): Partial<BoardMeta> {
  return {
    ...(input.revision === undefined ? {} : { revision: input.revision }),
    ...(typeof input.archivedAt === 'string' && input.archivedAt
      ? { archivedAt: input.archivedAt }
      : {}),
    ...(input.capabilities === undefined ? {} : { capabilities: input.capabilities }),
    ...(input.accessState === undefined ? {} : { accessState: input.accessState }),
    ...(input.workspaceAccess === undefined ? {} : { workspaceAccess: input.workspaceAccess }),
    ...(input.isCreator === undefined ? {} : { isCreator: input.isCreator }),
    ...(input.hasDirectGrant === undefined ? {} : { hasDirectGrant: input.hasDirectGrant }),
    ...(input.creatorUserId === undefined ? {} : { creatorUserId: input.creatorUserId }),
    ...(input.ownerAvatarUrl === undefined ? {} : { ownerAvatarUrl: input.ownerAvatarUrl }),
  }
}

/**
 * Registers a server-minted board identity locally (v1 create flow: the
 * server owns id/revision). Replaces any same-id entry; never mints.
 */
export function registerBoardMeta(input: RegisterBoardMetaInput): BoardMeta | null {
  const id = input.id.trim()
  const name = normalizeBoardName(input.name)
  if (!id || !name) return null
  const now = Date.now()
  const board: BoardMeta = withCurrentWorkspace({
    id,
    name,
    createdAt: Number.isFinite(input.createdAt) ? Number(input.createdAt) : now,
    updatedAt: Number.isFinite(input.updatedAt) ? Number(input.updatedAt) : now,
    ...toRegisterBoardFacts(input),
  })
  const valid = toValidBoardMeta(board)
  if (!valid) return null
  const boards = readRawBoards().filter((item) => item.id !== id)
  boards.unshift(valid)
  writeRawBoards(boards)
  publishCatalogChange()
  return valid
}

export function renameBoard(boardId: string, nextName: string): BoardMeta | null {
  const id = boardId.trim()
  const name = normalizeBoardName(nextName)
  if (!id || !name) return null
  if (isBoardNameTaken(name, id)) return null
  const boards = readRawBoards()
  const target = boards.find((board) => board.id === id)
  if (!target) return null
  target.name = name
  target.updatedAt = Date.now()
  writeRawBoards(boards)
  publishCatalogChange()
  return target
}

export function touchBoard(boardId: string): BoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  const target = boards.find((board) => board.id === id)
  if (!target) return null
  target.updatedAt = Date.now()
  writeRawBoards(boards)
  publishCatalogChange()
  return target
}

export function setBoardRevision(boardId: string, revision: number): BoardMeta | null {
  const id = boardId.trim()
  if (!id || !Number.isFinite(revision) || revision < 0) return null
  const boards = readRawBoards()
  const target = boards.find((board) => board.id === id)
  if (!target) return null
  target.revision = revision
  writeRawBoards(boards)
  publishCatalogChange()
  return target
}

/**
 * Tracks the server archived flag locally so the canvas can render the
 * archived banner without refetching. `archivedAt` null clears the flag.
 */
export function setBoardArchivedAt(
  boardId: string,
  archivedAt: string | null | undefined,
): BoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  const target = boards.find((board) => board.id === id)
  if (!target) return null
  if (typeof archivedAt === 'string' && archivedAt) {
    target.archivedAt = archivedAt
  } else {
    delete target.archivedAt
  }
  writeRawBoards(boards)
  publishCatalogChange()
  return target
}

export function deleteBoard(boardId: string): boolean {
  const id = boardId.trim()
  if (!id) return false
  const boards = readRawBoards()
  const nextBoards = boards.filter((board) => board.id !== id)
  if (nextBoards.length === boards.length) return false
  writeRawBoards(nextBoards)
  publishCatalogChange()
  return true
}

export function duplicateBoard(boardId: string): BoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  const source = boards.find((board) => board.id === id)
  if (!source) return null
  const now = Date.now()
  const copy: BoardMeta = withCurrentWorkspace({
    id: createBoardId(),
    name: makeUniqueBoardName(`${source.name} Copy`),
    createdAt: now,
    updatedAt: now,
  })
  boards.unshift(copy)
  writeRawBoards(boards)
  publishCatalogChange()
  return copy
}
