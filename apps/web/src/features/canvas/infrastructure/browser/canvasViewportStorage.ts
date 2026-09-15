import { clamp } from '../../../../shared/utils/number'
import { readLocalStorageItem, writeLocalStorageItem } from '../../../../shared/utils/storage'

export type CanvasFlowViewportState = {
  x: number
  y: number
  zoom: number
}

export type StoredCanvasViewportState = CanvasFlowViewportState & {
  updatedAt: number
}

export type CanvasViewportStorageIdentity = {
  userUid: string | null | undefined
  boardId: string | null | undefined
}

export const CANVAS_VIEWPORT_MIN_ZOOM = 0.1
export const CANVAS_VIEWPORT_MAX_ZOOM = 2
const DEFAULT_CANVAS_VIEWPORT: CanvasFlowViewportState = Object.freeze({
  x: 0,
  y: 0,
  zoom: 1,
})

const STORAGE_KEY_PREFIX = 'ideaboard:canvas:view'

function normalizeKeyPart(value: string | null | undefined): string | null {
  const normalized = `${value ?? ''}`.trim()
  return normalized ? encodeURIComponent(normalized) : null
}

function createCanvasViewportStorageKey(identity: CanvasViewportStorageIdentity): string | null {
  const userUid = normalizeKeyPart(identity.userUid)
  const boardId = normalizeKeyPart(identity.boardId)
  if (!userUid || !boardId) return null
  return `${STORAGE_KEY_PREFIX}:${userUid}:${boardId}`
}

function clampZoom(zoom: number): number {
  return clamp(zoom, CANVAS_VIEWPORT_MIN_ZOOM, CANVAS_VIEWPORT_MAX_ZOOM)
}

function normalizeViewportRecord(value: unknown): StoredCanvasViewportState | null {
  if (!value || typeof value !== 'object') return null
  const record = value as Record<string, unknown>
  if (
    typeof record.x !== 'number' ||
    typeof record.y !== 'number' ||
    typeof record.zoom !== 'number' ||
    typeof record.updatedAt !== 'number'
  ) {
    return null
  }
  if (
    !Number.isFinite(record.x) ||
    !Number.isFinite(record.y) ||
    !Number.isFinite(record.zoom) ||
    !Number.isFinite(record.updatedAt)
  ) {
    return null
  }
  return {
    x: record.x,
    y: record.y,
    zoom: clampZoom(record.zoom),
    updatedAt: record.updatedAt,
  }
}

function readCanvasViewportState(
  identity: CanvasViewportStorageIdentity,
): StoredCanvasViewportState | null {
  const key = createCanvasViewportStorageKey(identity)
  if (!key) return null
  const onError = (error: unknown) => console.warn('[canvas] read viewport failed', error)
  const raw = readLocalStorageItem(key, onError)
  if (!raw) return null
  try {
    return normalizeViewportRecord(JSON.parse(raw))
  } catch (error) {
    onError(error)
    return null
  }
}

export function resolveCanvasViewportState(
  identity: CanvasViewportStorageIdentity,
): CanvasFlowViewportState {
  const stored = readCanvasViewportState(identity)
  return stored ? { x: stored.x, y: stored.y, zoom: stored.zoom } : { ...DEFAULT_CANVAS_VIEWPORT }
}

export function writeCanvasViewportState(
  identity: CanvasViewportStorageIdentity,
  viewport: CanvasFlowViewportState,
  now = Date.now(),
): StoredCanvasViewportState | null {
  const key = createCanvasViewportStorageKey(identity)
  if (!key) return null
  const next = normalizeViewportRecord({
    ...viewport,
    updatedAt: Number.isFinite(now) ? now : Date.now(),
  })
  if (!next) return null
  const ok = writeLocalStorageItem(key, JSON.stringify(next), (error) =>
    console.warn('[canvas] persist viewport failed', error),
  )
  return ok ? next : null
}
