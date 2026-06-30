import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { usePendingImageResultNodes } from './usePendingImageResultNodes'

vi.mock('../../utils/animationFrame', () => ({
  waitForNextRenderFrame: vi.fn(async () => undefined),
}))

function makeAnchorNode(id = 'anchor'): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'prompt',
      snapshotNode: {
        width: 280,
      },
    },
  }
}

function makeImageSnapshot(overrides: Partial<ImageNodeItem> = {}): ImageNodeItem {
  return {
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/image.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
    ...overrides,
  }
}

function makeImageNode(
  nodeId: string,
  snapshotOverrides: Partial<ImageNodeItem> = {},
): Node<FlowNodeData> {
  const snapshot = makeImageSnapshot(snapshotOverrides)
  return {
    id: nodeId,
    type: 'canvas',
    position: { x: snapshot.x, y: snapshot.y },
    data: {
      kind: 'image',
      nodeType: 'image',
      title: 'Image',
      snapshotNode: snapshot,
    },
  }
}

function makeImageEdge(target: string, overrides: Partial<Edge> = {}): Edge {
  return {
    id: `edge-${target}`,
    source: 'anchor',
    sourceHandle: 'out:image',
    target,
    targetHandle: 'in:image',
    ...overrides,
  }
}

function createHarness(initialNodes: Node[], initialEdges: Edge[] = []) {
  const nodes: Ref<Node[]> = shallowRef([...initialNodes])
  const edges: Ref<Edge[]> = shallowRef([...initialEdges])

  const actions = usePendingImageResultNodes({
    nodes,
    edges,
    commitGraphMutation: (mutator) => {
      const next = mutator(nodes.value, edges.value)
      nodes.value = next.nodes
      edges.value = next.edges
    },
    commitNodeMutation: (mutator) => {
      nodes.value = mutator(nodes.value)
    },
    commitEdgeMutation: (mutator) => {
      edges.value = mutator(edges.value)
    },
  })

  return {
    actions,
    nodes,
    edges,
  }
}

function readImageSnapshot(nodes: Node[], nodeId: string): ImageNodeItem {
  const snapshot = nodes.find((node) => node.id === nodeId)?.data?.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') throw new Error(`missing image node ${nodeId}`)
  return snapshot as ImageNodeItem
}

describe('usePendingImageResultNodes', () => {
  it('reuses the first valid empty image child and ignores unrelated edges', () => {
    const invalidEmptyNode = makeImageNode('invalid-empty', {
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const firstReusableNode = makeImageNode('first-reusable', {
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const laterReusableNode = makeImageNode('later-reusable', {
      sourceImageUrl: '',
      outputStatus: 'idle',
    })
    const harness = createHarness(
      [makeAnchorNode(), invalidEmptyNode, firstReusableNode, laterReusableNode],
      [
        makeImageEdge('invalid-empty', { sourceHandle: 'out:prompt' }),
        makeImageEdge('first-reusable'),
        makeImageEdge('later-reusable'),
      ],
    )

    const resultNodeIds = harness.actions.appendPendingImageResultNodes(
      { url: 'https://example.com/source.png', ratio: 1 },
      'anchor',
      1,
      { targetRatio: 1, targetWidth: 320 },
    )

    expect(resultNodeIds).toEqual(['first-reusable'])
    expect(harness.nodes.value).toHaveLength(4)
    expect(readImageSnapshot(harness.nodes.value, 'invalid-empty').outputStatus).toBe('idle')
    expect(readImageSnapshot(harness.nodes.value, 'first-reusable')).toMatchObject({
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputStatus: 'running',
      outputImages: [],
      width: 320,
    })
  })

  it('places new results below the lowest valid image child only', () => {
    const lowChild = makeImageNode('low-child', { x: 420, y: 400, width: 280 })
    const highChild = makeImageNode('high-child', { x: 390, y: 40, width: 280 })
    const unrelatedFarChild = makeImageNode('unrelated-far-child', {
      x: 960,
      y: 3000,
      width: 280,
    })
    const harness = createHarness(
      [makeAnchorNode(), highChild, lowChild, unrelatedFarChild],
      [
        makeImageEdge('high-child'),
        makeImageEdge('low-child'),
        makeImageEdge('unrelated-far-child', { targetHandle: 'in:mask' }),
      ],
    )

    const resultNodeId = harness.actions.appendPendingImageResultNode(
      { url: 'https://example.com/source.png', ratio: 1 },
      'anchor',
      { targetRatio: 1, targetWidth: 280 },
    )
    const resultNode = harness.nodes.value.find((node) => node.id === resultNodeId)

    expect(resultNode?.position).toEqual({
      x: 420,
      y: 720,
    })
  })

  it('uses runtime layouts so batched pending nodes stack below each other', () => {
    const harness = createHarness([makeAnchorNode()])

    const resultNodeIds = harness.actions.appendPendingImageResultNodes(
      { url: 'https://example.com/source.png', ratio: 1 },
      'anchor',
      2,
      { targetRatio: 1, targetWidth: 280 },
    )
    const [firstNodeId, secondNodeId] = resultNodeIds
    const firstNode = harness.nodes.value.find((node) => node.id === firstNodeId)
    const secondNode = harness.nodes.value.find((node) => node.id === secondNodeId)

    expect(firstNode?.position).toEqual({ x: 380, y: 0 })
    expect(secondNode?.position).toEqual({ x: 380, y: 320 })
  })
})
