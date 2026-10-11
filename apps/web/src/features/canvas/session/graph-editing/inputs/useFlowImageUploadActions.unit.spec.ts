import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../../board'
import type { CanvasGraphNodeIdForKind } from '../../../core/graph'
import { unsupportedUploadImageMessage } from '../../../../../core/utils/imageUploadSupport'
import { useFlowImageUploadActions } from './useFlowImageUploadActions'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaAssetDelivery'

const mocks = vi.hoisted(() => ({
  prepareImageAssetForUpload: vi.fn(),
}))

vi.mock('../../../../../core/media/imageAssetPipeline', () => ({
  prepareImageAssetForUpload: mocks.prepareImageAssetForUpload,
}))

type Deferred<T> = {
  promise: Promise<T>
  resolve: (value: T) => void
  reject: (reason?: unknown) => void
}

function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

function makeFile(name: string): File {
  return new File(['image'], name, { type: 'image/png', lastModified: Date.now() })
}

function makeImageNode(url = ''): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: url,
    sourceImageRatio: 1,
    outputStatus: 'idle',
    outputImages: [],
  }
}

async function flushPromises() {
  for (let index = 0; index < 6; index += 1) await Promise.resolve()
}

describe('useFlowImageUploadActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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
    vi.unstubAllGlobals()
    resetMediaWorkspaceIdResolver()
  })

  it('keeps the latest image upload result for the same node', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const updates: string[] = []
    const uploadA = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const uploadB = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const uploadApi = {
      upload: vi.fn().mockReturnValueOnce(uploadA.promise).mockReturnValueOnce(uploadB.promise),
    }
    const warnings: string[] = []
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: (nodeId, patcher) => {
        const current = snapshots.get(nodeId)
        if (!current) return
        const next = patcher(current) as ImageNodeItem
        snapshots.set(nodeId, next)
        updates.push(next.sourceImageUrl)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi,
      onWarning: (msg) => warnings.push(msg),
      setLocalUploadState: vi.fn(),
    })

    const firstUpload = actions.applyImageFileToImageNode('node-1', makeFile('a.png'))
    const secondUpload = actions.applyImageFileToImageNode('node-1', makeFile('b.png'))
    await flushPromises()

    uploadB.resolve({ url: '/uploads/b.png', width: 100, height: 100, asset_id: 'asset_22' })
    await expect(secondUpload).resolves.toBe(true)
    uploadA.resolve({ url: '/uploads/a.png', width: 100, height: 100, asset_id: 'asset_11' })
    await expect(firstUpload).resolves.toBe(false)

    expect(snapshots.get('node-1')?.sourceImageUrl).toBe(
      '/api/v1/workspaces/ws_test123/media-assets/asset_22/derivatives/preview',
    )
    expect(snapshots.get('node-1')?.sourceMediaRef).toEqual({
      assetId: 'asset_22',
      derivativeTier: 'preview',
    })
    expect(updates).toEqual([
      '/api/v1/workspaces/ws_test123/media-assets/asset_22/derivatives/preview',
    ])
    expect(warnings).toEqual([])
  })

  it('keeps the second upload when the first drop resolves stale first', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const updates: string[] = []
    const uploadA = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const uploadB = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const uploadApi = {
      upload: vi.fn().mockReturnValueOnce(uploadA.promise).mockReturnValueOnce(uploadB.promise),
    }
    const warnings: string[] = []
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: (nodeId, patcher) => {
        const current = snapshots.get(nodeId)
        if (!current) return
        const next = patcher(current) as ImageNodeItem
        snapshots.set(nodeId, next)
        updates.push(next.sourceImageUrl)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi,
      onWarning: (msg) => warnings.push(msg),
      setLocalUploadState: vi.fn(),
    })

    const firstUpload = actions.applyImageFileToImageNode('node-1', makeFile('a.png'))
    const secondUpload = actions.applyImageFileToImageNode('node-1', makeFile('b.png'))
    await flushPromises()

    uploadA.resolve({ url: '/uploads/a.png', width: 100, height: 100, asset_id: 'asset_11' })
    await expect(firstUpload).resolves.toBe(false)
    uploadB.resolve({ url: '/uploads/b.png', width: 100, height: 100, asset_id: 'asset_22' })
    await expect(secondUpload).resolves.toBe(true)

    expect(snapshots.get('node-1')?.sourceImageUrl).toBe(
      '/api/v1/workspaces/ws_test123/media-assets/asset_22/derivatives/preview',
    )
    expect(snapshots.get('node-1')?.sourceMediaRef).toEqual({
      assetId: 'asset_22',
      derivativeTier: 'preview',
    })
    expect(updates).toEqual([
      '/api/v1/workspaces/ws_test123/media-assets/asset_22/derivatives/preview',
    ])
    expect(warnings).toEqual([])
  })

  it('publishes an optimistic blob preview when replacing an existing image node', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const runtimeStates: Array<{ previewUrl: string; failureMessage?: string } | null> = []
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: (nodeId, patcher) => {
        const current = snapshots.get(nodeId)
        if (current) snapshots.set(nodeId, patcher(current) as ImageNodeItem)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      createObjectUrl: vi.fn(() => 'blob:replacement-image'),
      revokeObjectUrl: vi.fn(),
      setLocalUploadState: (_nodeId, state) => runtimeStates.push(state),
    })

    const task = actions.applyImageFileToImageNode('node-1', makeFile('replacement.png'))
    await flushPromises()

    expect(runtimeStates).toEqual([{ previewUrl: 'blob:replacement-image' }])

    upload.resolve({
      url: '/uploads/replacement.png',
      width: 100,
      height: 100,
      asset_id: 'asset_71',
    })
    await expect(task).resolves.toBe(true)
    expect(runtimeStates).toEqual([{ previewUrl: 'blob:replacement-image' }, null])
    expect(snapshots.get('node-1')?.sourceMediaRef?.assetId).toBe('asset_71')
  })

  it('warns when an existing image node receives an unsupported file or has no board', async () => {
    const unsupportedWarnings: string[] = []
    const unsupportedActions = useFlowImageUploadActions({
      currentBoardId: { value: 'board-a' },
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: () => makeImageNode(),
      uploadApi: {
        upload: vi.fn().mockResolvedValue({
          url: '/uploads/decoded.png',
          asset_id: 'asset_64',
          width: 100,
          height: 100,
        }),
      },
      onWarning: (message) => unsupportedWarnings.push(message),
      setLocalUploadState: vi.fn(),
    })

    await expect(
      unsupportedActions.applyImageFileToImageNode(
        'node-1',
        new File(['not an image'], 'bad.txt', { type: 'text/plain' }),
      ),
    ).resolves.toBe(false)
    expect(unsupportedWarnings).toEqual([unsupportedUploadImageMessage()])

    const missingBoardWarnings: string[] = []
    const missingBoardActions = useFlowImageUploadActions({
      currentBoardId: { value: null },
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: () => makeImageNode(),
      uploadApi: { upload: vi.fn() },
      onWarning: (message) => missingBoardWarnings.push(message),
      setLocalUploadState: vi.fn(),
    })

    await expect(
      missingBoardActions.applyImageFileToImageNode('node-1', makeFile('ok.png')),
    ).resolves.toBe(false)
    expect(missingBoardWarnings).toEqual(['Missing board ID. Unable to upload the image.'])
  })

  it('ignores an image node upload that completes after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const updateSnapshotNode = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode,
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      setLocalUploadState: vi.fn(),
    })

    const task = actions.applyImageFileToImageNode('node-1', makeFile('a.png'))
    await flushPromises()
    currentBoardId.value = 'board-b'
    upload.resolve({ url: '/uploads/a.png', width: 100, height: 100, asset_id: 'asset_31' })

    await expect(task).resolves.toBe(false)
    expect(updateSnapshotNode).not.toHaveBeenCalled()
  })

  it('releases an optimistic image node upload that completes after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const createNodeAtPosition = vi.fn().mockReturnValue('node-new')
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition,
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: vi.fn(),
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      setLocalUploadState: vi.fn(),
    })

    const task = actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('a.png'))
    await flushPromises()
    currentBoardId.value = 'board-b'
    upload.resolve({ url: '/uploads/a.png', width: 100, height: 100, asset_id: 'asset_32' })

    await expect(task).resolves.toBeNull()
    expect(createNodeAtPosition).toHaveBeenCalledOnce()
  })

  it('does not reuse a completed upload when the same file is inserted again', async () => {
    const currentBoardId = { value: 'board-a' }
    const createdUrls: string[] = []
    const snapshots = new Map<string, ImageNodeItem>()
    const uploadApi = {
      upload: vi
        .fn()
        .mockResolvedValueOnce({
          url: '/uploads/tasks/user/board-a/content/first.png',
          asset_id: 'asset_41',
          width: 100,
          height: 100,
        })
        .mockResolvedValueOnce({
          url: '/uploads/tasks/user/board-a/content/second.png',
          asset_id: 'asset_42',
          width: 100,
          height: 100,
        }),
    }
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn((_kind, _x, _y, patch) => {
        createdUrls.push(patch?.sourceImageUrl ?? '')
        const nodeId = `image-${createdUrls.length}` as CanvasGraphNodeIdForKind<'image'>
        snapshots.set(nodeId, { ...makeImageNode(), ...patch })
        return nodeId
      }),
      updateSnapshotNode: (nodeId, patcher) => {
        const current = snapshots.get(nodeId)
        if (current) snapshots.set(nodeId, patcher(current) as ImageNodeItem)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi,
      onWarning: vi.fn(),
      setLocalUploadState: vi.fn(),
    })
    const file = makeFile('same.png')

    await expect(actions.createImageNodeFromFile({ x: 10, y: 20 }, file)).resolves.toEqual({
      kind: 'ready',
      nodeId: 'image-1',
    })
    await expect(actions.createImageNodeFromFile({ x: 30, y: 40 }, file)).resolves.toEqual({
      kind: 'ready',
      nodeId: 'image-2',
    })

    expect(uploadApi.upload).toHaveBeenCalledTimes(2)
    expect(createdUrls).toEqual(['', ''])
  })

  it('keeps a local object URL outside the snapshot until the managed upload succeeds', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>()
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const runtimeStates: Array<{ previewUrl: string; failureMessage?: string } | null> = []
    const revokeObjectUrl = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn((_kind, _x, _y, patch) => {
        snapshots.set('image-1', { ...makeImageNode(), ...patch })
        return 'image-1' as CanvasGraphNodeIdForKind<'image'>
      }),
      updateSnapshotNode: (nodeId, patcher) => {
        const current = snapshots.get(nodeId)
        if (current) snapshots.set(nodeId, patcher(current) as ImageNodeItem)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      createObjectUrl: vi.fn(() => 'blob:local-image'),
      revokeObjectUrl,
      setLocalUploadState: (_nodeId, state) => runtimeStates.push(state),
    })

    const task = actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('local.png'))
    await flushPromises()

    expect(snapshots.get('image-1')?.sourceImageUrl).toBe('')
    expect(runtimeStates).toEqual([{ previewUrl: 'blob:local-image' }])

    upload.resolve({
      url: '/uploads/local.png',
      width: 200,
      height: 100,
      asset_id: 'asset_61',
    })

    await expect(task).resolves.toEqual({ kind: 'ready', nodeId: 'image-1' })
    expect(snapshots.get('image-1')).toMatchObject({
      sourceImageUrl: '/api/v1/workspaces/ws_test123/media-assets/asset_61/derivatives/preview',
      sourceMediaRef: {
        assetId: 'asset_61',
        derivativeTier: 'preview',
      },
      sourceImageRatio: 1,
    })
    expect(runtimeStates).toEqual([{ previewUrl: 'blob:local-image' }, null])
    expect(revokeObjectUrl).toHaveBeenCalledWith('blob:local-image')
  })

  it('does not create a node until its local preview has decoded', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>()
    const decode = createDeferred<boolean>()
    const upload = createDeferred<{
      url: string
      asset_id: string
      width: number
      height: number
    }>()
    const createNodeAtPosition = vi.fn((_kind, _x, _y, patch) => {
      snapshots.set('image-1', { ...makeImageNode(), ...patch })
      return 'image-1' as CanvasGraphNodeIdForKind<'image'>
    })
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition,
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      createObjectUrl: vi.fn(() => 'blob:decoded-image'),
      revokeObjectUrl: vi.fn(),
      preloadImage: vi.fn(() => decode.promise),
      setLocalUploadState: vi.fn(),
    })

    const task = actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('decoded.png'))
    await flushPromises()
    expect(createNodeAtPosition).not.toHaveBeenCalled()

    decode.resolve(true)
    await flushPromises()
    expect(createNodeAtPosition).toHaveBeenCalledOnce()

    upload.resolve({
      url: '/uploads/decoded.png',
      asset_id: 'asset_64',
      width: 100,
      height: 100,
    })
    await expect(task).resolves.toMatchObject({ kind: 'ready', nodeId: 'image-1' })
  })

  it('keeps the local preview after a failed upload and retries with the same file', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>()
    const runtimeStates: Array<{ previewUrl: string; failureMessage?: string } | null> = []
    const revokeObjectUrl = vi.fn()
    const onWarning = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn((_kind, _x, _y, patch) => {
        snapshots.set('image-1', { ...makeImageNode(), ...patch })
        return 'image-1' as CanvasGraphNodeIdForKind<'image'>
      }),
      updateSnapshotNode: (nodeId, patcher) => {
        const current = snapshots.get(nodeId)
        if (current) snapshots.set(nodeId, patcher(current) as ImageNodeItem)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: {
        upload: vi
          .fn()
          .mockRejectedValueOnce(new Error('network unavailable'))
          .mockResolvedValueOnce({
            url: '/uploads/retried.png',
            width: 100,
            height: 100,
            asset_id: 'asset_62',
          }),
      },
      onWarning,
      createObjectUrl: vi.fn(() => 'blob:retry-image'),
      revokeObjectUrl,
      setLocalUploadState: (_nodeId, state) => runtimeStates.push(state),
    })

    await expect(
      actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('retry.png')),
    ).resolves.toEqual({
      kind: 'awaiting-retry',
      nodeId: 'image-1',
      failureMessage: 'Upload failed.',
    })
    expect(runtimeStates).toEqual([
      { previewUrl: 'blob:retry-image' },
      { previewUrl: 'blob:retry-image', failureMessage: 'Upload failed.' },
    ])
    expect(onWarning).not.toHaveBeenCalled()
    expect(revokeObjectUrl).not.toHaveBeenCalled()

    await expect(actions.retryLocalUpload('image-1')).resolves.toBe(true)
    expect(snapshots.get('image-1')?.sourceImageUrl).toBe(
      '/api/v1/workspaces/ws_test123/media-assets/asset_62/derivatives/preview',
    )
    expect(runtimeStates).toEqual([
      { previewUrl: 'blob:retry-image' },
      { previewUrl: 'blob:retry-image', failureMessage: 'Upload failed.' },
      { previewUrl: 'blob:retry-image' },
      null,
    ])
    expect(revokeObjectUrl).toHaveBeenCalledOnce()
  })

  it('releases a pending local preview when its node is detached', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>()
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const revokeObjectUrl = vi.fn()
    const updateSnapshotNode = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn((_kind, _x, _y, patch) => {
        snapshots.set('image-1', { ...makeImageNode(), ...patch })
        return 'image-1' as CanvasGraphNodeIdForKind<'image'>
      }),
      updateSnapshotNode,
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      createObjectUrl: vi.fn(() => 'blob:detached-image'),
      revokeObjectUrl,
      setLocalUploadState: vi.fn(),
    })

    const task = actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('detached.png'))
    await flushPromises()
    actions.releaseDetachedLocalUploads(new Set())
    upload.resolve({
      url: '/uploads/detached.png',
      width: 100,
      height: 100,
      asset_id: 'asset_63',
    })

    await expect(task).resolves.toBeNull()
    expect(updateSnapshotNode).not.toHaveBeenCalled()
    expect(revokeObjectUrl).toHaveBeenCalledWith('blob:detached-image')
  })

  it('does not create a node after its scope is disposed during image inspection', async () => {
    const bitmap = createDeferred<ImageBitmap>()
    vi.stubGlobal(
      'createImageBitmap',
      vi.fn(() => bitmap.promise),
    )
    const createNodeAtPosition = vi.fn()
    const createObjectUrl = vi.fn(() => 'blob:stale-image')
    const actions = useFlowImageUploadActions({
      currentBoardId: { value: 'board-a' },
      workspaceId: () => 'ws_test123',
      createNodeAtPosition,
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: vi.fn(),
      uploadApi: { upload: vi.fn() },
      onWarning: vi.fn(),
      setLocalUploadState: vi.fn(),
      createObjectUrl,
    })

    const task = actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('stale.png'))
    actions.disposeLocalUploads()
    bitmap.resolve({ width: 100, height: 100, close: vi.fn() } as unknown as ImageBitmap)

    await expect(task).resolves.toBeNull()
    expect(createNodeAtPosition).not.toHaveBeenCalled()
    expect(createObjectUrl).not.toHaveBeenCalled()
  })

  it('rejects shared upload results that complete after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      asset_id: string
    }>()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      workspaceId: () => 'ws_test123',
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: vi.fn(),
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      onWarning: vi.fn(),
      setLocalUploadState: vi.fn(),
    })

    const task = actions.uploadImageToBoard(makeFile('a.png'))
    await flushPromises()
    currentBoardId.value = 'board-b'
    upload.resolve({ url: '/uploads/a.png', width: 100, height: 100, asset_id: 'asset_51' })

    await expect(task).rejects.toThrow('Upload result expired. Please pick the image again.')
  })
})
