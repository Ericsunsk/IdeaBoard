import { describe, expect, it, vi } from 'vitest'
import type { Connection, Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../board'
import { CanvasGraphStore, isCanvasGraphNodeId, type CanvasGraphNodeId } from '../../domain/graph'
import { createCanvasConnectionCommandLifecycle } from './connectionCommandLifecycle'
import { createCanvasDeleteCommandLifecycle } from './deleteCommandLifecycle'
import { toCanvasSemanticEdges } from './canvasEdgeProjection'
import type { PendingImageExecutionLease } from '../../application/execution/pending-execution'

function createImageNode(id: number): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: id * 10,
    y: 20,
    width: 280,
    sourceImageUrl: `https://example.com/source-${id}.png`,
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'ready',
    outputImages: [],
  }
}

function createPromptNode(id: number): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x: id * 10,
    y: 320,
    width: 280,
    title: `Prompt ${id}`,
    text: `prompt ${id}`,
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }
}

function createSnapshot(): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = [createImageNode(1), createImageNode(2)]
  snapshot.promptNodes = [createPromptNode(1)]
  snapshot.connections = [
    {
      id: 1,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'prompt',
      toNodeId: 1,
      toPort: 'image',
    },
  ]
  return snapshot
}

function createFlowEdge(id: string, source: string, target: string): Edge {
  return {
    id,
    source,
    target,
    sourceHandle: 'out:image',
    targetHandle: 'in:image',
  } as Edge
}

function resolveTestRemovedFlowNodes(changes: readonly NodeChange[]) {
  const graphNodeIds: CanvasGraphNodeId[] = []
  const flowNodeIds: string[] = []
  for (const change of changes) {
    if (change.type !== 'remove') continue
    flowNodeIds.push(change.id)
    if (isCanvasGraphNodeId(change.id)) graphNodeIds.push(change.id)
  }
  return graphNodeIds.length > 0 ? { graphNodeIds, flowNodeIds } : null
}

describe('vue flow projection adapter contract', () => {
  it('shares one normalized validation between preview and commit', () => {
    const store = CanvasGraphStore.fromSnapshot(createSnapshot())
    const nodes: Node[] = [
      { id: 'image-1', position: { x: 0, y: 0 }, data: {} },
      { id: 'image-2', position: { x: 0, y: 0 }, data: {} },
      { id: 'prompt-1', position: { x: 0, y: 0 }, data: {} },
    ] as Node[]
    const edges: Edge[] = [createFlowEdge('connection-1', 'image-1', 'prompt-1')]
    const commits: string[] = []
    store.subscribe((event) => commits.push(event.summary.reason))
    const onWarning = vi.fn()
    const adapter = createCanvasConnectionCommandLifecycle({
      readNodes: () => nodes,
      readEdges: () => edges,
      readBoardBaseSnapshot: () => undefined,
      readGraphStore: () => store,
      isValidConnection: () => true,
      allowsMultipleTargetConnections: () => false,
      validateConnectionMutation: (connection, existingEdges) =>
        existingEdges.filter(
          (edge) => edge.target === connection.target && edge.targetHandle === 'in:image',
        ).length >= 1
          ? 'input limit reached'
          : null,
      graphAdapter: { removeEdges: vi.fn() } as never,
      onWarning,
      markFlowExecutionNodeDirty: () => undefined,
    })

    // The prompt input is already occupied, so preview and commit must agree.
    const occupied: Connection = {
      source: 'image-2',
      target: 'prompt-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    }
    expect(adapter.vueFlowBindings.isValidConnection(occupied)).toBe(false)
    expect(
      adapter.connectIntent({
        sourceNodeId: 'image-2',
        targetNodeId: 'prompt-1',
        sourcePort: 'image',
        targetPort: 'image',
      }),
    ).toBe(false)
    expect(onWarning).toHaveBeenCalledWith('input limit reached')
    expect(commits).toHaveLength(0)

    edges.length = 0
    onWarning.mockClear()
    expect(adapter.vueFlowBindings.isValidConnection(occupied)).toBe(true)
    expect(
      adapter.connectIntent({
        sourceNodeId: 'image-2',
        targetNodeId: 'prompt-1',
        sourcePort: 'image',
        targetPort: 'image',
      }),
    ).toBe(true)
    expect(onWarning).not.toHaveBeenCalled()
    expect(commits).toEqual(['connect'])
  })

  it('resolves pending delete targets from execution leases, not snapshot fields', () => {
    const snapshot = createSnapshot()
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const nodes: Node[] = [
      { id: 'image-1', position: { x: 0, y: 0 }, data: { kind: 'image' } },
      {
        id: 'image-2',
        position: { x: 0, y: 0 },
        data: {
          kind: 'image',
          snapshotNode: { outputStatus: 'running', pendingTaskId: 'task-stale-snapshot' },
        },
      },
    ] as Node[]
    const edges: Edge[] = [createFlowEdge('connection-9', 'image-1', 'image-2')]
    const microtasks: Array<() => void> = []
    let leases: PendingImageExecutionLease[] = []
    const released: PendingImageExecutionLease[][] = []
    const lifecycle = createCanvasDeleteCommandLifecycle({
      readNodes: () => nodes,
      readEdges: () => edges,
      readGraphStore: () => store,
      resolveRemovedFlowNodes: resolveTestRemovedFlowNodes,
      scheduleMicrotask: (callback) => {
        microtasks.push(callback)
      },
      onWarning: () => undefined,
      captureActiveNodeExecutions: () => leases,
      releaseNodeExecutions: (captured) => {
        released.push([...captured])
      },
    })

    // A stale `running` snapshot without a live lease must not delete the target.
    expect(lifecycle.deleteNode('image-1')).toBe(true)
    expect(store.getNode('image-1')).toBeUndefined()
    expect(store.getNode('image-2')).toBeDefined()

    // With a live lease the fed target is a pending result and goes with its source.
    const storeWithLease = CanvasGraphStore.fromSnapshot(createSnapshot())
    leases = [{ resultNodeId: 'image-2' } as unknown as PendingImageExecutionLease]
    const lifecycleWithLease = createCanvasDeleteCommandLifecycle({
      readNodes: () => nodes,
      readEdges: () => edges,
      readGraphStore: () => storeWithLease,
      resolveRemovedFlowNodes: resolveTestRemovedFlowNodes,
      scheduleMicrotask: (callback) => {
        microtasks.push(callback)
      },
      onWarning: () => undefined,
      captureActiveNodeExecutions: () => leases,
      releaseNodeExecutions: (captured) => {
        released.push([...captured])
      },
    })
    expect(lifecycleWithLease.deleteNode('image-1')).toBe(true)
    expect(storeWithLease.getNode('image-1')).toBeUndefined()
    expect(storeWithLease.getNode('image-2')).toBeUndefined()
    expect(released.length).toBeGreaterThan(0)
  })

  it('keeps a hydration-pending target without a live lease when its source is deleted', () => {
    // Store-level `running` + `pendingTaskId` are hydration-only facts. Before the
    // reattach driver claims a lease, deleting the source must not cascade.
    const snapshot = createSnapshot()
    snapshot.imageNodes[1] = {
      ...snapshot.imageNodes[1]!,
      outputStatus: 'running',
      pendingTaskId: 'task-hydrated-not-reattached',
      outputImages: [],
    }
    snapshot.connections = [
      {
        id: 9,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'image',
        toNodeId: 2,
        toPort: 'image',
      },
    ]
    const store = CanvasGraphStore.fromSnapshot(snapshot)
    const nodes: Node[] = [
      { id: 'image-1', position: { x: 0, y: 0 }, data: { kind: 'image' } },
      { id: 'image-2', position: { x: 0, y: 0 }, data: { kind: 'image' } },
    ] as Node[]
    const edges: Edge[] = [createFlowEdge('connection-9', 'image-1', 'image-2')]
    const lifecycle = createCanvasDeleteCommandLifecycle({
      readNodes: () => nodes,
      readEdges: () => edges,
      readGraphStore: () => store,
      resolveRemovedFlowNodes: resolveTestRemovedFlowNodes,
      scheduleMicrotask: (callback) => callback(),
      onWarning: () => undefined,
      captureActiveNodeExecutions: () => [],
      releaseNodeExecutions: () => undefined,
    })

    expect(lifecycle.deleteNode('image-1')).toBe(true)
    expect(store.getNode('image-1')).toBeUndefined()
    expect(store.getNode('image-2')).toBeDefined()
    expect(store.getNode('image-2')?.snapshot).toMatchObject({
      outputStatus: 'running',
      pendingTaskId: 'task-hydrated-not-reattached',
    })
  })

  it('coalesces native remove changes into one transaction', () => {
    const store = CanvasGraphStore.fromSnapshot(createSnapshot())
    const nodes: Node[] = [
      { id: 'image-1', position: { x: 0, y: 0 }, data: {} },
      { id: 'prompt-1', position: { x: 0, y: 0 }, data: {} },
    ] as Node[]
    const edges: Edge[] = [createFlowEdge('connection-1', 'image-1', 'prompt-1')]
    const microtasks: Array<() => void> = []
    const commits: string[] = []
    store.subscribe((event) => commits.push(event.summary.reason))
    const lifecycle = createCanvasDeleteCommandLifecycle({
      readNodes: () => nodes,
      readEdges: () => edges,
      readGraphStore: () => store,
      resolveRemovedFlowNodes: resolveTestRemovedFlowNodes,
      scheduleMicrotask: (callback) => {
        microtasks.push(callback)
      },
      onWarning: () => undefined,
      captureActiveNodeExecutions: () => [],
      releaseNodeExecutions: () => undefined,
    })

    expect(lifecycle.handleNodeChanges([{ id: 'image-1', type: 'remove' } as NodeChange])).toBe(
      true,
    )
    expect(
      lifecycle.handleEdgeChanges([{ id: 'connection-1', type: 'remove' } as EdgeChange]),
    ).toBe(true)
    while (microtasks.length > 0) microtasks.shift()?.()

    expect(commits).toHaveLength(1)
    expect(store.getNode('image-1')).toBeUndefined()
    expect(store.exportSnapshot().connections).toEqual([])
  })

  it('keeps renderer animation out of the semantic edge contract', () => {
    const edges = [{ id: 'connection-1', animated: true } as Edge]
    const semantic = toCanvasSemanticEdges(edges)
    expect(semantic[0]).not.toHaveProperty('animated')
    expect(semantic[0]?.id).toBe('connection-1')
  })
})
