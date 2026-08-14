import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../canvas-board/domain'
import {
  createCanvasGraphStore,
  type CanvasGraphEdgeId,
  type CanvasGraphNodeId,
  type CanvasGraphTransactionSummary,
} from '../graph-runtime'
import { createFlowEdgeProjectionIndex } from './flowConnectionIdentity'
import { applyFlowGraphDeleteProjection } from './flowGraphDeleteProjection'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
import type { CanvasExecutionLease } from '../execution/runtime/CanvasExecutionRuntime'

function createDeleteSummary(
  options: {
    reason?: CanvasGraphTransactionSummary['reason']
    removedNodeIds?: CanvasGraphNodeId[]
    removedEdgeIds?: CanvasGraphEdgeId[]
  } = {},
): CanvasGraphTransactionSummary {
  const removedNodeIds = options.removedNodeIds ?? []
  const removedEdgeIds = options.removedEdgeIds ?? []
  return {
    id: 'tx-delete',
    reason: options.reason ?? 'delete',
    didChange: true,
    hasDomainChanges: true,
    changedNodeIds: [],
    changedEdgeIds: [],
    changedGroupIds: [],
    removedNodeIds,
    removedEdgeIds,
    removedGroupIds: [],
    selectedNodeIds: [],
    selectedEdgeIds: [],
    didChangeOrder: false,
    shouldRecordHistory: true,
    shouldMarkDirty: true,
    projectionPatch: {
      nodes: {
        added: [],
        updated: [],
        removedIds: removedNodeIds,
        selectionChanged: [],
      },
      edges: {
        added: [],
        removedIds: removedEdgeIds,
        selectionChanged: [],
      },
      groups: {
        added: [],
        updated: [],
        removedIds: [],
      },
    },
  }
}

function createImageNode(id: string, taskId?: string): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: {
      kind: 'image',
      nodeType: 'image',
      snapshotNode: {
        id: 1,
        type: 'image-node',
        outputStatus: taskId ? 'running' : 'ready',
        pendingTaskId: taskId,
      },
    },
  } as Node
}

function createPromptNode(id = 'prompt-1'): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 320, y: 0 },
    data: {
      kind: 'prompt',
      nodeType: 'prompt',
      snapshotNode: {
        id: 1,
        type: 'prompt',
      },
    },
  } as Node
}

function createGraphAdapter() {
  const applyGraphChanges = vi.fn(
    (
      _changes: Parameters<CanvasVueFlowGraphAdapter['applyGraphChanges']>[0],
      commandOptions: Parameters<CanvasVueFlowGraphAdapter['applyGraphChanges']>[1],
    ) => ({
      command: 'applyGraphChanges' as const,
      before: { nodes: [] as Node[], edges: [] as Edge[] },
      after: { nodes: [] as Node[], edges: [] as Edge[] },
      didChange: true,
      metadata: commandOptions?.metadata,
    }),
  )
  return {
    graphAdapter: { applyGraphChanges } as Pick<CanvasVueFlowGraphAdapter, 'applyGraphChanges'>,
    applyGraphChanges,
  }
}

function createProjectionIndex(edges: Edge[]) {
  const snapshot = createEmptyCanvasBoardSnapshot()
  snapshot.imageNodes.push({
    id: 1,
    type: 'image-node',
    x: 0,
    y: 0,
    width: 280,
    sourceImageUrl: 'https://example.com/image.png',
    sourceImageRatio: 1,
    outputStatus: 'ready',
    outputImages: [],
  })
  snapshot.promptNodes.push({
    id: 1,
    type: 'prompt',
    x: 320,
    y: 0,
    width: 280,
    text: '',
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  })
  snapshot.connections.push({
    id: 3,
    fromNodeType: 'image',
    fromNodeId: 1,
    fromPort: 'image',
    toNodeType: 'prompt',
    toNodeId: 1,
    toPort: 'image',
  })
  return createFlowEdgeProjectionIndex(createCanvasGraphStore(snapshot), edges)
}

describe('flowGraphDeleteProjection', () => {
  it('projects native delete summaries to drifted Flow node ids and connected edge cleanup', () => {
    const imageNode = createImageNode('node-image-1', 'task-native')
    const promptNode = createPromptNode()
    const generatedEdge = {
      id: 'vueflow__edge-node-image-1out:image-prompt-1in:image',
      source: imageNode.id,
      target: promptNode.id,
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
      data: undefined,
    } as Edge
    const { graphAdapter, applyGraphChanges } = createGraphAdapter()
    const executionLease = { nodeId: imageNode.id } as CanvasExecutionLease
    const captureActiveNodeExecutions = vi.fn(() => [executionLease])
    const releaseNodeExecutions = vi.fn()

    applyFlowGraphDeleteProjection({
      graphAdapter,
      nodes: [imageNode, promptNode],
      edges: [generatedEdge],
      commitPolicy: {
        summary: createDeleteSummary({
          removedNodeIds: ['image-1'],
          removedEdgeIds: ['connection-3'],
        }),
      },
      flowNodeIds: [imageNode.id],
      projectionIndex: createProjectionIndex([generatedEdge]),
      captureActiveNodeExecutions,
      releaseNodeExecutions,
    })

    const projectedChanges = applyGraphChanges.mock.calls[0]?.[0]
    expect(projectedChanges?.nodes).toEqual([{ id: 'node-image-1', type: 'remove' }])
    expect(projectedChanges?.edges).toEqual([
      expect.objectContaining({
        id: generatedEdge.id,
        type: 'remove',
        source: imageNode.id,
        target: promptNode.id,
      }),
    ])
    expect(captureActiveNodeExecutions).toHaveBeenCalledWith(['node-image-1'])
    expect(releaseNodeExecutions).toHaveBeenCalledWith([executionLease])
    expect(captureActiveNodeExecutions.mock.invocationCallOrder[0]).toBeLessThan(
      applyGraphChanges.mock.invocationCallOrder[0]!,
    )
    expect(applyGraphChanges.mock.invocationCallOrder[0]).toBeLessThan(
      releaseNodeExecutions.mock.invocationCallOrder[0]!,
    )
  })

  it('projects group delete summaries through the same connected-edge and task side effects', () => {
    const imageNode = createImageNode('image-1', 'task-group')
    const promptNode = createPromptNode()
    const edge = {
      id: 'connection-3',
      source: imageNode.id,
      target: promptNode.id,
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    } as Edge
    const { graphAdapter, applyGraphChanges } = createGraphAdapter()
    const executionLease = { nodeId: imageNode.id } as CanvasExecutionLease
    const captureActiveNodeExecutions = vi.fn(() => [executionLease])
    const releaseNodeExecutions = vi.fn()

    applyFlowGraphDeleteProjection({
      graphAdapter,
      nodes: [imageNode, promptNode],
      edges: [edge],
      commitPolicy: {
        summary: createDeleteSummary({
          reason: 'group-delete',
          removedNodeIds: ['image-1', 'prompt-1'],
          removedEdgeIds: ['connection-3'],
        }),
      },
      projectionIndex: createProjectionIndex([edge]),
      captureActiveNodeExecutions,
      releaseNodeExecutions,
    })

    const projectedChanges = applyGraphChanges.mock.calls[0]?.[0]
    expect(projectedChanges?.nodes).toEqual([
      { id: 'image-1', type: 'remove' },
      { id: 'prompt-1', type: 'remove' },
    ])
    expect(projectedChanges?.edges).toEqual([
      expect.objectContaining({
        id: 'connection-3',
        type: 'remove',
        source: imageNode.id,
        target: promptNode.id,
      }),
    ])
    expect(captureActiveNodeExecutions).toHaveBeenCalledWith(['image-1', 'prompt-1'])
    expect(releaseNodeExecutions).toHaveBeenCalledWith([executionLease])
    expect(captureActiveNodeExecutions.mock.invocationCallOrder[0]).toBeLessThan(
      applyGraphChanges.mock.invocationCallOrder[0]!,
    )
    expect(applyGraphChanges.mock.invocationCallOrder[0]).toBeLessThan(
      releaseNodeExecutions.mock.invocationCallOrder[0]!,
    )
  })

  it('removes active output slots with their source and preserves completed images', () => {
    const sourceNode = createPromptNode()
    const firstSlot = createImageNode('image-2', 'task-shared')
    const secondSlot = createImageNode('image-3', 'task-shared')
    const completedImage = createImageNode('image-4')
    const edges = [firstSlot, secondSlot, completedImage].map(
      (slot, index) =>
        ({
          id: `pending-edge-${index}`,
          source: sourceNode.id,
          sourceHandle: 'out:image',
          target: slot.id,
          targetHandle: 'in:image',
        }) as Edge,
    )
    const { graphAdapter, applyGraphChanges } = createGraphAdapter()
    const leases = [
      { nodeId: firstSlot.id } as CanvasExecutionLease,
      { nodeId: secondSlot.id } as CanvasExecutionLease,
    ]
    const captureActiveNodeExecutions = vi.fn(() => leases)
    const releaseNodeExecutions = vi.fn()

    applyFlowGraphDeleteProjection({
      graphAdapter,
      nodes: [sourceNode, firstSlot, secondSlot, completedImage],
      edges,
      commitPolicy: {
        summary: createDeleteSummary({ removedNodeIds: ['prompt-1'] }),
      },
      projectionIndex: createProjectionIndex(edges),
      captureActiveNodeExecutions,
      releaseNodeExecutions,
    })

    expect(applyGraphChanges.mock.calls[0]?.[0].nodes).toEqual([
      { id: 'prompt-1', type: 'remove' },
      { id: 'image-2', type: 'remove' },
      { id: 'image-3', type: 'remove' },
    ])
    expect(captureActiveNodeExecutions).toHaveBeenCalledWith([
      'prompt-1',
      'image-2',
      'image-3',
      'image-4',
    ])
    expect(releaseNodeExecutions).toHaveBeenCalledWith(leases)
  })
})
