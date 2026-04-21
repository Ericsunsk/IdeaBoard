import { getAuthStorageUserScope } from './auth'

export interface CanvasBoardMeta {
  id: string
  name: string
  createdAt: number
  updatedAt: number
}

const LEGACY_STORAGE_KEY = 'ideaboard.canvas.boards.v1'
const STORAGE_KEY_PREFIX = 'ideaboard.canvas.boards.v2'
const MAX_BOARD_NAME_LEN = 60

function readStorageValue(key: string): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem(key)
}

function createBoardId(now: number): string {
  return `board_${now}_${Math.random().toString(36).slice(2, 8)}`
}

function normalizeBoardName(name: string): string {
  const trimmed = name.trim().replace(/\s+/g, ' ')
  if (!trimmed) return ''
  return trimmed.slice(0, MAX_BOARD_NAME_LEN)
}

function toValidBoardMeta(item: unknown): CanvasBoardMeta | null {
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
  const scope = getAuthStorageUserScope()
  if (!scope) return null
  return `${STORAGE_KEY_PREFIX}.${scope}`
}

function migrateLegacyBoardsIfNeeded(scopedKey: string): string | null {
  const scopedRaw = readStorageValue(scopedKey)
  if (scopedRaw) return scopedRaw
  const legacyRaw = readStorageValue(LEGACY_STORAGE_KEY)
  if (!legacyRaw) return null
  localStorage.setItem(scopedKey, legacyRaw)
  localStorage.removeItem(LEGACY_STORAGE_KEY)
  return legacyRaw
}

function readRawBoards(): CanvasBoardMeta[] {
  if (typeof window === 'undefined') return []
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return []
  const raw = migrateLegacyBoardsIfNeeded(scopedKey) || readStorageValue(scopedKey)
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map(toValidBoardMeta).filter((item): item is CanvasBoardMeta => !!item)
  } catch {
    return []
  }
}

function writeRawBoards(boards: CanvasBoardMeta[]) {
  if (typeof window === 'undefined') return
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return
  localStorage.setItem(scopedKey, JSON.stringify(boards))
}

function compareByRecentDesc(a: CanvasBoardMeta, b: CanvasBoardMeta): number {
  return b.updatedAt - a.updatedAt
}

export function listCanvasBoards(): CanvasBoardMeta[] {
  return readRawBoards().sort(compareByRecentDesc)
}

export function getCanvasBoardById(boardId: string): CanvasBoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  return boards.find(board => board.id === id) || null
}

export function createCanvasBoard(name = 'Untitled'): CanvasBoardMeta {
  const now = Date.now()
  const boards = readRawBoards()
  const board: CanvasBoardMeta = {
    id: createBoardId(now),
    name: normalizeBoardName(name) || 'Untitled',
    createdAt: now,
    updatedAt: now,
  }
  boards.unshift(board)
  writeRawBoards(boards)
  return board
}

export function renameCanvasBoard(boardId: string, nextName: string): CanvasBoardMeta | null {
  const id = boardId.trim()
  const name = normalizeBoardName(nextName)
  if (!id || !name) return null
  const boards = readRawBoards()
  const target = boards.find(board => board.id === id)
  if (!target) return null
  target.name = name
  target.updatedAt = Date.now()
  writeRawBoards(boards)
  return target
}

export function touchCanvasBoard(boardId: string): CanvasBoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  const target = boards.find(board => board.id === id)
  if (!target) return null
  target.updatedAt = Date.now()
  writeRawBoards(boards)
  return target
}

export function deleteCanvasBoard(boardId: string): boolean {
  const id = boardId.trim()
  if (!id) return false
  const boards = readRawBoards()
  const nextBoards = boards.filter(board => board.id !== id)
  if (nextBoards.length === boards.length) return false
  writeRawBoards(nextBoards)
  return true
}

export function duplicateCanvasBoard(boardId: string): CanvasBoardMeta | null {
  const id = boardId.trim()
  if (!id) return null
  const boards = readRawBoards()
  const source = boards.find(board => board.id === id)
  if (!source) return null
  const now = Date.now()
  const copy: CanvasBoardMeta = {
    id: createBoardId(now),
    name: normalizeBoardName(`${source.name} Copy`) || 'Untitled Copy',
    createdAt: now,
    updatedAt: now,
  }
  boards.unshift(copy)
  writeRawBoards(boards)
  return copy
}
