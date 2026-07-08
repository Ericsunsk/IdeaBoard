import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  InvalidBoardStateError,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
} from '../domain'
import { getAuthStorageUserScope } from '../../../shared/utils/auth'
import { normalizeCanvasBoardState, serializeCanvasBoardState } from './boardStateSerialization'

const STORAGE_KEY_PREFIX = 'ideaboard.canvas.boardStates.v2'

function readStorageValue(key: string): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem(key)
}

function getScopedStorageKey(): string | null {
  const scope = getAuthStorageUserScope()
  if (!scope) return null
  return `${STORAGE_KEY_PREFIX}.${scope}`
}

function toValidBoardState(
  item: unknown,
  boundary: string,
  options: { throwOnInvalid?: boolean; boardId?: string } = {},
): CanvasBoardState | null {
  const result = normalizeCanvasBoardState(item, { mode: 'result', boundary })
  if (result.ok) return result.value
  console.warn('[canvas] invalid board state rejected', options.boardId, result.error)
  if (options.throwOnInvalid) {
    throw new InvalidBoardStateError(result.error)
  }
  return null
}

function stripPreviewSnapshotsFromStateMap(
  stateMap: Record<string, CanvasBoardState>,
): Record<string, CanvasBoardState> {
  return Object.fromEntries(
    Object.entries(stateMap).map(([boardId, state]) => [
      boardId,
      serializeCanvasBoardState(state, 'local-preview-fallback'),
    ]),
  )
}

function readRawStateMap(): Record<string, CanvasBoardState> {
  if (typeof window === 'undefined') return {}
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return {}
  const raw = readStorageValue(scopedKey)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    const result: Record<string, CanvasBoardState> = {}
    Object.entries(parsed as Record<string, unknown>).forEach(([boardId, stateLike]) => {
      const state = toValidBoardState(stateLike, 'local-state-read', { boardId })
      if (!state) return
      result[boardId] = state
    })
    return result
  } catch {
    return {}
  }
}

function tryWriteRawStateMap(stateMap: Record<string, CanvasBoardState>): boolean {
  if (typeof window === 'undefined') return true
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return true
  try {
    localStorage.setItem(scopedKey, JSON.stringify(stateMap))
    return true
  } catch (error) {
    console.warn('[canvas] persist board state failed', error)
    return false
  }
}

function writeRawStateMap(stateMap: Record<string, CanvasBoardState>): boolean {
  return tryWriteRawStateMap(stateMap)
}

function writeRawStateMapWithPreviewFallback(
  stateMap: Record<string, CanvasBoardState>,
): Record<string, CanvasBoardState> | null {
  if (tryWriteRawStateMap(stateMap)) return stateMap
  const compactStateMap = stripPreviewSnapshotsFromStateMap(stateMap)
  return tryWriteRawStateMap(compactStateMap) ? compactStateMap : null
}

export function replaceCanvasBoardStateMap(stateMap: Record<string, CanvasBoardState>) {
  const normalized: Record<string, CanvasBoardState> = {}
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

function appendOutputNodeImageUrls(
  target: Set<string>,
  nodes: Array<{ outputImages: Array<{ url: string }> }>,
) {
  nodes.forEach((node) => {
    node.outputImages.forEach((output) => {
      const normalized = `${output.url ?? ''}`.trim()
      if (normalized) {
        target.add(normalized)
      }
    })
  })
}

export function collectCanvasBoardImageUrls(
  snapshot: CanvasBoardSnapshot | null | undefined,
): Set<string> {
  const urls = new Set<string>()
  if (!snapshot) return urls

  snapshot.imageNodes.forEach((node) => {
    const normalized = `${node.sourceImageUrl ?? ''}`.trim()
    if (normalized) {
      urls.add(normalized)
    }
  })

  appendOutputNodeImageUrls(urls, snapshot.imageNodes)
  appendOutputNodeImageUrls(urls, snapshot.fusionNodes)

  return urls
}

export function getCanvasBoardState(boardId: string): CanvasBoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap()
  return stateMap[id] || null
}

export function setCanvasBoardState(
  boardId: string,
  state: CanvasBoardSnapshot,
): CanvasBoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap()
  const existing = stateMap[id]
  const validState = toValidBoardState(
    {
      ...state,
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
      previewSnapshotDataUrl: existing?.previewSnapshotDataUrl,
      previewSnapshotUpdatedAt: existing?.previewSnapshotUpdatedAt,
      previewSnapshotVersion: existing?.previewSnapshotVersion,
      updatedAt: Date.now(),
    },
    'board-save',
    { boardId: id, throwOnInvalid: true },
  )
  if (!validState) return null
  stateMap[id] = validState
  const persistedStateMap = writeRawStateMapWithPreviewFallback(stateMap)
  return persistedStateMap?.[id] ?? serializeCanvasBoardState(validState, 'local-preview-fallback')
}

export function setCanvasBoardPreviewSnapshot(
  boardId: string,
  previewSnapshotDataUrl: string | undefined,
  previewSnapshotUpdatedAt: number | undefined,
  previewSnapshotVersion: number | undefined,
): CanvasBoardState | null {
  const id = normalizeBoardId(boardId)
  if (!id) return null
  const stateMap = readRawStateMap()
  const existing = stateMap[id]
  if (!existing) return null
  const validState = toValidBoardState(
    {
      ...existing,
      previewSnapshotDataUrl,
      previewSnapshotUpdatedAt,
      previewSnapshotVersion,
      updatedAt: existing.updatedAt,
    },
    'preview-refresh',
    { boardId: id },
  )
  if (!validState) return null
  stateMap[id] = validState
  writeRawStateMap(stateMap)
  return validState
}

export function deleteCanvasBoardState(boardId: string): boolean {
  const id = normalizeBoardId(boardId)
  if (!id) return false
  const stateMap = readRawStateMap()
  if (!stateMap[id]) return false
  delete stateMap[id]
  writeRawStateMap(stateMap)
  return true
}

export function duplicateCanvasBoardState(sourceBoardId: string, targetBoardId: string): boolean {
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

export function isImageUrlReferencedInOtherCanvasBoardStates(
  url: string,
  excludeBoardId?: string,
): boolean {
  const targetURL = url.trim()
  if (!targetURL) return false
  const excluded = excludeBoardId?.trim() || ''
  const stateMap = readRawStateMap()
  return Object.entries(stateMap).some(([boardId, state]) => {
    if (excluded && boardId === excluded) return false
    return collectCanvasBoardImageUrls(state).has(targetURL)
  })
}
