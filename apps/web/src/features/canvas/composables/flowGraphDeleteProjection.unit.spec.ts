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
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'

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
        updated: [],
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
    const onNativeRemove = vi.fn()
    const stopDeletedRunningTasks = vi.fn()

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
      onNativeRemove,
      stopDeletedRunningTasks,
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
    expect(onNativeRemove).toHaveBeenCalledOnce()
    expect(stopDeletedRunningTasks).toHaveBeenCalledWith(['task-native'])
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
    const onNativeRemove = vi.fn()
    const stopDeletedRunningTasks = vi.fn()

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
      onNativeRemove,
      stopDeletedRunningTasks,
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
    expect(onNativeRemove).toHaveBeenCalledOnce()
    expect(stopDeletedRunningTasks).toHaveBeenCalledWith(['task-group'])
  })
})
