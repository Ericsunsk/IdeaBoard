import { ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { createEmptyCanvasBoardSnapshot } from '../../../canvas-board/domain'
import { readFlowGraphCommandSideEffects } from '../../composables/flowGraphAdapterCommandSideEffects'
import { CanvasGraphStore } from '../../graph-runtime'
import { usePendingImageResultExecution } from './usePendingImageResultNodes'

function imageNode(id: number, x: number, sourceImageUrl = 'source') {
  return {
    id: `image-${id}`,
    position: { x, y: 20 },
    data: {
      kind: 'image',
      snapshotNode: {
        id,
        type: 'image-node',
        x,
        y: 20,
        width: 280,
        sourceImageUrl,
        sourceImageRatio: 1,
        outputStatus: 'ready',
        outputImages: [],
      },
    },
  }
}

function createHarness(activeNodeIds: readonly string[] = []) {
  const nodes = ref<Node[]>([imageNode(1, 10) as Node])
  const edges = ref<Edge[]>([])
  const replaceGraph = vi.fn((graph) => {
    nodes.value = graph.nodes
    edges.value = graph.edges
  })
  const addEdges = vi.fn((next) => {
    edges.value = [...edges.value, ...next]
  })
  const updateNode = vi.fn((nodeId, update) => {
    nodes.value = nodes.value.map((node) =>
      node.id === nodeId ? { ...node, ...update, data: update.data ?? node.data } : node,
    )
  })
  const actions = usePendingImageResultExecution({
    nodes,
    edges,
    graphAdapter: { replaceGraph, addEdges, updateNode, removeNodes: vi.fn() } as never,
    createCurrentCanvasGraphStore: () => CanvasGraphStore.fromSnapshot(createEmptyCanvasBoardSnapshot()),
    isNodeExecutionActive: (nodeId) => activeNodeIds.includes(nodeId),
  })
  return { actions, addEdges, edges, nodes, replaceGraph, updateNode }
}

const source = { url: 'source', ratio: 1 }

describe('usePendingImageResultExecution reservation projection', () => {
  it('reserves a nearby derived image slot as runtime graph state without semantic history', () => {
    const harness = createHarness()
    const reservation = harness.actions.reservePendingResults({
      source,
      sourceNodeId: 'image-1',
      outputCount: 1,
    })

    expect(reservation.resultNodeIds).toHaveLength(1)
    expect(harness.nodes.value).toHaveLength(2)
    expect(harness.nodes.value[1].position.x).toBeGreaterThan(harness.nodes.value[0].position.x)
    const commandOptions = harness.replaceGraph.mock.calls[0]?.[1]
    expect(readFlowGraphCommandSideEffects(commandOptions.metadata)).toMatchObject({
      recordHistory: false,
    })
  })

  it('reuses the first inactive empty image child before appending a slot', () => {
    const harness = createHarness()
    harness.nodes.value.push(imageNode(2, 390, ''))
    harness.edges.value.push({
      id: 'edge-1', source: 'image-1', sourceHandle: 'out:image', target: 'image-2', targetHandle: 'in:image',
    })

    const reservation = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })

    expect(reservation.resultNodeIds).toEqual(['image-2'])
    expect(harness.nodes.value).toHaveLength(2)
    expect(harness.nodes.value.find((node) => node.id === 'image-2')?.data.snapshotNode.outputStatus).toBe('running')
  })

  it('does not reuse an active empty child slot', () => {
    const harness = createHarness(['image-2'])
    harness.nodes.value.push(imageNode(2, 390, ''))
    harness.edges.value.push({
      id: 'edge-1', source: 'image-1', sourceHandle: 'out:image', target: 'image-2', targetHandle: 'in:image',
    })

    const reservation = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })

    expect(reservation.resultNodeIds[0]).not.toBe('image-2')
    expect(harness.nodes.value).toHaveLength(3)
  })

  it('places a new result below an occupied preferred column instead of colliding', () => {
    const harness = createHarness()
    harness.nodes.value.push({
      id: 'note-2',
      position: { x: 390, y: 20 },
      dimensions: { width: 280, height: 180 },
      data: { kind: 'note', snapshotNode: { width: 280, height: 180 } },
    })

    const reservation = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })
    const resultNode = harness.nodes.value.find((node) => node.id === reservation.resultNodeIds[0])

    expect(resultNode.position).toMatchObject({ x: 390 })
    expect(resultNode.position.y).toBeGreaterThan(20)
  })

  it('attaches persisted task identity without adding graph history', () => {
    const harness = createHarness()
    const { resultNodeIds } = harness.actions.reservePendingResults({ source, sourceNodeId: 'image-1', outputCount: 1 })

    harness.actions.attachPendingTaskIds([
      {
        resultNodeId: resultNodeIds[0]!,
        resultIndex: 0,
        taskId: 'task-1',
        clientActionId: 'canvas:action-1',
      },
    ])

    const node = harness.nodes.value.find((item) => item.id === resultNodeIds[0])
    expect(node.data.snapshotNode).toMatchObject({
      pendingTaskId: 'task-1',
      pendingResultIndex: 0,
      pendingClientActionId: 'canvas:action-1',
    })
    const commandOptions = harness.updateNode.mock.calls[0]?.[2]
    expect(readFlowGraphCommandSideEffects(commandOptions.metadata)).toMatchObject({
      recordHistory: false,
    })
  })
})
