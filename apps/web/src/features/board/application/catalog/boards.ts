import { createPrefixedId } from '../../../../core/utils/ids'
import { readLocalStorageItem, writeLocalStorageJson } from '../../../../core/utils/storage'
import { getRecoverableBoardScope, scopedBoardStorageKey } from './scope'

export interface BoardMeta {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number
  archivedAt?: string | undefined
  /** Owning workspace: board identity is User + Workspace scoped (issue #894). */
  workspaceId?: string | undefined
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

/**
 * Registers a server-minted board identity locally (v1 create flow: the
 * server owns id/revision). Replaces any same-id entry; never mints.
 */
export function registerBoardMeta(input: {
  id: string
  name: string
  createdAt?: number
  updatedAt?: number
  revision?: number
  archivedAt?: string | null | undefined
}): BoardMeta | null {
  const id = input.id.trim()
  const name = normalizeBoardName(input.name)
  if (!id || !name) return null
  const now = Date.now()
  const board: BoardMeta = withCurrentWorkspace({
    id,
    name,
    createdAt: Number.isFinite(input.createdAt) ? Number(input.createdAt) : now,
    updatedAt: Number.isFinite(input.updatedAt) ? Number(input.updatedAt) : now,
    ...(input.revision === undefined ? {} : { revision: input.revision }),
    ...(typeof input.archivedAt === 'string' && input.archivedAt
      ? { archivedAt: input.archivedAt }
      : {}),
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
