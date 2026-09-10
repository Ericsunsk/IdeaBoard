import type {
  BoardGraphSession,
  PendingImageMutationRejectReason,
  PendingImageResolvedResult,
  PendingImageResultNodeId,
} from './boardGraphSession'
import type { BoardAttachment } from '../../board-attachment'

declare const pendingImageExecutionLeaseBrand: unique symbol

export type PendingImageExecutionStatus =
  'submitting' | 'running' | 'succeeded' | 'failed' | 'cancelled'

export type PendingImageExecutionFact = Readonly<{
  resultNodeId: PendingImageResultNodeId
  status: PendingImageExecutionStatus
  taskId?: string
  clientActionId?: string
}>

export type PendingImageExecutionLease = Readonly<{
  attachment: BoardAttachment
  resultNodeId: PendingImageResultNodeId
  sequence: number
  [pendingImageExecutionLeaseBrand]: true
}>

export type PendingImageMaterializationSlot = Readonly<{
  lease: PendingImageExecutionLease
  result: PendingImageResolvedResult
}>

export type PendingImageHydrationIdentity = Readonly<{
  resultNodeId: PendingImageResultNodeId
  taskId: string
  clientActionId: string
}>

export type PendingImageExecutionSnapshot = Readonly<{
  version: number
  attachment: Readonly<{ boardId: string; epoch: number }> | null
  executions: readonly PendingImageExecutionFact[]
}>

export type PendingImageExecutionRejectionReason =
  'stale-execution' | 'wrong-board' | 'task-mismatch'

export type PendingImageExecutionTransition =
  | Readonly<{ kind: 'applied'; fact: PendingImageExecutionFact }>
  | Readonly<{ kind: 'rejected'; reason: PendingImageExecutionRejectionReason }>

export type PendingImageSupersededExecution = Readonly<{
  resultNodeId: PendingImageResultNodeId
  sequence: number
  status: 'submitting' | 'running'
  taskId?: string
}>

export type PendingImageExecutionStartResult =
  | Readonly<{
      kind: 'started'
      lease: PendingImageExecutionLease
      superseded?: PendingImageSupersededExecution
      cancellableTaskIds: readonly string[]
    }>
  | Readonly<{ kind: 'rejected'; reason: 'stale-attachment' | 'wrong-board' }>

export type PendingImageExecutionReleaseOutcome =
  | Readonly<{
      resultNodeId: PendingImageResultNodeId
      kind: 'cancelled' | 'released'
    }>
  | Readonly<{
      resultNodeId: PendingImageResultNodeId
      kind: 'rejected'
      reason: 'stale-execution' | 'wrong-board'
    }>

export type PendingImageExecutionReleaseResult = Readonly<{
  outcomes: readonly PendingImageExecutionReleaseOutcome[]
  cancellableTaskIds: readonly string[]
}>

export type PendingImageMaterializationOutcome =
  | Readonly<{ resultNodeId: PendingImageResultNodeId; kind: 'materialized' }>
  | Readonly<{
      resultNodeId: PendingImageResultNodeId
      kind: 'rejected'
      reason:
        'stale-execution' | 'wrong-board' | 'duplicate-slot' | PendingImageMutationRejectReason
    }>
  | Readonly<{
      resultNodeId: PendingImageResultNodeId
      kind: 'failed'
      reason: 'mutation-error'
    }>

export type PendingImageMaterializationResult = Readonly<{
  didChange: boolean
  changedNodeIds: readonly PendingImageResultNodeId[]
  outcomes: readonly PendingImageMaterializationOutcome[]
}>

export type PendingImageExecutionCoreOptions = Readonly<{
  onSubscriberError?: (error: unknown) => void
}>

export type PendingImageExecutionCore = {
  snapshot: () => PendingImageExecutionSnapshot
  subscribe: (listener: (snapshot: PendingImageExecutionSnapshot) => void) => () => void
  attachBoard: (session: BoardGraphSession) => BoardAttachment
  detachBoard: () => void
  currentAttachment: () => BoardAttachment | null
  isAttachmentCurrent: (attachment: BoardAttachment) => boolean
  begin: (
    attachment: BoardAttachment,
    resultNodeId: PendingImageResultNodeId,
  ) => PendingImageExecutionStartResult
  hydratePendingLease: (
    attachment: BoardAttachment,
    pending: PendingImageHydrationIdentity,
  ) => PendingImageExecutionStartResult
  claimHydratedLease: (
    attachment: BoardAttachment,
    pending: PendingImageHydrationIdentity,
  ) => PendingImageExecutionLease | null
  markSubmitted: (
    lease: PendingImageExecutionLease,
    taskId: string,
    clientActionId: string,
  ) => PendingImageExecutionTransition
  fail: (lease: PendingImageExecutionLease) => PendingImageExecutionTransition
  cancel: (leases: readonly PendingImageExecutionLease[]) => PendingImageExecutionReleaseResult
  release: (leases: readonly PendingImageExecutionLease[]) => PendingImageExecutionReleaseResult
  materializeBatch: (
    slots: readonly PendingImageMaterializationSlot[],
  ) => PendingImageMaterializationResult
  isCurrent: (lease: PendingImageExecutionLease, taskId?: string) => boolean
  factFor: (resultNodeId: PendingImageResultNodeId) => PendingImageExecutionFact | null
  captureActiveLeases: (resultNodeIds: readonly string[]) => PendingImageExecutionLease[]
}
