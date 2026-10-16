import type { Edge, Node } from '@vue-flow/core'
import { effectScope, ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../../board'
import { serializeBoardState } from '../../../../board/ports/canvasSession'
import { makeBoardState } from '../../../../board/ports/testing'
import { materializeLibraryAssetForCanvas } from '../../../../library'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import * as imageDimensions from '../../../../../core/media/imageDimensions'
import type { BoardAttachment } from '../../execution/board-attachment'
import { useCanvasVueFlowGraphAdapter } from '../../../session/flow-adapter'
import type { FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { useCanvasLibraryImportRuntime } from './useCanvasLibraryImportRuntime'

function flushPromises() {
  return new Promise((resolve) => setTimeout(resolve, 0))
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve
    reject = onReject
  })
  return { promise, resolve, reject }
}

function attachment(): BoardAttachment {
  return {} as BoardAttachment
}

function pendingNode(id = 'image-1', libraryAssetId = 'lasset_asset0007'): Node<FlowNodeData> {
  const snapshot: ImageNodeItem = {
    id: 1,
    type: 'image-node',
    x: 100,
    y: 200,
    width: 320,
    sourceImageUrl: '',
    sourceImageRatio: 0.5,
    outputStatus: 'idle',
    outputImages: [],
    libraryImport: { state: 'pending', libraryAssetId },
  }
  return {
    id,
    type: 'canvas',
    position: { x: snapshot.x, y: snapshot.y },
    data: { kind: 'image', nodeType: 'image', title: 'Library image', snapshotNode: snapshot },
  }
}

function createHarness(
  initialNodes: Node[] = [],
  preloadImage: (url: string) => Promise<boolean> = async () => true,
) {
  const nodes = shallowRef<Node[]>(initialNodes)
  const edges = shallowRef<Edge[]>([])
  const currentAttachment = ref<BoardAttachment | null>(attachment())
  const graphAdapter = useCanvasVueFlowGraphAdapter({ nodes, edges })
  const updateImageSnapshot = vi.fn(
    (nodeId: string, mutate: (snapshot: ImageNodeItem) => ImageNodeItem) => {
      nodes.value = nodes.value.map((node) => {
        if (node.id !== nodeId) return node
        const data = node.data as FlowNodeData
        const snapshot = data.snapshotNode as ImageNodeItem
        const { libraryImportPreviewUrl: _preview, ...nextData } = data
        return {
          ...node,
          data: { ...nextData, snapshotNode: mutate(snapshot) },
        }
      })
    },
  )
  const materialize = vi.fn()
  const addImageNodeAtPosition = vi.fn((x: number, y: number, patch: object) => {
    const nodeId = `image-${nodes.value.length + 1}`
    nodes.value = [
      ...nodes.value,
      {
        id: nodeId,
        type: 'canvas',
        position: { x, y },
        data: {
          kind: 'image',
          nodeType: 'image',
          title: 'Library image',
          snapshotNode: { id: nodes.value.length + 1, type: 'image-node', x, y, ...patch },
        } satisfies FlowNodeData,
      },
    ]
    return nodeId as `image-${number}`
  })
  const refreshNodeInternals = vi.fn()
  const onWarning = vi.fn()
  const imports = useCanvasLibraryImportRuntime({
    nodes,
    readAttachment: () => currentAttachment.value,
    graphAdapter,
    updateImageSnapshot,
    createImageNodeAtPosition: addImageNodeAtPosition,
    refreshNodeInternals,
    selectOnlyNode: vi.fn(() => true),
    materialize,
    onWarning,
    preloadImage,
  })
  return {
    nodes,
    currentAttachment,
    materialize,
    imports,
    refreshNodeInternals,
    onWarning,
    updateImageSnapshot,
  }
}

describe('useCanvasLibraryImportRuntime', () => {
  it('uses the decoded image aspect ratio when existing Media metadata has no dimensions', async () => {
    const readSize = vi
      .spyOn(imageDimensions, 'readImageNaturalSize')
      .mockResolvedValueOnce({ width: 240, height: 360 })
    try {
      const harness = createHarness()
      harness.materialize.mockResolvedValue({
        mediaAssetId: 'asset_media0042',
        url: '/media/42',
        width: 0,
        height: 0,
      })
      await harness.imports.start(
        {
          assetId: 'lasset_asset0007',
          title: 'Portrait',
          previewUrl: '/preview.png',
          width: null,
          height: null,
        },
        { x: 100, y: 200 },
      )
      await flushPromises()

      const snapshot = (harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem
      const saved = serializeBoardState(
        makeBoardState({ imageNodes: [snapshot] }),
        'remote-envelope',
      )
      expect(readSize).toHaveBeenCalledExactlyOnceWith('/media/42')
      expect(saved.imageNodes[0]).toMatchObject({ width: 260, sourceImageRatio: 1.5 })
      expect(saved.imageNodes[0]?.libraryImport).toBeUndefined()
    } finally {
      readSize.mockRestore()
    }
  })

  it('discards dimensions decoded after the Board attachment changes', async () => {
    const dimensions = deferred<{ width: number; height: number } | null>()
    const readSize = vi
      .spyOn(imageDimensions, 'readImageNaturalSize')
      .mockReturnValueOnce(dimensions.promise)
    try {
      const harness = createHarness([pendingNode()])
      harness.materialize.mockResolvedValue({
        mediaAssetId: 'asset_media0042',
        url: '/media/42',
        width: 0,
        height: 0,
      })
      harness.imports.resumePending()
      await flushPromises()
      harness.currentAttachment.value = attachment()
      dimensions.resolve({ width: 240, height: 360 })
      await flushPromises()

      expect(harness.updateImageSnapshot).not.toHaveBeenCalled()
      expect(harness.onWarning).not.toHaveBeenCalled()
    } finally {
      readSize.mockRestore()
    }
  })

  it('preserves the managed Media identity from a saved Search reference through Board save', async () => {
    const workspaceId = 'ws_thumbnail'
    const libraryAssetId = 'lasset_thumbnail'
    const mediaAssetId = 'asset_thumbnail'
    const contentUrl = `/api/v1/workspaces/${workspaceId}/media-assets/${mediaAssetId}/content`
    useMswHandlers(
      http.get(`/api/v1/workspaces/${workspaceId}/media-assets/${libraryAssetId}`, () =>
        HttpResponse.json({
          data: {
            id: mediaAssetId,
            status: 'ready',
            mime_type: 'image/png',
            width: 240,
            height: 360,
            size_bytes: 1000,
            content_url: contentUrl,
            derivatives: {},
          },
        }),
      ),
    )
    const harness = createHarness()
    const materialized = deferred<void>()
    harness.materialize.mockImplementation(async (assetId: string) => {
      const result = await materializeLibraryAssetForCanvas(workspaceId, assetId)
      materialized.resolve()
      return result
    })
    await harness.imports.start(
      {
        assetId: libraryAssetId,
        title: 'Search reference',
        previewUrl: '/preview.png',
        width: null,
        height: null,
      },
      { x: 100, y: 200 },
    )
    await materialized.promise
    await flushPromises()
    const snapshot = (harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem
    const saved = serializeBoardState(makeBoardState({ imageNodes: [snapshot] }), 'remote-envelope')
    expect(saved.imageNodes[0]?.sourceMediaRef).toEqual({
      assetId: mediaAssetId,
      derivativeTier: 'preview',
    })
    expect(saved.imageNodes[0]).toMatchObject({ width: 260, sourceImageRatio: 1.5 })
    expect(saved.imageNodes[0]?.libraryImport).toBeUndefined()
    expect(saved.imageNodes[0]).not.toHaveProperty('libraryImportPreviewUrl')
    expect(harness.updateImageSnapshot).toHaveBeenCalledTimes(1)
  })

  it('waits for the Library preview to decode before creating a node', async () => {
    const request = deferred<boolean>()
    const harness = createHarness([], () => request.promise)
    harness.materialize.mockReturnValue(new Promise(() => {}))
    const start = harness.imports.start(
      {
        assetId: 'lasset_asset0007',
        title: 'Reference',
        previewUrl: 'https://provider.example/preview.png',
        width: 200,
        height: 100,
      },
      { x: 100, y: 200 },
    )

    await flushPromises()
    expect(harness.nodes.value).toHaveLength(0)

    request.resolve(true)
    await start
    expect(harness.nodes.value).toHaveLength(1)
  })

  it('discards a decoded preview after the Board attachment changes', async () => {
    const request = deferred<boolean>()
    const harness = createHarness([], () => request.promise)
    const pending = harness.imports.start(
      {
        assetId: 'lasset_asset0007',
        title: 'Reference',
        previewUrl: 'https://provider.example/preview.png',
        width: 200,
        height: 100,
      },
      { x: 100, y: 200 },
    )
    harness.currentAttachment.value = attachment()
    request.resolve(true)
    expect(await pending).toBeNull()
    expect(harness.nodes.value).toHaveLength(0)
    expect(harness.materialize).not.toHaveBeenCalled()
    expect(harness.onWarning).not.toHaveBeenCalled()
  })

  it('persists only the owned asset fact, then atomically promotes to managed Media', async () => {
    const harness = createHarness()
    const request = deferred<{ mediaAssetId: string; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValue(request.promise)

    const nodeId = await harness.imports.start(
      {
        assetId: 'lasset_asset0007',
        title: 'Reference',
        previewUrl: 'https://provider.example/preview.png',
        width: 200,
        height: 100,
      },
      { x: 100, y: 200 },
    )

    const pending = (harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem
    expect(nodeId).toBe('image-1')
    expect(pending).toMatchObject({
      sourceImageUrl: '',
      libraryImport: { state: 'pending', libraryAssetId: 'lasset_asset0007' },
    })
    expect(pending.sourceMediaAssetId).toBeUndefined()
    expect(pending).not.toHaveProperty('previewUrl')
    expect((harness.nodes.value[0]!.data as FlowNodeData).libraryImportPreviewUrl).toBe(
      'https://provider.example/preview.png',
    )

    request.resolve({
      mediaAssetId: 'asset_media0042',
      url: 'https://app.example/media/42',
      width: 400,
      height: 200,
    })
    await flushPromises()

    const snapshot = (harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem
    expect(snapshot).toMatchObject({
      sourceImageUrl: 'https://app.example/media/42',
      outputStatus: 'ready',
    })
    expect(snapshot.sourceMediaAssetId).toBeUndefined()
    expect(snapshot.libraryImport).toBeUndefined()
    expect((harness.nodes.value[0]!.data as FlowNodeData).libraryImportPreviewUrl).toBeUndefined()
  })

  it('resumes each pending node at most once for one attachment', () => {
    const harness = createHarness([pendingNode()])
    harness.materialize.mockReturnValue(new Promise(() => {}))

    expect(harness.imports.resumePending()).toBe(1)
    expect(harness.imports.resumePending()).toBe(0)
    expect(harness.materialize).toHaveBeenCalledTimes(1)
  })

  it('recovers pending work once for a new attachment and rejects the old completion', async () => {
    const harness = createHarness([pendingNode()])
    const first = deferred<{ mediaAssetId: string; url: string; width: number; height: number }>()
    const second = deferred<{ mediaAssetId: string; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

    expect(harness.imports.resumePending()).toBe(1)
    harness.currentAttachment.value = attachment()
    expect(harness.imports.resumePending()).toBe(1)
    expect(harness.imports.resumePending()).toBe(0)

    first.resolve({
      mediaAssetId: 'asset_media0042',
      url: 'https://app.example/old',
      width: 100,
      height: 100,
    })
    await flushPromises()
    expect(
      ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem).libraryImport,
    ).toEqual({
      state: 'pending',
      libraryAssetId: 'lasset_asset0007',
    })

    second.resolve({
      mediaAssetId: 'asset_media0043',
      url: 'https://app.example/new',
      width: 100,
      height: 100,
    })
    await flushPromises()
    expect(
      ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem)
        .sourceMediaAssetId,
    ).toBeUndefined()
  })

  it('leaves a failed fact inert until an explicit retry', async () => {
    const harness = createHarness([pendingNode()])
    harness.materialize.mockRejectedValueOnce(new Error('源图不可访问'))

    harness.imports.resumePending()
    await flushPromises()
    const failed = ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem)
      .libraryImport
    expect(failed).toEqual({
      state: 'failed',
      libraryAssetId: 'lasset_asset0007',
      failureMessage: 'Library import failed. Please try again later.',
    })
    expect(harness.imports.resumePending()).toBe(0)
    expect(harness.materialize).toHaveBeenCalledTimes(1)

    harness.materialize.mockResolvedValueOnce({
      mediaAssetId: 'asset_media0042',
      url: 'https://app.example/media/42',
      width: 200,
      height: 100,
    })
    expect(harness.imports.retry('image-1')).toBe(true)
    await flushPromises()
    expect(harness.materialize).toHaveBeenCalledTimes(2)
    expect(harness.materialize).toHaveBeenNthCalledWith(2, 'lasset_asset0007')
    expect(
      ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem).libraryImport,
    ).toBeUndefined()
  })

  it('keeps a not-yet-materialized node pending and shares the active wait with recovery', async () => {
    vi.useFakeTimers()
    try {
      const harness = createHarness([pendingNode()])
      harness.materialize
        .mockRejectedValueOnce(
          Object.assign(new Error('media_not_found'), { code: 'media_not_found' }),
        )
        .mockResolvedValue({
          mediaAssetId: 'asset_media0042',
          url: 'https://app.example/media/42',
          width: 200,
          height: 100,
        })
      expect(harness.imports.resumePending()).toBe(1)
      await vi.advanceTimersByTimeAsync(0)
      expect(harness.imports.resumePending()).toBe(0)
      const snapshot = () =>
        (harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem
      expect(snapshot().libraryImport?.state).toBe('pending')
      expect(harness.onWarning).not.toHaveBeenCalled()
      expect(harness.materialize).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(500)
      expect(snapshot().libraryImport).toBeUndefined()
      expect(snapshot().sourceImageUrl).toBe('https://app.example/media/42')
      expect(harness.materialize).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('reports failure only after the readiness deadline and allows an explicit retry', async () => {
    vi.useFakeTimers()
    try {
      const harness = createHarness()
      harness.materialize.mockRejectedValue(
        Object.assign(new Error('media_not_found'), { code: 'media_not_found' }),
      )
      await harness.imports.start(
        {
          assetId: 'lasset_asset0007',
          title: '',
          previewUrl: 'https://example.com/preview.png',
          width: 200,
          height: 100,
        },
        { x: 0, y: 0 },
      )
      await vi.advanceTimersByTimeAsync(89_999)
      expect(harness.onWarning).not.toHaveBeenCalled()
      expect(
        ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem).libraryImport
          ?.state,
      ).toBe('pending')
      await vi.advanceTimersByTimeAsync(1)
      expect(harness.onWarning).toHaveBeenCalledExactlyOnceWith(
        'Library import failed. Please try again later.',
      )
      expect(
        ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem).libraryImport
          ?.state,
      ).toBe('failed')
      expect(harness.materialize.mock.calls.length).toBeLessThan(25)
      harness.materialize.mockResolvedValue({
        mediaAssetId: 'asset_media0042',
        url: 'https://app.example/media/42',
        width: 200,
        height: 100,
      })
      expect(harness.imports.retry('image-1')).toBe(true)
      await vi.advanceTimersByTimeAsync(0)
      expect(
        ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem)
          .libraryImport,
      ).toBeUndefined()
    } finally {
      vi.useRealTimers()
    }
  })

  it.each(['board', 'deleted', 'disposed'])(
    'stops readiness reads when the node is %s',
    async (reason) => {
      vi.useFakeTimers()
      const scope = effectScope()
      try {
        const harness = scope.run(() => createHarness([pendingNode()]))!
        harness.materialize.mockRejectedValue(
          Object.assign(new Error('media_not_found'), { code: 'media_not_found' }),
        )
        harness.imports.resumePending()
        await vi.advanceTimersByTimeAsync(0)
        if (reason === 'board') harness.currentAttachment.value = attachment()
        else if (reason === 'deleted') harness.nodes.value = []
        else scope.stop()
        await vi.advanceTimersByTimeAsync(10_000)
        expect(harness.materialize).toHaveBeenCalledTimes(1)
        expect(harness.onWarning).not.toHaveBeenCalled()
      } finally {
        scope.stop()
        vi.useRealTimers()
      }
    },
  )

  it('does not apply a late completion after the node is deleted', async () => {
    const harness = createHarness([pendingNode()])
    const request = deferred<{ mediaAssetId: string; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValue(request.promise)
    harness.imports.resumePending()
    harness.nodes.value = []
    request.resolve({
      mediaAssetId: 'asset_media0042',
      url: 'https://app.example/media/42',
      width: 200,
      height: 100,
    })

    await flushPromises()
    expect(harness.nodes.value).toEqual([])
  })

  it('keeps a retried failed node deleted when materialization completes late', async () => {
    const failedNode = pendingNode()
    const failedSnapshot = (failedNode.data as FlowNodeData).snapshotNode as ImageNodeItem
    failedSnapshot.libraryImport = {
      state: 'failed',
      libraryAssetId: 'lasset_asset0007',
      failureMessage: '源图不可访问',
    }
    const harness = createHarness([failedNode])
    const request = deferred<{ mediaAssetId: string; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValue(request.promise)

    expect(harness.imports.retry('image-1')).toBe(true)
    harness.nodes.value = []
    request.resolve({
      mediaAssetId: 'asset_media0042',
      url: 'https://app.example/media/42',
      width: 200,
      height: 100,
    })

    await flushPromises()
    expect(harness.materialize).toHaveBeenCalledWith('lasset_asset0007')
    expect(harness.nodes.value).toEqual([])
  })
})
