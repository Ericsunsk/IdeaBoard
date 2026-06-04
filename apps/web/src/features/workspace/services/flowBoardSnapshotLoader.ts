import type { LocationQueryValue } from 'vue-router'
import { getCanvasBoardState } from '../../canvas/lib/canvasBoardState'
import { hydrateCanvasBoardsFromServer } from '../../boards/lib/boardSync'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'

export function resolveBoardIdFromQueryValue(
  raw: LocationQueryValue | LocationQueryValue[] | null | undefined,
): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw
  if (typeof value !== 'string') return null
  const boardId = value.trim()
  return boardId || null
}

export async function loadBoardSnapshotForFlow(
  boardId: string,
): Promise<CanvasBoardSnapshot | null> {
  const normalized = boardId.trim()
  if (!normalized) return null
  await hydrateCanvasBoardsFromServer()
  return getCanvasBoardState(normalized)
}
