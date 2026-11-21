import { InvalidBoardStateError } from '../../core/document/latestBoardState'
import {
  BOARD_STATE_SCHEMA_VERSION,
  type BoardSnapshot,
  type BoardState,
  type ImageNodeItem,
} from '../../core/document/nodes'
import { readLocalStorageItem, writeLocalStorageItem } from '../../../../core/utils/storage'
import { normalizeBoardState } from './boardStateSerialization'
import { getBoardScope, isCompleteBoardScope, scopedBoardStorageKey } from '../catalog/scope'
import { isBoardAccessLost } from '../revocation/accessLoss'

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
  // Strict cutover: unknown fields (including the retired preview cache)
  // fail validation. Undecodable bytes are rejected and dropped: reads skip
  // them, writes only emit validated content, and any leftover is discarded
  // by the next write. Explicit deletion of valid entries goes through
  // `deleteBoardState`.
  const result = normalizeBoardState(item, {
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
  options: { throwOnInvalidBoardId?: string; storageKey?: string } = {},
): Record<string, BoardState> {
  if (typeof window === 'undefined') return {}
  const scopedKey = options.storageKey ?? getScopedStorageKey()
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

function writeStateMap(
  stateMap: Record<string, BoardState>,
  scopedKey = getScopedStorageKey(),
): boolean {
  // Strict-only write: only validated content is emitted. Entries absent
  // from the next map — including undecodable leftovers — are dropped.
  if (typeof window === 'undefined') return true
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

export function replaceBoardStateMap(stateMap: Record<string, BoardState>) {
  const normalized: Record<string, BoardState> = {}
  Object.entries(stateMap).forEach(([boardId, state]) => {
    const validState = toValidBoardState(state, 'local-state-replace', { boardId })
    if (!validState) return
    normalized[normalizeBoardId(boardId)] = validState
  })
  writeStateMap(normalized)
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
  const savedState = mergeAcceptedBoardTaskIdentities(validState, stateMap[id]?.imageNodes ?? [])
  stateMap[id] = savedState
  // Gallery previews no longer ride the document: without the retired base64
  // cache, a quota failure has nothing left to shed, so it fails closed and
  // the caller surfaces the write error instead of persisting silently.
  if (!writeStateMap(stateMap)) return null
  return savedState
}

type AcceptedBoardTaskSlot = Pick<
  ImageNodeItem,
  'id' | 'pendingTaskId' | 'pendingClientActionId' | 'pendingResultIndex'
>

/** Enrich matching pending actions without replacing newer document content or its timestamp. */
export function mergeAcceptedBoardTaskIdentities(
  target: BoardState,
  accepted: readonly AcceptedBoardTaskSlot[],
): BoardState {
  const byId = new Map(accepted.map((slot) => [slot.id, slot]))
  let didChange = false
  const imageNodes = target.imageNodes.map((node) => {
    const slot = byId.get(node.id)
    if (
      node.outputStatus !== 'running' ||
      !slot?.pendingTaskId ||
      !slot.pendingClientActionId ||
      node.pendingClientActionId !== slot.pendingClientActionId ||
      node.pendingResultIndex !== slot.pendingResultIndex ||
      (node.pendingTaskId && node.pendingTaskId !== slot.pendingTaskId) ||
      node.pendingTaskId === slot.pendingTaskId
    )
      return node
    didChange = true
    return { ...node, pendingTaskId: slot.pendingTaskId }
  })
  return didChange ? { ...target, imageNodes } : target
}

/**
 * Capture while authorized, before submit. Acceptance may only enrich these
 * already-persisted actions under their original User + Workspace key. It
 * never writes remotely, restores a removed Board/slot, or changes residency.
 */
export function capturePendingBoardTask(input: {
  boardId: string
  snapshot: BoardSnapshot
  resultNodeIds: readonly string[]
  clientActionId: string
}): (taskId: string) => boolean {
  const scope = getBoardScope()
  const boardId = normalizeBoardId(input.boardId)
  const storageKey = scopedBoardStorageKey(STORAGE_KEY_PREFIX, scope)
  if (!boardId || !isCompleteBoardScope(scope) || !storageKey || isBoardAccessLost(boardId)) {
    throw new Error('Cannot preserve pending task recovery outside an authorized Board scope')
  }
  const slots = input.resultNodeIds.map((nodeId, resultIndex) => ({
    id: Number(nodeId.slice('image-'.length)),
    pendingClientActionId: input.clientActionId,
    pendingResultIndex: resultIndex,
  }))
  if (!setBoardState(boardId, input.snapshot)) {
    throw new Error('Cannot preserve pending task recovery in local storage')
  }
  return (taskId) => {
    const stateMap = readRawStateMap({ storageKey })
    const current = stateMap[boardId]
    if (!current) return true
    const merged = mergeAcceptedBoardTaskIdentities(
      current,
      slots.map((slot) => ({ ...slot, pendingTaskId: taskId })),
    )
    if (merged === current) return true
    stateMap[boardId] = merged
    return writeStateMap(stateMap, storageKey)
  }
}

export function deleteBoardState(boardId: string): boolean {
  const id = normalizeBoardId(boardId)
  if (!id) return false
  const stateMap = readRawStateMap()
  if (!(id in stateMap)) return false
  delete stateMap[id]
  return writeStateMap(stateMap)
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
  writeStateMap(stateMap)
  return true
}
