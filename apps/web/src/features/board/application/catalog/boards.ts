import { getRecoverableAuthStorageUserScope } from '../../../../shared/utils/auth'

export interface BoardMeta {
  id: string
  name: string
  createdAt: number
  updatedAt: number
}

const STORAGE_KEY_PREFIX = 'ideaboard.canvas.boards.v2'
const MAX_BOARD_NAME_LEN = 60
const boardChangeListeners = new Set<() => void>()

export function publishCatalogChange() {
  boardChangeListeners.forEach((listener) => listener())
}

function readStorageValue(key: string): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem(key)
}

function createBoardId(now: number): string {
  return `board_${now}_${Math.random().toString(36).slice(2, 8)}`
}

export function normalizeBoardName(name: string): string {
  const trimmed = name.trim().replace(/\s+/g, ' ')
  if (!trimmed) return ''
  return trimmed.slice(0, MAX_BOARD_NAME_LEN)
}

function toValidBoardMeta(item: unknown): BoardMeta | null {
  if (!item || typeof item !== 'object') return null
  const record = item as Record<string, unknown>
  if (typeof record.id !== 'string') return null
  if (typeof record.name !== 'string') return null
  if (typeof record.createdAt !== 'number') return null
  if (typeof record.updatedAt !== 'number') return null
  return {
    id: record.id,
    name: normalizeBoardName(record.name) || 'Untitled',
    createdAt: record.createdAt,
    updatedAt: record.updatedAt,
  }
}

function getScopedStorageKey(): string | null {
  const scope = getRecoverableAuthStorageUserScope()
  if (!scope) return null
  return `${STORAGE_KEY_PREFIX}.${scope}`
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
  if (typeof window === 'undefined') return
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return
  localStorage.setItem(scopedKey, JSON.stringify(boards))
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

export function replaceBoards(
  boards: BoardMeta[],
  options: { publish?: boolean } = {},
) {
  const normalized = boards
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
  const board: BoardMeta = {
    id: createBoardId(now),
    name: makeUniqueBoardName(name),
    createdAt: now,
    updatedAt: now,
  }
  boards.unshift(board)
  writeRawBoards(boards)
  publishCatalogChange()
  return board
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
  const copy: BoardMeta = {
    id: createBoardId(now),
    name: makeUniqueBoardName(`${source.name} Copy`),
    createdAt: now,
    updatedAt: now,
  }
  boards.unshift(copy)
  writeRawBoards(boards)
  publishCatalogChange()
  return copy
}
