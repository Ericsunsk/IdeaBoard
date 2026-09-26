import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { uploadApi } from '../../../../shared/api/upload'
import { makeBoardSnapshot } from '../testing/boardFixtures'
import { createBoardSessionUploadLifecycle } from './uploadLifecycle'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../shared/api/mediaAssetDelivery'

const mocks = vi.hoisted(() => ({
  prepareImageAssetForUpload: vi.fn(),
}))

vi.mock('../../../../shared/media/imageAssetPipeline', () => ({
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
    setMediaWorkspaceIdResolver(() => 'ws_test123')
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    resetMediaWorkspaceIdResolver()
  })

  it('stabilizes the save-owned snapshot without cloning it', async () => {
    const snapshot = makeBoardSnapshot({
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
      workspaceId: 'ws_test123',
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
        asset_id: 'asset_201',
      })
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/fusion-output.png',
        asset_id: 'asset_202',
      })
    const snapshot = makeBoardSnapshot({
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
      workspaceId: 'ws_test123',
      warning: vi.fn(),
    }).stabilizeSnapshotForSave(snapshot)

    expect(snapshot.imageNodes[0]?.outputImages[0]).toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_201/derivatives/preview',
      mediaRef: { assetId: 'asset_201', derivativeTier: 'preview' },
    })
    expect(snapshot.fusionNodes[0]?.outputImages[0]).toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_202/derivatives/preview',
      mediaRef: { assetId: 'asset_202', derivativeTier: 'preview' },
    })
    expect(snapshot.mediaRefs).toBeUndefined()
  })

  it('stabilizes prompt and fusion inputs and rebuilds their media refs', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(new Blob(['image'], { type: 'image/png' }), { status: 200 })),
    )
    vi.spyOn(uploadApi, 'upload')
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/prompt-input.png',
        asset_id: 'asset_301',
      })
      .mockResolvedValueOnce({
        url: '/uploads/canvas/user-uid/board-a/content/fusion-input.png',
        asset_id: 'asset_302',
      })
    const snapshot = makeBoardSnapshot({
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
      workspaceId: 'ws_test123',
      warning: vi.fn(),
    }).stabilizeSnapshotForSave(snapshot)

    expect(snapshot.promptNodes[0]?.inputImages[0]).toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_301/derivatives/preview',
      mediaRef: { assetId: 'asset_301', derivativeTier: 'preview' },
    })
    expect(snapshot.fusionNodes[0]?.inputImages[0]).toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_302/derivatives/preview',
      mediaRef: { assetId: 'asset_302', derivativeTier: 'preview' },
    })
    expect(snapshot.mediaRefs).toBeUndefined()
  })

  it('rewrites an existing provider URL to the stable media asset content URL', async () => {
    const snapshot = makeBoardSnapshot({
      imageNodes: [
        {
          id: 1,
          type: 'image-node',
          x: 0,
          y: 0,
          width: 280,
          sourceImageUrl: 'https://cdn.example.com/signed/source.png?expires=1',
          sourceMediaRef: {
            assetId: 'asset_401',
            derivativeTier: 'preview',
          },
          sourceImageRatio: 1,
          outputStatus: 'ready',
          outputImages: [
            {
              url: 'https://cdn.example.com/signed/output.png?expires=1',
              mediaRef: {
                assetId: 'asset_402',
                derivativeTier: 'preview',
              },
              ratio: 1,
            },
          ],
        },
      ],
    })

    await createBoardSessionUploadLifecycle({
      userUid: 'user-uid',
      boardId: 'board-a',
      warning: vi.fn(),
    }).stabilizeSnapshotForSave(snapshot)

    expect(snapshot.imageNodes[0]).toMatchObject({
      sourceImageUrl: '/api/v1/workspaces/ws_test123/media-assets/asset_401/derivatives/preview',
      sourceMediaRef: {
        assetId: 'asset_401',
        derivativeTier: 'preview',
      },
    })
    expect(snapshot.imageNodes[0]?.outputImages[0]).toMatchObject({
      url: '/api/v1/workspaces/ws_test123/media-assets/asset_402/derivatives/preview',
      mediaRef: {
        assetId: 'asset_402',
        derivativeTier: 'preview',
      },
    })
  })
})
