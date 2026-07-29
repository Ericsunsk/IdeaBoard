import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadApi } from '../../../shared/api/upload'
import { makeCanvasBoardSnapshot } from '../../canvas-board/testing'
import { createBoardSessionUploadLifecycle } from './boardSessionUploadLifecycle'

const mocks = vi.hoisted(() => ({
  prepareImageAssetForUpload: vi.fn(),
}))

vi.mock('../../../shared/media/imageAssetPipeline', () => ({
  prepareImageAssetForUpload: mocks.prepareImageAssetForUpload,
}))

describe('createBoardSessionUploadLifecycle', () => {
  beforeEach(() => {
    mocks.prepareImageAssetForUpload.mockImplementation(async (file: File) => ({
      originalFile: file,
      uploadFile: file,
      width: 100,
      height: 100,
      ratio: 1,
      dimensionsKnown: true,
      warnings: [],
      compressed: false,
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

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

  it('records media asset ids when stabilizing image and fusion outputs', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(new Blob(['image'], { type: 'image/png' }), { status: 200 })),
    )
    vi.spyOn(uploadApi, 'upload')
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/image-output.png',
        media_asset_id: 201,
      })
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/fusion-output.png',
        media_asset_id: 202,
      })
    const snapshot = makeCanvasBoardSnapshot({
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: '/uploads/canvas/user-uid/board-a/content/source.png',
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [{ url: 'https://provider.example/image-output.png', ratio: 1 }],
        },
      ],
      fusionNodes: [
        {
          id: 2,
          type: 'fusion',
          x: 320,
          y: 0,
          width: 280,
          prompt: '',
          outputCount: 1,
          aspectRatio: 'auto',
          blendPoint: { x: 0.5, y: 0.5 },
          inputImages: [],
          outputStatus: 'ready',
          outputImages: [{ url: 'https://provider.example/fusion-output.png', ratio: 1 }],
        },
      ],
    })

    await createBoardSessionUploadLifecycle({
      userUid: 'user-uid',
      boardId: 'board-a',
      warning: vi.fn(),
    }).stabilizeSnapshotForSave(snapshot)

    expect(snapshot.imageNodes[0]?.outputImages[0]).toMatchObject({
      url: '/uploads/canvas/user-uid/board-a/content/image-output.png',
      mediaAssetId: 201,
    })
    expect(snapshot.fusionNodes[0]?.outputImages[0]).toMatchObject({
      url: '/uploads/canvas/user-uid/board-a/content/fusion-output.png',
      mediaAssetId: 202,
    })
    expect(snapshot.mediaRefs).toEqual([
      {
        mediaAssetId: 201,
        refType: 'board_node',
        refId: 'image-1',
        role: 'output:0',
      },
      {
        mediaAssetId: 202,
        refType: 'board_node',
        refId: 'fusion-2',
        role: 'output:0',
      },
    ])
  })

  it('stabilizes prompt and fusion inputs and rebuilds their media refs', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(new Blob(['image'], { type: 'image/png' }), { status: 200 })),
    )
    vi.spyOn(uploadApi, 'upload')
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/prompt-input.png',
        media_asset_id: 301,
      })
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/fusion-input.png',
        media_asset_id: 302,
      })
    const snapshot = makeCanvasBoardSnapshot({
      promptNodes: [
        {
          id: 1,
          type: 'prompt',
          x: 0,
          y: 0,
          width: 280,
          text: '',
          outputCount: 1,
          aspectRatio: 'auto',
          inputImages: [
            {
              id: 'external:prompt',
              sourceType: 'external',
              sourceId: 'prompt-input',
              url: 'https://provider.example/prompt-input.png',
              ratio: 1,
            },
          ],
        },
      ],
      fusionNodes: [
        {
          id: 2,
          type: 'fusion',
          x: 320,
          y: 0,
          width: 280,
          prompt: '',
          outputCount: 1,
          aspectRatio: 'auto',
          blendPoint: { x: 0.5, y: 0.5 },
          inputImages: [
            {
              id: 'external:fusion',
              sourceType: 'external',
              sourceId: 'fusion-input',
              url: 'https://provider.example/fusion-input.png',
              ratio: 1,
            },
          ],
          outputStatus: 'idle',
          outputImages: [],
        },
      ],
    })

    await createBoardSessionUploadLifecycle({
      userUid: 'user-uid',
      boardId: 'board-a',
      warning: vi.fn(),
    }).stabilizeSnapshotForSave(snapshot)

    expect(snapshot.promptNodes[0]?.inputImages[0]).toMatchObject({
      url: '/uploads/canvas/user-uid/board-a/content/prompt-input.png',
      mediaAssetId: 301,
    })
    expect(snapshot.fusionNodes[0]?.inputImages[0]).toMatchObject({
      url: '/uploads/canvas/user-uid/board-a/content/fusion-input.png',
      mediaAssetId: 302,
    })
    expect(snapshot.mediaRefs).toEqual([
      { mediaAssetId: 301, refType: 'board_node', refId: 'prompt-1', role: 'input:0' },
      { mediaAssetId: 302, refType: 'board_node', refId: 'fusion-2', role: 'input:0' },
    ])
  })
})
