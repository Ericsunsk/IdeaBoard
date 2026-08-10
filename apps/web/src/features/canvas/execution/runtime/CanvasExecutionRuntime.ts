import type { Edge } from '@vue-flow/core'
import { shallowRef, type Ref } from 'vue'

export type CanvasExecutionStatus = 'submitting' | 'running' | 'succeeded' | 'failed' | 'cancelled'

export type CanvasNodeExecutionState = Readonly<{
  status: CanvasExecutionStatus
  taskId?: string
}>

export type CanvasExecutionRuntime = {
  readonly nodeStates: Readonly<Ref<ReadonlyMap<string, CanvasNodeExecutionState>>>
  getNodeState: (nodeId: string) => CanvasNodeExecutionState | null
  transitionNode: (nodeId: string, state: CanvasNodeExecutionState) => CanvasNodeExecutionState
  clearNode: (nodeId: string) => void
  clear: () => void
}

export type CanvasExecutionEdgeProjection = {
  edges: Edge[]
  didChange: boolean
}

const ACTIVE_EXECUTION_STATUSES: ReadonlySet<CanvasExecutionStatus> = new Set([
  'submitting',
  'running',
])

function normalizeNodeId(nodeId: string): string {
  const normalizedNodeId = nodeId.trim()
  if (!normalizedNodeId) throw new Error('Canvas execution nodeId is required')
  return normalizedNodeId
}

function normalizeNodeState(state: CanvasNodeExecutionState): CanvasNodeExecutionState {
  const taskId = state.taskId?.trim()
  return taskId ? { status: state.status, taskId } : { status: state.status }
}

function isSameNodeState(
  current: CanvasNodeExecutionState | undefined,
  next: CanvasNodeExecutionState,
): boolean {
  return current?.status === next.status && current.taskId === next.taskId
}

function isExecutionActive(state: CanvasNodeExecutionState | undefined): boolean {
  return !!state && ACTIVE_EXECUTION_STATUSES.has(state.status)
}

export function createCanvasExecutionRuntime(): CanvasExecutionRuntime {
  const nodeStates = shallowRef<ReadonlyMap<string, CanvasNodeExecutionState>>(new Map())

  function getNodeState(nodeId: string): CanvasNodeExecutionState | null {
    const normalizedNodeId = nodeId.trim()
    if (!normalizedNodeId) return null
    return nodeStates.value.get(normalizedNodeId) ?? null
  }

  function transitionNode(
    nodeId: string,
    state: CanvasNodeExecutionState,
  ): CanvasNodeExecutionState {
    const normalizedNodeId = normalizeNodeId(nodeId)
    const nextState = normalizeNodeState(state)
    if (isSameNodeState(nodeStates.value.get(normalizedNodeId), nextState)) return nextState

    const nextNodeStates = new Map(nodeStates.value)
    nextNodeStates.set(normalizedNodeId, nextState)
    nodeStates.value = nextNodeStates
    return nextState
  }

  function clearNode(nodeId: string): void {
    const normalizedNodeId = nodeId.trim()
    if (!normalizedNodeId || !nodeStates.value.has(normalizedNodeId)) return
    const nextNodeStates = new Map(nodeStates.value)
    nextNodeStates.delete(normalizedNodeId)
    nodeStates.value = nextNodeStates
  }

  function clear(): void {
    if (nodeStates.value.size === 0) return
    nodeStates.value = new Map()
  }

  return {
    nodeStates,
    getNodeState,
    transitionNode,
    clearNode,
    clear,
  }
}

export function projectCanvasExecutionEdgeAnimation(
  edges: Edge[],
  nodeStates: ReadonlyMap<string, CanvasNodeExecutionState>,
): CanvasExecutionEdgeProjection {
  let didChange = false
  const nextEdges = edges.map((edge) => {
    if (edge.targetHandle !== 'in:image') return edge
    const shouldAnimate = isExecutionActive(nodeStates.get(edge.target))
    if (edge.animated === shouldAnimate) return edge
    didChange = true
    return { ...edge, animated: shouldAnimate }
  })
  return { edges: didChange ? nextEdges : edges, didChange }
}
