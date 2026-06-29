import { describe, expect, it, vi } from 'vitest'
import { makeCanvasBoardSnapshot } from '../../canvas-board/testing'
import { createBoardSessionUploadLifecycle } from './boardSessionUploadLifecycle'

describe('createBoardSessionUploadLifecycle', () => {
  it('stabilizes the save-owned snapshot without cloning it', async () => {
    const snapshot = makeCanvasBoardSnapshot({
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: '  /uploads/canvas/user-uid/board-a/content/source.png  ',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [
            {
              url: '  /uploads/canvas/user-uid/board-a/content/output.png  ',
              ratio: 1,
            },
          ],
        },
      ],
    })
    const lifecycle = createBoardSessionUploadLifecycle({
      userId: 1,
      userUid: 'user-uid',
      boardId: 'board-a',
      warning: vi.fn(),
    })

    const stabilized = await lifecycle.stabilizeSnapshotForSave(snapshot)

    expect(stabilized).toBe(snapshot)
    expect(snapshot.imageNodes[0]?.sourceImageUrl).toBe(
      '/uploads/canvas/user-uid/board-a/content/source.png',
    )
    expect(snapshot.imageNodes[0]?.outputImages[0]?.url).toBe(
      '/uploads/canvas/user-uid/board-a/content/output.png',
    )
  })
})
