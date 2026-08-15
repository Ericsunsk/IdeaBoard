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

export type CanvasPendingResultLifecycle = {
  begin: (nodeId: string) => CanvasExecutionLease | null
  adopt: (nodeId: string, taskId: string) => CanvasExecutionLease | null
  isCurrent: (nodeId: string, taskId?: string) => boolean
  transition: (
    nodeId: string,
    status: Exclude<CanvasExecutionStatus, 'submitting'>,
    taskId?: string,
  ) => boolean
  transitionAll: (
    nodeIds: readonly string[],
    status: Exclude<CanvasExecutionStatus, 'submitting' | 'running'>,
  ) => boolean
  leaseFor: (nodeId: string) => CanvasExecutionLease | null
}

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
  releaseNodeExecutions: (leases: readonly CanvasExecutionLease[]) => void
  cancelNodeExecutions: (leases: readonly CanvasExecutionLease[]) => string[]
}

export type CanvasExecutionEdgeProjection = {
  edges: Edge[]
  didChange: boolean
}

export function createCanvasPendingResultLifecycle(
  runtime: CanvasExecutionRuntime,
  attachment: CanvasExecutionAttachment,
): CanvasPendingResultLifecycle {
  const leasesByNodeId = new Map<string, CanvasExecutionLease>()

  function begin(nodeId: string): CanvasExecutionLease | null {
    const lease = runtime.beginSubmission(attachment, nodeId)
    if (lease) leasesByNodeId.set(lease.nodeId, lease)
    return lease
  }

  function adopt(nodeId: string, taskId: string): CanvasExecutionLease | null {
    const normalizedTaskId = taskId.trim()
    if (!normalizedTaskId) return null
    const lease = runtime.getNodeLease(nodeId)
    const state = runtime.getNodeState(nodeId)
    if (
      !lease ||
      lease.attachment !== attachment ||
      !runtime.isNodeActive(nodeId) ||
      state?.taskId !== normalizedTaskId
    ) {
      return null
    }
    leasesByNodeId.set(lease.nodeId, lease)
    return lease
  }

  function leaseFor(nodeId: string): CanvasExecutionLease | null {
    return leasesByNodeId.get(nodeId.trim()) ?? null
  }

  function isCurrent(nodeId: string, taskId?: string): boolean {
    const lease = leaseFor(nodeId)
    if (
      !lease ||
      !runtime.isAttachmentCurrent(attachment) ||
      runtime.getNodeLease(lease.nodeId) !== lease ||
      !runtime.isNodeActive(lease.nodeId)
    ) {
      return false
    }
    const currentTaskId = runtime.getNodeState(lease.nodeId)?.taskId
    return taskId === undefined || currentTaskId === taskId
  }

  function transition(
    nodeId: string,
    status: Exclude<CanvasExecutionStatus, 'submitting'>,
    taskId?: string,
  ): boolean {
    const lease = leaseFor(nodeId)
    if (!lease) return false
    const currentTaskId = runtime.getNodeState(lease.nodeId)?.taskId
    if (!isCurrent(nodeId) || (currentTaskId && taskId && currentTaskId !== taskId)) return false
    return runtime.transitionNode(lease, {
      status,
      taskId: taskId ?? currentTaskId,
    })
  }

  function transitionAll(
    nodeIds: readonly string[],
    status: Exclude<CanvasExecutionStatus, 'submitting' | 'running'>,
  ): boolean {
    let didTransition = false
    for (const nodeId of nodeIds) {
      didTransition = transition(nodeId, status) || didTransition
    }
    return didTransition
  }

  return { begin, adopt, isCurrent, transition, transitionAll, leaseFor }
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

  function releaseNodeExecutions(leases: readonly CanvasExecutionLease[]): void {
    const nextNodeStates = new Map(nodeStates.value)
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
      nextNodeStates.delete(lease.nodeId)
      leasesByNodeId.delete(lease.nodeId)
      didChange = true
    })
    if (didChange) nodeStates.value = nextNodeStates
  }

  function cancelNodeExecutions(leases: readonly CanvasExecutionLease[]): string[] {
    if (leases.length === 0) return []
    const nextNodeStates = new Map(nodeStates.value)
    const cancelledTaskIds = new Set<string>()
    let didChange = false
    const uniqueLeases = new Set(leases)
    uniqueLeases.forEach((lease) => {
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
      if (current?.taskId) cancelledTaskIds.add(current.taskId)
      didChange = true
    })
    if (didChange) nodeStates.value = nextNodeStates

    // A shared Task can own multiple result nodes. Cancelling one slot must not
    // cancel the provider Task while another slot with the same task is active.
    const remainingActiveTaskIds = new Set<string>()
    nextNodeStates.forEach((state) => {
      if (isExecutionActive(state) && state.taskId) remainingActiveTaskIds.add(state.taskId)
    })
    return [...cancelledTaskIds].filter((taskId) => !remainingActiveTaskIds.has(taskId))
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
    releaseNodeExecutions,
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
