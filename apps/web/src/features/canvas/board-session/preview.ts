import {
  assertLatestCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import {
  BOARD_PREVIEW_RENDERER_VERSION,
  renderBoardPreviewDataUrl,
} from '../../canvas-board/preview'
import type { BoardSessionRepository } from './repository'

export type BoardSessionPreviewRenderer = {
  render: (snapshot: CanvasBoardSnapshot) => Promise<string>
  rendererVersion: number
}

export type RefreshBoardSessionPreviewOptions = {
  boardId: string
  snapshot: CanvasBoardSnapshot
  repository: Pick<BoardSessionRepository, 'setPreview' | 'sync'>
  renderer?: BoardSessionPreviewRenderer
}

export const defaultBoardSessionPreviewRenderer: BoardSessionPreviewRenderer = {
  render: renderBoardPreviewDataUrl,
  rendererVersion: BOARD_PREVIEW_RENDERER_VERSION,
}

export async function refreshBoardSessionPreview(
  options: RefreshBoardSessionPreviewOptions,
): Promise<void> {
  const renderer = options.renderer ?? defaultBoardSessionPreviewRenderer
  const snapshot = assertLatestCanvasBoardSnapshot(options.snapshot, 'session-preview-refresh')
  const snapshotDataUrl = await renderer.render(snapshot)
  options.repository.setPreview(
    options.boardId,
    snapshotDataUrl || undefined,
    snapshotDataUrl ? Date.now() : undefined,
    snapshotDataUrl ? renderer.rendererVersion : undefined,
  )
  await options.repository.sync(options.boardId)
}
