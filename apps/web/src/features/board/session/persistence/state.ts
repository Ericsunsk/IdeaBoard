import { InvalidBoardStateError } from '../../core/document/latestBoardState'
import {
  BOARD_STATE_SCHEMA_VERSION,
  stripLegacyBoardPreviewFields,
  type BoardSnapshot,
  type BoardState,
} from '../../core/document/nodes'
import { readLocalStorageItem, writeLocalStorageItem } from '../../../../core/utils/storage'
import { normalizeBoardState } from './boardStateSerialization'
import { scopedBoardStorageKey } from '../catalog/scope'

const STORAGE_KEY_PREFIX = 'ideaboard.canvas.boardStates.v2'

function readStorageValue(key: string): string | null {
  return readLocalStorageItem(key, (error) => {
    console.warn('[canvas] read board state failed', error)
  })
}

function getScopedStorageKey(): string | null {
  return scopedBoardStorageKey(STORAGE_KEY_PREFIX)
}

function toValidBoardState(
  item: unknown,
  boundary: string,
  options: { throwOnInvalid?: boolean; boardId?: string } = {},
): BoardState | null {
  // One-way cleanup: drafts persisted by old clients still carry the retired
  // preview cache. Strip it before strict validation so content survives;
  // the cache itself is never read back (see `stripLegacyBoardPreviewFields`).
  const result = normalizeBoardState(stripLegacyBoardPreviewFields(item), {
    mode: 'result',
    boundary,
  })
  if (result.isOk) return result.value
  console.warn('[canvas] invalid board state rejected', options.boardId, result.error)
  if (options.throwOnInvalid) {
    throw new InvalidBoardStateError(result.error)
  }
  return null
}

function readRawStateMap(
  options: { throwOnInvalidBoardId?: string } = {},
): Record<string, BoardState> {
  if (typeof window === 'undefined') return {}
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return {}
  const raw = readStorageValue(scopedKey)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    const result: Record<string, BoardState> = {}
    Object.entries(parsed as Record<string, unknown>).forEach(([boardId, stateLike]) => {
      const normalizedBoardId = normalizeBoardId(boardId)
      const state = toValidBoardState(stateLike, 'local-state-read', {
        boardId: normalizedBoardId,
        throwOnInvalid: normalizedBoardId === options.throwOnInvalidBoardId,
      })
      if (!state) return
      result[normalizedBoardId] = state
    })
    return result
  } catch (error) {
    console.warn('[canvas] read board states failed', error)
    if (options.throwOnInvalidBoardId) throw error
    return {}
  }
}

function tryWriteRawStateMap(stateMap: Record<string, BoardState>): boolean {
  if (typeof window === 'undefined') return true
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return true
  try {
    const raw = JSON.stringify(stateMap)
    return writeLocalStorageItem(scopedKey, raw, (error) => {
      console.warn('[canvas] persist board state failed', error)
    })
  } catch (error) {
    console.warn('[canvas] persist board state failed', error)
    return false
  }
}

function writeRawStateMap(stateMap: Record<string, BoardState>): boolean {
  return tryWriteRawStateMap(stateMap)
}

export function replaceBoardStateMap(stateMap: Record<string, BoardState>) {
  const normalized: Record<string, BoardState> = {}
  Object.entries(stateMap).forEach(([boardId, state]) => {
    const validState = toValidBoardState(state, 'local-state-replace', { boardId })
    if (!validState) return
    normalized[normalizeBoardId(boardId)] = validState
  })
  writeRawStateMap(normalized)
}

function normalizeBoardId(boardId: string): string {
  return boardId.trim()
}

export function getBoardState(boardId: string): BoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap()
  return stateMap[id] || null
}

export function getBoardStates(boardIds: readonly string[]): Record<string, BoardState | null> {
  const stateMap = readRawStateMap()
  const result: Record<string, BoardState | null> = {}
  boardIds.forEach((boardId) => {
    const id = normalizeBoardId(boardId)
    if (!id) return
    result[boardId] = stateMap[id] || null
  })
  return result
}

export function loadBoardState(boardId: string): BoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap({ throwOnInvalidBoardId: id })
  return stateMap[id] || null
}

export function setBoardState(boardId: string, state: BoardSnapshot): BoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap()
  const validState = toValidBoardState(
    {
      ...state,
      schemaVersion: BOARD_STATE_SCHEMA_VERSION,
      updatedAt: Date.now(),
    },
    'board-save',
    { boardId: id, throwOnInvalid: true },
  )
  if (!validState) return null
  stateMap[id] = validState
  // Gallery previews no longer ride the document: without the retired base64
  // cache, a quota failure has nothing left to shed, so it fails closed and
  // the caller surfaces the write error instead of persisting silently.
  if (!writeRawStateMap(stateMap)) return null
  return validState
}

export function deleteBoardState(boardId: string): boolean {
  const id = normalizeBoardId(boardId)
  if (!id) return false
  const stateMap = readRawStateMap()
  if (!stateMap[id]) return false
  delete stateMap[id]
  writeRawStateMap(stateMap)
  return true
}

export function duplicateBoardState(sourceBoardId: string, targetBoardId: string): boolean {
  const sourceId = normalizeBoardId(sourceBoardId)
  const targetId = normalizeBoardId(targetBoardId)
  if (!sourceId || !targetId) return false
  const stateMap = readRawStateMap()
  const sourceState = stateMap[sourceId]
  if (!sourceState) return false
  const copiedState = toValidBoardState(
    {
      ...sourceState,
      updatedAt: Date.now(),
    },
    'board-duplicate',
    { boardId: sourceId },
  )
  if (!copiedState) return false
  stateMap[targetId] = copiedState
  writeRawStateMap(stateMap)
  return true
}
