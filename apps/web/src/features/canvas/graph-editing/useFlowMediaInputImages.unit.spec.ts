import { describe, expect, it, vi } from 'vitest'
import { nextTick, shallowRef } from 'vue'
import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type {
  CanvasNodeItem,
  ImageNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../board/domain'
import { createEmptyBoardSnapshot } from '../../board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { CanvasGraphSession } from '../graph-runtime'
import { flowConnectionToCanvasConnection } from '../utils/flowConnectionAdapter'
import { useFlowMediaInputImages } from './useFlowMediaInputImages'
import { useCanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { MediaRef } from '../../../shared/media/mediaRef'

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    title: 'Image',
    sourceImageUrl: 'https://example.com/source.png',
    sourceMediaRef: {
      assetId: 'asset_101',
      derivativeTier: 'preview',
    },
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makePromptSnapshot(overrides: Partial<PromptNodeItem> = {}): PromptNodeItem {
  return {
    id: 2,
    type: 'prompt',
    x: 320,
    y: 0,
    width: 280,
    title: 'Prompt',
    text: '',
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
    ...overrides,
  }
}

function makeFlowNode(
  id: string,
  kind: FlowNodeData['kind'],
  snapshotNode: ImageNodeItem | PromptNodeItem,
): Node<FlowNodeData> {
  return {
    id,
    type: 'canvas',
    position: { x: snapshotNode.x, y: snapshotNode.y },
    data: {
      kind,
      nodeType: kind,
      title: snapshotNode.title || kind,
      snapshotNode,
    },
  }
}

function createGraphSession(nodes: Node[], edges: Edge[]) {
  const snapshot = createEmptyBoardSnapshot()
  for (const node of nodes) {
    const snapshotNode = node.data?.snapshotNode as CanvasNodeItem | undefined
    if (!snapshotNode) continue
    if (snapshotNode.type === 'image-node') snapshot.imageNodes.push(snapshotNode)
    else if (snapshotNode.type === 'prompt') snapshot.promptNodes.push(snapshotNode)
    else snapshot.fusionNodes.push(snapshotNode)
  }
  for (const edge of edges) {
    const match = /^connection-(\d+)$/.exec(edge.id)
    if (!match) continue
    const connection = flowConnectionToCanvasConnection(Number(match[1]), edge)
    if (connection) snapshot.connections.push(connection)
  }
  return CanvasGraphSession.hydrate('board-a', snapshot)
}

function createHarness(nodes: Ref<Node[]>, edges: Ref<Edge[]>) {
  const graphActions = {
    setNodes: vi.fn(),
    setEdges: vi.fn(),
  }
  const graphAdapter = useCanvasVueFlowGraphAdapter({
    nodes,
    edges,
    actions: graphActions,
  })
  const markFlowExecutionNodeDirty = vi.fn()
  const graphSession = createGraphSession(nodes.value, edges.value)
  const updateSnapshotNodeBase = vi.fn(
    (nodeId: string, mutate: (snapshotNode: CanvasNodeItem) => CanvasNodeItem) => {
      const target = graphSession.store.getNode(nodeId as `prompt-${number}` | `fusion-${number}`)
      if (target) {
        const transaction = graphSession.store.beginTransaction('node-update', {
          recordHistory: false,
          markDirty: false,
        })
        transaction.updateNode(target.id, mutate)
        transaction.commit()
      }
      nodes.value = nodes.value.map((node) => {
        if (node.id !== nodeId) return node
        const snapshotNode = node.data?.snapshotNode
        if (!snapshotNode || typeof snapshotNode !== 'object') return node
        const nextSnapshotNode = mutate(snapshotNode as CanvasNodeItem)
        if (nextSnapshotNode === snapshotNode) return node
        return {
          ...node,
          data: {
            ...node.data,
            snapshotNode: nextSnapshotNode,
          },
        }
      })
    },
  )

  const uploadImageToBoard = vi.fn(async () => ({
    url: 'https://example.com/external.png',
    ratio: 0.75,
    assetId: 'asset_501',
    mediaRef: {
      assetId: 'asset_501',
      derivativeTier: 'preview',
    } satisfies MediaRef,
  }))
  const api = useFlowMediaInputImages({
    browser: {
      pickImageFile: vi.fn(async () => null),
      readDroppedFile: (event) => event.dataTransfer?.files?.[0] ?? null,
    },
    nodes,
    edges,
    uploadImageToBoard,
    updateSnapshotNodeBase,
    readCanvasGraphStore: () => graphSession.store,
    graphAdapter,
    markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload: (nodeId) => {
      const node = nodes.value.find((item) => item.id === nodeId)
      const snapshot = node?.data?.snapshotNode as ImageNodeItem | undefined
      if (!snapshot?.sourceImageUrl) return null
      return {
        url: snapshot.sourceImageUrl,
        ratio: snapshot.sourceImageRatio,
        mediaRef: snapshot.sourceMediaRef,
      }
    },
    openImageUrlPreview: vi.fn(),
    onWarning: vi.fn(),
  })

  return {
    ...api,
    uploadImageToBoard,
    updateSnapshotNodeBase,
    graphActions,
    markFlowExecutionNodeDirty,
    graphSession,
    project: () => {
      const snapshot = graphSession.store.exportSnapshot()
      const nextEdges = snapshot.connections.map((connection) => ({
        id: `connection-${connection.id}`,
        source: `${connection.fromNodeType}-${connection.fromNodeId}`,
        sourceHandle: `out:${connection.fromPort}`,
        target: `${connection.toNodeType}-${connection.toNodeId}`,
        targetHandle: `in:${connection.toPort}`,
      }))
      edges.value = nextEdges as Edge[]
    },
  }
}

describe('useFlowMediaInputImages', () => {
  it('does not resync linked media inputs when only node positions change', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode(
        'prompt-1',
        'prompt',
        makePromptSnapshot({
          id: 1,
          inputImages: [
            {
              id: 'node:image-1',
              sourceType: 'node',
              sourceId: 'image-1',
              sourceNodeId: 'image-1',
              url: 'https://example.com/source.png',
              ratio: 1,
            } satisfies PromptNodeInputImage,
          ],
        }),
      ),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'connection-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'prompt-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])
    const { updateSnapshotNodeBase } = createHarness(nodes, edges)
    updateSnapshotNodeBase.mockClear()

    nodes.value = nodes.value.map((node) =>
      node.id === 'prompt-1' ? { ...node, position: { x: 500, y: 600 } } : node,
    )
    await nextTick()

    expect(updateSnapshotNodeBase).not.toHaveBeenCalled()
  })

  it('syncs a linked image when an image input edge is added', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode('prompt-1', 'prompt', makePromptSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([])
    const harness = createHarness(nodes, edges)

    harness.graphSession.store
      .beginTransaction('connect', { recordHistory: false, markDirty: false })
      .addEdge({
        id: 1,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 2,
        toPort: 'image',
      })
      .commit()
    harness.project()
    await nextTick()

    expect(harness.getMediaInputImages('prompt-2')).toEqual([
      {
        id: 'node:image-1',
        sourceType: 'node',
        sourceId: 'image-1',
        sourceNodeId: 'image-1',
        url: 'https://example.com/source.png',
        ratio: 1,
        mediaRef: {
          assetId: 'asset_101',
          derivativeTier: 'preview',
        },
      },
    ])
  })

  it('preserves the public asset reference for an external input upload', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('prompt-1', 'prompt', makePromptSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([])
    const { getMediaInputImages, onFlowMediaInputImageDrop } = createHarness(nodes, edges)
    const file = new File(['image'], 'input.png', { type: 'image/png' })

    await onFlowMediaInputImageDrop('prompt-2', {
      dataTransfer: { files: [file] },
    } as unknown as DragEvent)

    expect(getMediaInputImages('prompt-2')).toEqual([
      expect.objectContaining({
        sourceType: 'external',
        url: 'https://example.com/external.png',
        ratio: 0.75,
        mediaRef: {
          assetId: 'asset_501',
          derivativeTier: 'preview',
        },
      }),
    ])
  })

  it('removes a node-linked media input and its edge through the graph adapter', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode(
        'prompt-1',
        'prompt',
        makePromptSnapshot({
          id: 1,
          inputImages: [
            {
              id: 'node:image-1',
              sourceType: 'node',
              sourceId: 'image-1',
              sourceNodeId: 'image-1',
              url: 'https://example.com/source.png',
              ratio: 1,
            } satisfies PromptNodeInputImage,
          ],
        }),
      ),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'connection-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'prompt-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])
    const { onFlowMediaInputImageRemove, graphActions, markFlowExecutionNodeDirty } = createHarness(
      nodes,
      edges,
    )

    onFlowMediaInputImageRemove('prompt-1', 'node:image-1')
    await nextTick()

    expect(nodes.value.find((node) => node.id === 'prompt-1')?.data?.snapshotNode).toMatchObject({
      inputImages: [],
    })
    expect(edges.value).toEqual([])
    expect(graphActions.setNodes).toHaveBeenCalledOnce()
    expect(graphActions.setEdges).toHaveBeenCalledOnce()
    expect(markFlowExecutionNodeDirty).toHaveBeenCalledWith('prompt-1', { includeSelf: true })
  })

  it('removes from canonical Graph state when the Flow projection is stale', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode(
        'prompt-1',
        'prompt',
        makePromptSnapshot({
          id: 1,
          inputImages: [
            {
              id: 'node:image-1',
              sourceType: 'node',
              sourceId: 'image-1',
              sourceNodeId: 'image-1',
              url: 'https://example.com/source.png',
              ratio: 1,
            } satisfies PromptNodeInputImage,
          ],
        }),
      ),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'connection-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'prompt-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])
    const { graphSession, onFlowMediaInputImageRemove } = createHarness(nodes, edges)

    nodes.value = [
      makeFlowNode('prompt-1', 'prompt', makePromptSnapshot({ id: 1, inputImages: [] })),
    ] as Node[]
    edges.value = []
    onFlowMediaInputImageRemove('prompt-1', 'node:image-1')

    expect(graphSession.store.exportSnapshot().promptNodes[0]?.inputImages).toEqual([])
    expect(graphSession.store.exportSnapshot().connections).toEqual([])
    expect(nodes.value.map((node) => node.id)).toEqual(['image-1', 'prompt-1'])
    expect(edges.value).toEqual([])
  })

  it('removes a node-linked media input when its connection disappears', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('image-1', 'image', makeImageSnapshot()),
      makeFlowNode(
        'prompt-1',
        'prompt',
        makePromptSnapshot({
          inputImages: [
            {
              id: 'node:image-1',
              sourceType: 'node',
              sourceId: 'image-1',
              sourceNodeId: 'image-1',
              url: 'https://example.com/source.png',
              ratio: 1,
              mediaAssetId: 101,
            },
          ],
        }),
      ),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'prompt-1',
        targetHandle: 'in:image',
      },
    ] as Edge[])
    const { getMediaInputImages } = createHarness(nodes, edges)

    edges.value = []
    await nextTick()

    expect(getMediaInputImages('prompt-1')).toEqual([])
  })
})
