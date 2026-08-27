import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../canvas-board/domain'
import type { CanvasGraphNodeIdForKind } from '../graph-runtime'
import { useFlowImageUploadActions } from './useFlowImageUploadActions'

const mocks = vi.hoisted(() => ({
  prepareImageAssetForUpload: vi.fn(),
}))

vi.mock('../../../shared/media/imageAssetPipeline', () => ({
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
  await Promise.resolve()
  await Promise.resolve()
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
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps the latest image upload result for the same node', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const updates: string[] = []
    const uploadA = createDeferred<{
      url: string
      width: number
      height: number
      media_asset_id?: number
    }>()
    const uploadB = createDeferred<{
      url: string
      width: number
      height: number
      media_asset_id?: number
    }>()
    const uploadApi = {
      upload: vi.fn().mockReturnValueOnce(uploadA.promise).mockReturnValueOnce(uploadB.promise),
    }
    const warnings: string[] = []
    const actions = useFlowImageUploadActions({
      currentBoardId,
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

    uploadB.resolve({ url: '/uploads/b.png', width: 100, height: 100, media_asset_id: 22 })
    await expect(secondUpload).resolves.toBe(true)
    uploadA.resolve({ url: '/uploads/a.png', width: 100, height: 100, media_asset_id: 11 })
    await expect(firstUpload).resolves.toBe(false)

    expect(snapshots.get('node-1')?.sourceImageUrl).toBe('/uploads/b.png')
    expect(snapshots.get('node-1')?.sourceMediaAssetId).toBe(22)
    expect(updates).toEqual(['/uploads/b.png'])
    expect(warnings).toEqual([])
  })

  it('ignores an image node upload that completes after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      media_asset_id?: number
    }>()
    const updateSnapshotNode = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
    upload.resolve({ url: '/uploads/a.png', width: 100, height: 100, media_asset_id: 31 })

    await expect(task).resolves.toBe(false)
    expect(updateSnapshotNode).not.toHaveBeenCalled()
  })

  it('releases an optimistic image node upload that completes after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const upload = createDeferred<{
      url: string
      width: number
      height: number
      media_asset_id?: number
    }>()
    const createNodeAtPosition = vi.fn().mockReturnValue('node-new')
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
    upload.resolve({ url: '/uploads/a.png', width: 100, height: 100, media_asset_id: 32 })

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
          url: '/uploads/canvas/user/board-a/content/first.png',
          media_asset_id: 41,
          width: 100,
          height: 100,
        })
        .mockResolvedValueOnce({
          url: '/uploads/canvas/user/board-a/content/second.png',
          media_asset_id: 42,
          width: 100,
          height: 100,
        }),
    }
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
      media_asset_id: number
    }>()
    const runtimeStates: Array<{ previewUrl: string; failureMessage?: string } | null> = []
    const revokeObjectUrl = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
      media_asset_id: 61,
    })

    await expect(task).resolves.toEqual({ kind: 'ready', nodeId: 'image-1' })
    expect(snapshots.get('image-1')).toMatchObject({
      sourceImageUrl: '/uploads/local.png',
      sourceMediaAssetId: 61,
      sourceImageRatio: 1,
    })
    expect(runtimeStates).toEqual([{ previewUrl: 'blob:local-image' }, null])
    expect(revokeObjectUrl).toHaveBeenCalledWith('blob:local-image')
  })

  it('keeps the local preview after a failed upload and retries with the same file', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>()
    const runtimeStates: Array<{ previewUrl: string; failureMessage?: string } | null> = []
    const revokeObjectUrl = vi.fn()
    const onWarning = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
            media_asset_id: 62,
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
    expect(snapshots.get('image-1')?.sourceImageUrl).toBe('/uploads/retried.png')
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
      media_asset_id: number
    }>()
    const revokeObjectUrl = vi.fn()
    const updateSnapshotNode = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
      media_asset_id: 63,
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
      media_asset_id?: number
    }>()
    const actions = useFlowImageUploadActions({
      currentBoardId,
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
    upload.resolve({ url: '/uploads/a.png', width: 100, height: 100, media_asset_id: 51 })

    await expect(task).rejects.toThrow('Upload result expired. Please pick the image again.')
  })
})
