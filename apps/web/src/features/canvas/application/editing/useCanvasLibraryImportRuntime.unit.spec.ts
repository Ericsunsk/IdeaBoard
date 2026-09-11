import type { Edge, Node } from '@vue-flow/core'
import { ref, shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../board'
import type { BoardAttachment } from '../execution/board-attachment'
import { useCanvasVueFlowGraphAdapter } from '../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'
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

function pendingNode(id = 'image-1', libraryAssetId = 7): Node<FlowNodeData> {
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
  it('waits for the Library preview to decode before creating a node', async () => {
    const request = deferred<boolean>()
    const harness = createHarness([], () => request.promise)
    harness.materialize.mockReturnValue(new Promise(() => {}))
    const start = harness.imports.start(
      {
        assetId: 7,
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

  it('persists only the owned asset fact, then atomically promotes to managed Media', async () => {
    const harness = createHarness()
    const request = deferred<{ mediaAssetId: number; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValue(request.promise)

    const nodeId = await harness.imports.start(
      {
        assetId: 7,
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
      libraryImport: { state: 'pending', libraryAssetId: 7 },
    })
    expect(pending.sourceMediaAssetId).toBeUndefined()
    expect(pending).not.toHaveProperty('previewUrl')
    expect((harness.nodes.value[0]!.data as FlowNodeData).libraryImportPreviewUrl).toBe(
      'https://provider.example/preview.png',
    )

    request.resolve({
      mediaAssetId: 42,
      url: 'https://app.example/media/42',
      width: 400,
      height: 200,
    })
    await flushPromises()

    const snapshot = (harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem
    expect(snapshot).toMatchObject({
      sourceImageUrl: 'https://app.example/media/42',
      sourceMediaAssetId: 42,
      outputStatus: 'ready',
    })
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
    const first = deferred<{ mediaAssetId: number; url: string; width: number; height: number }>()
    const second = deferred<{ mediaAssetId: number; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

    expect(harness.imports.resumePending()).toBe(1)
    harness.currentAttachment.value = attachment()
    expect(harness.imports.resumePending()).toBe(1)
    expect(harness.imports.resumePending()).toBe(0)

    first.resolve({ mediaAssetId: 42, url: 'https://app.example/old', width: 100, height: 100 })
    await flushPromises()
    expect(
      ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem).libraryImport,
    ).toEqual({
      state: 'pending',
      libraryAssetId: 7,
    })

    second.resolve({ mediaAssetId: 43, url: 'https://app.example/new', width: 100, height: 100 })
    await flushPromises()
    expect(
      ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem)
        .sourceMediaAssetId,
    ).toBe(43)
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
      libraryAssetId: 7,
      failureMessage: 'Library import failed. Please try again later.',
    })
    expect(harness.imports.resumePending()).toBe(0)
    expect(harness.materialize).toHaveBeenCalledTimes(1)

    harness.materialize.mockResolvedValueOnce({
      mediaAssetId: 42,
      url: 'https://app.example/media/42',
      width: 200,
      height: 100,
    })
    expect(harness.imports.retry('image-1')).toBe(true)
    await flushPromises()
    expect(harness.materialize).toHaveBeenCalledTimes(2)
    expect(harness.materialize).toHaveBeenNthCalledWith(2, 7)
    expect(
      ((harness.nodes.value[0]!.data as FlowNodeData).snapshotNode as ImageNodeItem).libraryImport,
    ).toBeUndefined()
  })

  it('does not apply a late completion after the node is deleted', async () => {
    const harness = createHarness([pendingNode()])
    const request = deferred<{ mediaAssetId: number; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValue(request.promise)
    harness.imports.resumePending()
    harness.nodes.value = []
    request.resolve({
      mediaAssetId: 42,
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
      libraryAssetId: 7,
      failureMessage: '源图不可访问',
    }
    const harness = createHarness([failedNode])
    const request = deferred<{ mediaAssetId: number; url: string; width: number; height: number }>()
    harness.materialize.mockReturnValue(request.promise)

    expect(harness.imports.retry('image-1')).toBe(true)
    harness.nodes.value = []
    request.resolve({
      mediaAssetId: 42,
      url: 'https://app.example/media/42',
      width: 200,
      height: 100,
    })

    await flushPromises()
    expect(harness.materialize).toHaveBeenCalledWith(7)
    expect(harness.nodes.value).toEqual([])
  })
})
