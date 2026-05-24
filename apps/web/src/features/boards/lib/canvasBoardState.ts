import type {
  CanvasBoardSnapshot,
  CanvasConnection,
  CanvasViewportState,
  FusionNodeItem,
  ImageNodeItem,
  ImageNodeOutputItem,
  PromptNodeItem,
  ViewpointNodeItem,
} from '../../workspace/types/canvasNodes'
import { getAuthStorageUserScope } from '../../../shared/utils/auth'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'

export type CanvasBoardStatePromptNode = PromptNodeItem
export type CanvasBoardStateViewpointNode = ViewpointNodeItem
export type CanvasBoardStateImageNodeOutput = ImageNodeOutputItem
export type CanvasBoardStateImageNode = ImageNodeItem
export type CanvasBoardStateFusionNode = FusionNodeItem
export type CanvasBoardStateConnection = CanvasConnection
export type CanvasBoardStateViewport = CanvasViewportState

export interface CanvasBoardState extends CanvasBoardSnapshot {
  previewSnapshotDataUrl?: string
  previewSnapshotUpdatedAt?: number
  previewSnapshotVersion?: number
  updatedAt: number
}

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

function toValidBoardState(item: unknown): CanvasBoardState | null {
  const parsed = canvasBoardStateSchema.safeParse(item)
  return parsed.success ? parsed.data : null
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
      const state = toValidBoardState(stateLike)
      if (!state) return
      result[boardId] = state
    })
    return result
  } catch {
    return {}
  }
}

function writeRawStateMap(stateMap: Record<string, CanvasBoardState>) {
  if (typeof window === 'undefined') return
  const scopedKey = getScopedStorageKey()
  if (!scopedKey) return
  localStorage.setItem(scopedKey, JSON.stringify(stateMap))
}

export function replaceCanvasBoardStateMap(stateMap: Record<string, CanvasBoardState>) {
  const normalized: Record<string, CanvasBoardState> = {}
  Object.entries(stateMap).forEach(([boardId, state]) => {
    const validState = toValidBoardState(state)
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
  appendOutputNodeImageUrls(urls, snapshot.viewpointNodes)
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
  const validState = toValidBoardState({
    ...state,
    previewSnapshotDataUrl: existing?.previewSnapshotDataUrl,
    previewSnapshotUpdatedAt: existing?.previewSnapshotUpdatedAt,
    previewSnapshotVersion: existing?.previewSnapshotVersion,
    updatedAt: Date.now(),
  })
  if (!validState) return null
  stateMap[id] = validState
  writeRawStateMap(stateMap)
  return validState
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
  const validState = toValidBoardState({
    ...existing,
    previewSnapshotDataUrl,
    previewSnapshotUpdatedAt,
    previewSnapshotVersion,
    updatedAt: existing.updatedAt,
  })
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
  const copiedState = toValidBoardState({
    ...sourceState,
    updatedAt: Date.now(),
  })
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
