import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../types/canvasNodes'
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

  it('keeps the latest image upload result for the same node', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const updates: string[] = []
    const uploadA = createDeferred<{
      code: number
      data: { url: string; width: number; height: number }
    }>()
    const uploadB = createDeferred<{
      code: number
      data: { url: string; width: number; height: number }
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
        const next = patcher(
          current as unknown as Record<string, unknown>,
        ) as unknown as ImageNodeItem
        snapshots.set(nodeId, next)
        updates.push(next.sourceImageUrl)
      },
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi,
      sanitizeApiErrorMessage: (msg, fallback) => msg || fallback,
      onWarning: (msg) => warnings.push(msg),
      onSuccess: vi.fn(),
    })

    const firstUpload = actions.applyImageFileToImageNode('node-1', makeFile('a.png'))
    const secondUpload = actions.applyImageFileToImageNode('node-1', makeFile('b.png'))
    await flushPromises()

    uploadB.resolve({ code: 0, data: { url: '/uploads/b.png', width: 100, height: 100 } })
    await expect(secondUpload).resolves.toBe(true)
    uploadA.resolve({ code: 0, data: { url: '/uploads/a.png', width: 100, height: 100 } })
    await expect(firstUpload).resolves.toBe(false)

    expect(snapshots.get('node-1')?.sourceImageUrl).toBe('/uploads/b.png')
    expect(updates).toEqual(['/uploads/b.png'])
    expect(warnings).toEqual([])
  })

  it('ignores an image node upload that completes after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const snapshots = new Map<string, ImageNodeItem>([['node-1', makeImageNode()]])
    const upload = createDeferred<{
      code: number
      data: { url: string; width: number; height: number }
    }>()
    const updateSnapshotNode = vi.fn()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode,
      getImageNodeSnapshot: (nodeId) => snapshots.get(nodeId) ?? null,
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      sanitizeApiErrorMessage: (msg, fallback) => msg || fallback,
      onWarning: vi.fn(),
      onSuccess: vi.fn(),
    })

    const task = actions.applyImageFileToImageNode('node-1', makeFile('a.png'))
    await flushPromises()
    currentBoardId.value = 'board-b'
    upload.resolve({ code: 0, data: { url: '/uploads/a.png', width: 100, height: 100 } })

    await expect(task).resolves.toBe(false)
    expect(updateSnapshotNode).not.toHaveBeenCalled()
  })

  it('does not create an image node when upload completes after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const upload = createDeferred<{
      code: number
      data: { url: string; width: number; height: number }
    }>()
    const createNodeAtPosition = vi.fn().mockReturnValue('node-new')
    const actions = useFlowImageUploadActions({
      currentBoardId,
      createNodeAtPosition,
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: vi.fn(),
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      sanitizeApiErrorMessage: (msg, fallback) => msg || fallback,
      onWarning: vi.fn(),
      onSuccess: vi.fn(),
    })

    const task = actions.createImageNodeFromFile({ x: 10, y: 20 }, makeFile('a.png'))
    await flushPromises()
    currentBoardId.value = 'board-b'
    upload.resolve({ code: 0, data: { url: '/uploads/a.png', width: 100, height: 100 } })

    await expect(task).resolves.toBeNull()
    expect(createNodeAtPosition).not.toHaveBeenCalled()
  })

  it('rejects shared upload results that complete after switching boards', async () => {
    const currentBoardId = { value: 'board-a' }
    const upload = createDeferred<{
      code: number
      data: { url: string; width: number; height: number }
    }>()
    const actions = useFlowImageUploadActions({
      currentBoardId,
      createNodeAtPosition: vi.fn(),
      updateSnapshotNode: vi.fn(),
      getImageNodeSnapshot: vi.fn(),
      uploadApi: { upload: vi.fn().mockReturnValue(upload.promise) },
      sanitizeApiErrorMessage: (msg, fallback) => msg || fallback,
      onWarning: vi.fn(),
      onSuccess: vi.fn(),
    })

    const task = actions.uploadImageToBoard(makeFile('a.png'))
    await flushPromises()
    currentBoardId.value = 'board-b'
    upload.resolve({ code: 0, data: { url: '/uploads/a.png', width: 100, height: 100 } })

    await expect(task).rejects.toThrow('上传结果已过期')
  })
})
