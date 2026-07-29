import { describe, expect, it, vi } from 'vitest'
import { nextTick, shallowRef } from 'vue'
import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type {
  CanvasNodeItem,
  ImageNodeItem,
  PromptNodeInputImage,
  PromptNodeItem,
} from '../../canvas-board/domain'
import type { FlowNodeData } from '../types/flowTypes'
import { useFlowMediaInputImages } from './useFlowMediaInputImages'
import { useCanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    title: 'Image',
    sourceImageUrl: 'https://example.com/source.png',
    sourceMediaAssetId: 101,
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
  const updateSnapshotNodeBase = vi.fn(
    (nodeId: string, mutate: (snapshotNode: CanvasNodeItem) => CanvasNodeItem) => {
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
    mediaAssetId: 501,
  }))
  const api = useFlowMediaInputImages({
    nodes,
    edges,
    uploadImageToBoard,
    updateSnapshotNodeBase,
    graphAdapter,
    markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload: (nodeId) => {
      const node = nodes.value.find((item) => item.id === nodeId)
      const snapshot = node?.data?.snapshotNode as ImageNodeItem | undefined
      if (!snapshot?.sourceImageUrl) return null
      return {
        url: snapshot.sourceImageUrl,
        ratio: snapshot.sourceImageRatio,
        mediaAssetId: snapshot.sourceMediaAssetId,
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
        id: 'edge-1',
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
    const { getMediaInputImages } = createHarness(nodes, edges)

    edges.value = [
      {
        id: 'edge-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'prompt-1',
        targetHandle: 'in:image',
      },
    ] as Edge[]
    await nextTick()

    expect(getMediaInputImages('prompt-1')).toEqual([
      {
        id: 'node:image-1',
        sourceType: 'node',
        sourceId: 'image-1',
        sourceNodeId: 'image-1',
        url: 'https://example.com/source.png',
        ratio: 1,
        mediaAssetId: 101,
      },
    ])
  })

  it('preserves the media asset id for an external input upload', async () => {
    const nodes: Ref<Node[]> = shallowRef([
      makeFlowNode('prompt-1', 'prompt', makePromptSnapshot()),
    ] as Node[])
    const edges: Ref<Edge[]> = shallowRef([])
    const { getMediaInputImages, onFlowMediaInputImageDrop } = createHarness(nodes, edges)
    const file = new File(['image'], 'input.png', { type: 'image/png' })

    await onFlowMediaInputImageDrop('prompt-1', {
      dataTransfer: { files: [file] },
    } as unknown as DragEvent)

    expect(getMediaInputImages('prompt-1')).toEqual([
      expect.objectContaining({
        sourceType: 'external',
        url: 'https://example.com/external.png',
        ratio: 0.75,
        mediaAssetId: 501,
      }),
    ])
  })

  it('removes a node-linked media input and its edge through the graph adapter', () => {
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
            } satisfies PromptNodeInputImage,
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
    const {
      onFlowMediaInputImageRemove,
      getMediaInputImages,
      graphActions,
      markFlowExecutionNodeDirty,
    } = createHarness(nodes, edges)

    onFlowMediaInputImageRemove('prompt-1', 'node:image-1')

    expect(getMediaInputImages('prompt-1')).toEqual([])
    expect(edges.value).toEqual([])
    expect(graphActions.setNodes).toHaveBeenCalledOnce()
    expect(graphActions.setEdges).toHaveBeenCalledOnce()
    expect(markFlowExecutionNodeDirty).toHaveBeenCalledWith('prompt-1', { includeSelf: true })
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
