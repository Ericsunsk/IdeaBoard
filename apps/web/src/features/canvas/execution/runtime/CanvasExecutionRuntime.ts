import type { Edge } from '@vue-flow/core'
import { shallowRef, type Ref } from 'vue'

declare const canvasExecutionAttachmentBrand: unique symbol
declare const canvasExecutionLeaseBrand: unique symbol

export type CanvasExecutionStatus = 'submitting' | 'running' | 'succeeded' | 'failed' | 'cancelled'

export type CanvasNodeExecutionState = Readonly<{
  status: CanvasExecutionStatus
  taskId?: string
}>

export type CanvasExecutionAttachment = Readonly<{
  boardId: string
  epoch: number
  [canvasExecutionAttachmentBrand]: true
}>

export type CanvasExecutionLease = Readonly<{
  attachment: CanvasExecutionAttachment
  nodeId: string
  sequence: number
  [canvasExecutionLeaseBrand]: true
}>

export type CanvasExecutionRuntime = {
  readonly attachment: Readonly<Ref<CanvasExecutionAttachment | null>>
  readonly nodeStates: Readonly<Ref<ReadonlyMap<string, CanvasNodeExecutionState>>>
  attachBoard: (boardId: string) => CanvasExecutionAttachment
  detachBoard: () => void
  isAttachmentCurrent: (attachment: CanvasExecutionAttachment) => boolean
  beginSubmission: (
    attachment: CanvasExecutionAttachment,
    nodeId: string,
  ) => CanvasExecutionLease | null
  hydratePending: (
    attachment: CanvasExecutionAttachment,
    nodeId: string,
    taskId: string,
  ) => CanvasExecutionLease | null
  transitionNode: (lease: CanvasExecutionLease, state: CanvasNodeExecutionState) => boolean
  getNodeState: (nodeId: string) => CanvasNodeExecutionState | null
  getNodeLease: (nodeId: string) => CanvasExecutionLease | null
  isNodeActive: (nodeId: string) => boolean
  captureActiveNodeExecutions: (nodeIds: readonly string[]) => CanvasExecutionLease[]
  cancelNodeExecutions: (leases: readonly CanvasExecutionLease[]) => string[]
}

export type CanvasExecutionEdgeProjection = {
  edges: Edge[]
  didChange: boolean
}

const ACTIVE_EXECUTION_STATUSES: ReadonlySet<CanvasExecutionStatus> = new Set([
  'submitting',
  'running',
])

function normalizeIdentity(value: string, label: 'boardId' | 'nodeId'): string {
  const normalized = value.trim()
  if (!normalized) throw new Error(`Canvas execution ${label} is required`)
  return normalized
}

function normalizeNodeState(state: CanvasNodeExecutionState): CanvasNodeExecutionState {
  const taskId = state.taskId?.trim()
  return taskId ? { status: state.status, taskId } : { status: state.status }
}

function isExecutionActive(state: CanvasNodeExecutionState | undefined): boolean {
  return !!state && ACTIVE_EXECUTION_STATUSES.has(state.status)
}

function canTransitionExecution(
  current: CanvasNodeExecutionState,
  next: CanvasNodeExecutionState,
): boolean {
  if (current.taskId && current.taskId !== next.taskId) return false
  if (!isExecutionActive(current)) {
    return current.status === next.status && current.taskId === next.taskId
  }
  if (current.status === 'running' && next.status === 'submitting') return false
  return true
}

export function createCanvasExecutionRuntime(): CanvasExecutionRuntime {
  const attachment = shallowRef<CanvasExecutionAttachment | null>(null)
  const nodeStates = shallowRef<ReadonlyMap<string, CanvasNodeExecutionState>>(new Map())
  const leasesByNodeId = new Map<string, CanvasExecutionLease>()
  let attachmentEpoch = 0
  let leaseSequence = 0

  function resetNodeState() {
    leasesByNodeId.clear()
    if (nodeStates.value.size > 0) nodeStates.value = new Map()
  }

  function attachBoard(boardId: string): CanvasExecutionAttachment {
    const nextAttachment = {
      boardId: normalizeIdentity(boardId, 'boardId'),
      epoch: ++attachmentEpoch,
    } as CanvasExecutionAttachment
    attachment.value = nextAttachment
    resetNodeState()
    return nextAttachment
  }

  function detachBoard(): void {
    attachmentEpoch += 1
    attachment.value = null
    resetNodeState()
  }

  function isAttachmentCurrent(candidate: CanvasExecutionAttachment): boolean {
    return attachment.value === candidate
  }

  function startNode(
    targetAttachment: CanvasExecutionAttachment,
    nodeId: string,
    state: CanvasNodeExecutionState,
  ): CanvasExecutionLease | null {
    if (!isAttachmentCurrent(targetAttachment)) return null
    const normalizedNodeId = normalizeIdentity(nodeId, 'nodeId')
    const lease = {
      attachment: targetAttachment,
      nodeId: normalizedNodeId,
      sequence: ++leaseSequence,
    } as CanvasExecutionLease
    leasesByNodeId.set(normalizedNodeId, lease)
    const nextNodeStates = new Map(nodeStates.value)
    nextNodeStates.set(normalizedNodeId, normalizeNodeState(state))
    nodeStates.value = nextNodeStates
    return lease
  }

  function beginSubmission(
    targetAttachment: CanvasExecutionAttachment,
    nodeId: string,
  ): CanvasExecutionLease | null {
    return startNode(targetAttachment, nodeId, { status: 'submitting' })
  }

  function hydratePending(
    targetAttachment: CanvasExecutionAttachment,
    nodeId: string,
    taskId: string,
  ): CanvasExecutionLease | null {
    return startNode(targetAttachment, nodeId, { status: 'running', taskId })
  }

  function transitionNode(lease: CanvasExecutionLease, state: CanvasNodeExecutionState): boolean {
    if (!isAttachmentCurrent(lease.attachment) || leasesByNodeId.get(lease.nodeId) !== lease) {
      return false
    }
    const nextState = normalizeNodeState(state)
    const currentState = nodeStates.value.get(lease.nodeId)
    if (!currentState || !canTransitionExecution(currentState, nextState)) return false
    if (currentState?.status === nextState.status && currentState.taskId === nextState.taskId) {
      return true
    }
    const nextNodeStates = new Map(nodeStates.value)
    nextNodeStates.set(lease.nodeId, nextState)
    nodeStates.value = nextNodeStates
    return true
  }

  function getNodeState(nodeId: string): CanvasNodeExecutionState | null {
    const normalizedNodeId = nodeId.trim()
    return normalizedNodeId ? (nodeStates.value.get(normalizedNodeId) ?? null) : null
  }

  function getNodeLease(nodeId: string): CanvasExecutionLease | null {
    const normalizedNodeId = nodeId.trim()
    return normalizedNodeId ? (leasesByNodeId.get(normalizedNodeId) ?? null) : null
  }

  function isNodeActive(nodeId: string): boolean {
    return isExecutionActive(getNodeState(nodeId) ?? undefined)
  }

  function captureActiveNodeExecutions(nodeIds: readonly string[]): CanvasExecutionLease[] {
    const normalizedNodeIds = new Set(nodeIds.map((nodeId) => nodeId.trim()).filter(Boolean))
    return Array.from(normalizedNodeIds).flatMap((nodeId) => {
      const lease = leasesByNodeId.get(nodeId)
      const current = nodeStates.value.get(nodeId)
      return lease && isAttachmentCurrent(lease.attachment) && isExecutionActive(current)
        ? [lease]
        : []
    })
  }

  function cancelNodeExecutions(leases: readonly CanvasExecutionLease[]): string[] {
    if (leases.length === 0) return []
    const nextNodeStates = new Map(nodeStates.value)
    const taskIds = new Set<string>()
    let didChange = false
    new Set(leases).forEach((lease) => {
      const current = nextNodeStates.get(lease.nodeId)
      if (
        leasesByNodeId.get(lease.nodeId) !== lease ||
        !isAttachmentCurrent(lease.attachment) ||
        !isExecutionActive(current)
      ) {
        return
      }
      nextNodeStates.set(
        lease.nodeId,
        current?.taskId ? { status: 'cancelled', taskId: current.taskId } : { status: 'cancelled' },
      )
      if (current?.taskId) taskIds.add(current.taskId)
      didChange = true
    })
    if (didChange) nodeStates.value = nextNodeStates
    return [...taskIds]
  }

  return {
    attachment,
    nodeStates,
    attachBoard,
    detachBoard,
    isAttachmentCurrent,
    beginSubmission,
    hydratePending,
    transitionNode,
    getNodeState,
    getNodeLease,
    isNodeActive,
    captureActiveNodeExecutions,
    cancelNodeExecutions,
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
