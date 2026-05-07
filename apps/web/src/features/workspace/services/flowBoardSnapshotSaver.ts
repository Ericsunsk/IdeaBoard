import { syncCanvasBoardToServer } from '../../boards/lib/boardSync'
import { getCanvasBoardById, touchCanvasBoard } from '../../boards/lib/canvasBoards'
import { setCanvasBoardState } from '../../boards/lib/canvasBoardState'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'

export async function saveBoardSnapshotFromFlow(boardId: string, snapshot: CanvasBoardSnapshot): Promise<void> {
  const normalized = boardId.trim()
  if (!normalized) {
    throw new Error('MISSING_BOARD_ID')
  }
  const board = getCanvasBoardById(normalized)
  if (!board) {
    throw new Error('BOARD_NOT_FOUND')
  }
  const saved = setCanvasBoardState(normalized, snapshot)
  if (!saved) {
    throw new Error('SET_BOARD_STATE_FAILED')
  }
  const touched = touchCanvasBoard(normalized)
  if (!touched) {
    throw new Error('TOUCH_BOARD_FAILED')
  }
  await syncCanvasBoardToServer(normalized)
}
