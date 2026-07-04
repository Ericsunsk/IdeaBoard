import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { shallowRef } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { FlowNodeData } from '../../types/flowTypes'
import { usePendingImageResultNodeLifecycle } from './usePendingImageResultNodes'

const animationFrameMocks = vi.hoisted(() => ({
  waitForNextRenderFrame: vi.fn<() => Promise<void>>(async () => undefined),
}))
vi.mock('../../utils/animationFrame', () => ({
  waitForNextRenderFrame: animationFrameMocks.waitForNextRenderFrame,
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

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

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

  const actions = usePendingImageResultNodeLifecycle({
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
  beforeEach(() => {
    animationFrameMocks.waitForNextRenderFrame.mockReset()
    animationFrameMocks.waitForNextRenderFrame.mockResolvedValue(undefined)
  })

  it('reserves a pending result node through the lifecycle interface', () => {
    const harness = createHarness([makeAnchorNode()])

    const reservation = harness.actions.reservePendingResult({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      resultOptions: { targetRatio: 1, targetWidth: 320 },
    })

    const pending = readImageSnapshot(harness.nodes.value, reservation.resultNodeId)
    expect(pending).toMatchObject({
      sourceImageUrl: '',
      sourceImageRatio: 1,
      outputStatus: 'running',
      outputImages: [],
      width: 320,
    })
    expect(pending.pendingTaskId).toBeUndefined()
  })

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

    const resultNodeIds = harness.actions
      .reservePendingResults({
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'anchor',
        outputCount: 1,
        resultOptions: { targetRatio: 1, targetWidth: 320 },
      })
      .map((reservation) => reservation.resultNodeId)

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

    const resultNodeId = harness.actions.reservePendingResult({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      resultOptions: { targetRatio: 1, targetWidth: 280 },
    }).resultNodeId
    const resultNode = harness.nodes.value.find((node) => node.id === resultNodeId)

    expect(resultNode?.position).toEqual({
      x: 420,
      y: 720,
    })
  })

  it('uses runtime layouts so batched pending nodes stack below each other', () => {
    const harness = createHarness([makeAnchorNode()])

    const resultNodeIds = harness.actions
      .reservePendingResults({
        source: { url: 'https://example.com/source.png', ratio: 1 },
        sourceNodeId: 'anchor',
        outputCount: 2,
        resultOptions: { targetRatio: 1, targetWidth: 280 },
      })
      .map((reservation) => reservation.resultNodeId)
    const [firstNodeId, secondNodeId] = resultNodeIds
    const firstNode = harness.nodes.value.find((node) => node.id === firstNodeId)
    const secondNode = harness.nodes.value.find((node) => node.id === secondNodeId)

    expect(firstNode?.position).toEqual({ x: 380, y: 0 })
    expect(secondNode?.position).toEqual({ x: 380, y: 320 })
  })

  it('attaches a task id and materializes a resolved image result', async () => {
    const harness = createHarness([makeAnchorNode()])

    const { resultNodeId } = harness.actions.reservePendingResult({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
      resultOptions: { targetRatio: 1, targetWidth: 280 },
    })
    await flushPromises()
    expect(harness.edges.value).toHaveLength(1)
    expect(harness.edges.value[0]).toMatchObject({
      source: 'anchor',
      target: resultNodeId,
      animated: true,
    })

    harness.actions.attachPendingTaskId({ resultNodeId, taskId: ' task-1 ' })
    expect(readImageSnapshot(harness.nodes.value, resultNodeId).pendingTaskId).toBe('task-1')

    harness.actions.materializeResolvedImage({
      resultNodeId,
      result: {
        url: 'https://example.com/result.png',
        ratio: 1,
        width: 400,
        height: 200,
      },
    })

    expect(readImageSnapshot(harness.nodes.value, resultNodeId)).toMatchObject({
      sourceImageUrl: 'https://example.com/result.png',
      sourceImageRatio: 0.5,
      outputStatus: 'ready',
      outputImages: [],
      width: 520,
    })
    expect(readImageSnapshot(harness.nodes.value, resultNodeId).pendingTaskId).toBeUndefined()
    expect(harness.edges.value[0]?.animated).toBe(false)
  })

  it('cancels a pending result before the delayed edge append resolves', async () => {
    const nextFrame = createDeferred<void>()
    animationFrameMocks.waitForNextRenderFrame.mockReturnValueOnce(nextFrame.promise)
    const harness = createHarness([makeAnchorNode()])

    const { resultNodeId } = harness.actions.reservePendingResult({
      source: { url: 'https://example.com/source.png', ratio: 1 },
      sourceNodeId: 'anchor',
    })

    harness.actions.cancelPendingResult({ resultNodeId })
    expect(harness.nodes.value.some((node) => node.id === resultNodeId)).toBe(false)

    nextFrame.resolve(undefined)
    await flushPromises()

    expect(harness.edges.value).toEqual([])
  })
})
