import {
  bindCanvasBoardGraphSession,
  type CanvasBoardGraphSession,
  type PendingImageDomainMutationInput,
  type PendingImageDomainMutationPort,
  type PendingImageDomainMutationResult,
  type PendingImageMutationRejectReason,
  type PendingImageResolvedResult,
  type PendingImageResultNodeId,
} from './CanvasBoardGraphSession'
import type { CanvasBoardAttachment, CanvasBoardSessionIdentity } from '../../board-attachment'
import { createPendingImageExecutionPublisher } from './pendingImageExecutionPublisher'

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
  attachment: CanvasBoardAttachment
  resultNodeId: PendingImageResultNodeId
  sequence: number
  [pendingImageExecutionLeaseBrand]: true
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

export class PendingImageMaterializationError extends Error {
  readonly outcomes: readonly PendingImageMaterializationOutcome[]

  constructor(cause: unknown, outcomes: readonly PendingImageMaterializationOutcome[]) {
    super('Pending image result materialization failed', { cause })
    this.name = 'PendingImageMaterializationError'
    this.outcomes = outcomes
  }
}

export class PendingImageMutationContractError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'PendingImageMutationContractError'
  }
}

export type PendingImageExecutionCoreOptions = Readonly<{
  onSubscriberError?: (error: unknown) => void
}>

export type PendingImageExecutionCore = {
  snapshot: () => PendingImageExecutionSnapshot
  subscribe: (listener: (snapshot: PendingImageExecutionSnapshot) => void) => () => void
  attachBoard: (session: CanvasBoardGraphSession) => CanvasBoardAttachment
  detachBoard: () => void
  currentAttachment: () => CanvasBoardAttachment | null
  isAttachmentCurrent: (attachment: CanvasBoardAttachment) => boolean
  begin: (
    attachment: CanvasBoardAttachment,
    resultNodeId: PendingImageResultNodeId,
  ) => PendingImageExecutionStartResult
  hydratePendingLease: (
    attachment: CanvasBoardAttachment,
    pending: PendingImageHydrationIdentity,
  ) => PendingImageExecutionStartResult
  claimHydratedLease: (
    attachment: CanvasBoardAttachment,
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
    slots: readonly Readonly<{
      lease: PendingImageExecutionLease
      result: PendingImageResolvedResult
    }>[],
  ) => PendingImageMaterializationResult
  isCurrent: (lease: PendingImageExecutionLease, taskId?: string) => boolean
  factFor: (resultNodeId: PendingImageResultNodeId) => PendingImageExecutionFact | null
  captureActiveLeases: (resultNodeIds: readonly string[]) => PendingImageExecutionLease[]
}

const ACTIVE_STATUSES: ReadonlySet<PendingImageExecutionStatus> = new Set(['submitting', 'running'])

function requireIdentity(
  value: string,
  label: 'resultNodeId' | 'taskId' | 'clientActionId',
): string {
  const normalized = value.trim()
  if (!normalized) throw new Error(`Pending image execution ${label} is required`)
  return normalized
}

function isActive(
  fact: PendingImageExecutionFact | undefined,
): fact is PendingImageExecutionFact & {
  status: 'submitting' | 'running'
} {
  return !!fact && ACTIVE_STATUSES.has(fact.status)
}

function freezeFact(
  resultNodeId: PendingImageResultNodeId,
  status: PendingImageExecutionStatus,
  taskId?: string,
  clientActionId?: string,
): PendingImageExecutionFact {
  return Object.freeze({
    resultNodeId,
    status,
    ...(taskId ? { taskId } : {}),
    ...(clientActionId ? { clientActionId } : {}),
  })
}

function freezeList<T>(items: readonly T[]): readonly T[] {
  return Object.freeze([...items])
}

function reportSubscriberErrorByDefault(error: unknown): void {
  console.error('[canvas] pending image execution subscriber failed', error)
}

export function createPendingImageExecutionCore(
  options: PendingImageExecutionCoreOptions = {},
): PendingImageExecutionCore {
  let attachment: CanvasBoardAttachment | null = null
  let mutationPort: PendingImageDomainMutationPort | null = null
  let sessionIdentity: CanvasBoardSessionIdentity | null = null
  let attachmentEpoch = 0
  let leaseSequence = 0
  let factsByNodeId = new Map<PendingImageResultNodeId, PendingImageExecutionFact>()
  const leasesByNodeId = new Map<PendingImageResultNodeId, PendingImageExecutionLease>()
  const claimedHydrationLeases = new WeakSet<PendingImageExecutionLease>()
  const reportSubscriberError = options.onSubscriberError ?? reportSubscriberErrorByDefault
  const publisher = createPendingImageExecutionPublisher({
    createSnapshot: (version) => {
      const attachmentSnapshot = attachment
        ? Object.freeze({ boardId: attachment.boardId, epoch: attachment.epoch })
        : null
      return Object.freeze({
        version,
        attachment: attachmentSnapshot,
        executions: freezeList([...factsByNodeId.values()]),
      })
    },
    onSubscriberError: reportSubscriberError,
  })

  function resetExecutions(): void {
    factsByNodeId = new Map()
    leasesByNodeId.clear()
  }

  function attachBoard(session: CanvasBoardGraphSession): CanvasBoardAttachment {
    const nextEpoch = ++attachmentEpoch
    const binding = session[bindCanvasBoardGraphSession](nextEpoch)
    const nextAttachment = binding.attachment
    const nextMutationPort = binding.mutations
    if (
      nextAttachment.boardId !== session.boardId ||
      nextAttachment.epoch !== nextEpoch ||
      nextAttachment.sessionIdentity !== binding.sessionIdentity ||
      nextMutationPort.attachment !== nextAttachment ||
      nextMutationPort.sessionIdentity !== binding.sessionIdentity ||
      nextMutationPort.attachment.epoch !== nextEpoch
    ) {
      throw new Error('Pending image mutation port belongs to a different Board attachment')
    }
    attachment = nextAttachment
    mutationPort = nextMutationPort
    sessionIdentity = binding.sessionIdentity
    resetExecutions()
    publisher.publish()
    return nextAttachment
  }

  function detachBoard(): void {
    attachmentEpoch += 1
    attachment = null
    mutationPort = null
    sessionIdentity = null
    resetExecutions()
    publisher.publish()
  }

  function currentAttachment(): CanvasBoardAttachment | null {
    return attachment
  }

  function isAttachmentCurrent(candidate: CanvasBoardAttachment): boolean {
    return candidate === attachment && candidate.sessionIdentity === sessionIdentity
  }

  function attachmentRejection(
    candidate: CanvasBoardAttachment,
  ): 'stale-attachment' | 'wrong-board' | null {
    if (!attachment || candidate.boardId !== attachment.boardId) return 'wrong-board'
    return candidate === attachment && candidate.sessionIdentity === sessionIdentity
      ? null
      : 'stale-attachment'
  }

  function leaseRejection(
    lease: PendingImageExecutionLease,
  ): 'stale-execution' | 'wrong-board' | null {
    if (!attachment || lease.attachment.boardId !== attachment.boardId) return 'wrong-board'
    if (
      lease.attachment !== attachment ||
      lease.attachment.sessionIdentity !== sessionIdentity ||
      leasesByNodeId.get(lease.resultNodeId) !== lease ||
      !isActive(factsByNodeId.get(lease.resultNodeId))
    ) {
      return 'stale-execution'
    }
    return null
  }

  function activeTaskIds(): Set<string> {
    const taskIds = new Set<string>()
    factsByNodeId.forEach((fact) => {
      if (isActive(fact) && fact.taskId) taskIds.add(fact.taskId)
    })
    return taskIds
  }

  function start(
    targetAttachment: CanvasBoardAttachment,
    rawResultNodeId: PendingImageResultNodeId,
    status: 'submitting' | 'running',
    taskId?: string,
    clientActionId?: string,
  ): PendingImageExecutionStartResult {
    const attachmentReason = attachmentRejection(targetAttachment)
    if (attachmentReason) return { kind: 'rejected', reason: attachmentReason }
    const resultNodeId = requireIdentity(
      rawResultNodeId,
      'resultNodeId',
    ) as PendingImageResultNodeId
    const previousLease = leasesByNodeId.get(resultNodeId)
    const previousFact = factsByNodeId.get(resultNodeId)
    const lease = Object.freeze({
      attachment: targetAttachment,
      resultNodeId,
      sequence: ++leaseSequence,
    }) as PendingImageExecutionLease
    leasesByNodeId.set(resultNodeId, lease)
    factsByNodeId = new Map(factsByNodeId).set(
      resultNodeId,
      freezeFact(resultNodeId, status, taskId, clientActionId),
    )
    const remainingTaskIds = activeTaskIds()
    const cancellableTaskIds =
      previousFact?.taskId && !remainingTaskIds.has(previousFact.taskId)
        ? [previousFact.taskId]
        : []
    const superseded =
      previousLease && isActive(previousFact)
        ? Object.freeze({
            resultNodeId,
            sequence: previousLease.sequence,
            status: previousFact.status,
            ...(previousFact.taskId ? { taskId: previousFact.taskId } : {}),
          })
        : undefined
    publisher.publish()
    return Object.freeze({
      kind: 'started',
      lease,
      ...(superseded ? { superseded } : {}),
      cancellableTaskIds: freezeList(cancellableTaskIds),
    })
  }

  function begin(
    targetAttachment: CanvasBoardAttachment,
    resultNodeId: PendingImageResultNodeId,
  ): PendingImageExecutionStartResult {
    return start(targetAttachment, resultNodeId, 'submitting')
  }

  function hydratePendingLease(
    targetAttachment: CanvasBoardAttachment,
    pending: PendingImageHydrationIdentity,
  ): PendingImageExecutionStartResult {
    return start(
      targetAttachment,
      pending.resultNodeId,
      'running',
      requireIdentity(pending.taskId, 'taskId'),
      requireIdentity(pending.clientActionId, 'clientActionId'),
    )
  }

  function claimHydratedLease(
    targetAttachment: CanvasBoardAttachment,
    pending: PendingImageHydrationIdentity,
  ): PendingImageExecutionLease | null {
    if (attachmentRejection(targetAttachment)) return null
    const resultNodeId = requireIdentity(
      pending.resultNodeId,
      'resultNodeId',
    ) as PendingImageResultNodeId
    const taskId = requireIdentity(pending.taskId, 'taskId')
    const clientActionId = requireIdentity(pending.clientActionId, 'clientActionId')
    const lease = leasesByNodeId.get(resultNodeId)
    const fact = factsByNodeId.get(resultNodeId)
    if (
      !lease ||
      leaseRejection(lease) ||
      fact?.status !== 'running' ||
      fact.taskId !== taskId ||
      fact.clientActionId !== clientActionId
    ) {
      return null
    }
    if (claimedHydrationLeases.has(lease)) return null
    claimedHydrationLeases.add(lease)
    return lease
  }

  function factFor(resultNodeId: PendingImageResultNodeId): PendingImageExecutionFact | null {
    return factsByNodeId.get(resultNodeId) ?? null
  }

  function captureActiveLeases(resultNodeIds: readonly string[]): PendingImageExecutionLease[] {
    const uniqueNodeIds = new Set(resultNodeIds as readonly PendingImageResultNodeId[])
    return [...uniqueNodeIds].flatMap((resultNodeId) => {
      const lease = leasesByNodeId.get(resultNodeId)
      return lease && !leaseRejection(lease) ? [lease] : []
    })
  }

  function isCurrent(lease: PendingImageExecutionLease, taskId?: string): boolean {
    if (leaseRejection(lease)) return false
    return taskId === undefined || factsByNodeId.get(lease.resultNodeId)?.taskId === taskId.trim()
  }

  function applyFact(
    lease: PendingImageExecutionLease,
    fact: PendingImageExecutionFact,
    shouldPublish = true,
  ): PendingImageExecutionTransition {
    const reason = leaseRejection(lease)
    if (reason) return { kind: 'rejected', reason }
    factsByNodeId = new Map(factsByNodeId).set(lease.resultNodeId, fact)
    if (shouldPublish) publisher.publish()
    return { kind: 'applied', fact }
  }

  function markSubmitted(
    lease: PendingImageExecutionLease,
    rawTaskId: string,
    rawClientActionId: string,
  ): PendingImageExecutionTransition {
    const taskId = requireIdentity(rawTaskId, 'taskId')
    const clientActionId = requireIdentity(rawClientActionId, 'clientActionId')
    const reason = leaseRejection(lease)
    if (reason) return { kind: 'rejected', reason }
    const current = factsByNodeId.get(lease.resultNodeId)
    if (current?.taskId && current.taskId !== taskId) {
      return { kind: 'rejected', reason: 'task-mismatch' }
    }
    return applyFact(lease, freezeFact(lease.resultNodeId, 'running', taskId, clientActionId))
  }

  function fail(lease: PendingImageExecutionLease): PendingImageExecutionTransition {
    const current = factsByNodeId.get(lease.resultNodeId)
    return applyFact(
      lease,
      freezeFact(lease.resultNodeId, 'failed', current?.taskId, current?.clientActionId),
    )
  }

  function releaseMany(
    leases: readonly PendingImageExecutionLease[],
    mode: 'cancelled' | 'released',
  ): PendingImageExecutionReleaseResult {
    const taskIdsBefore = new Set<string>()
    const outcomes: PendingImageExecutionReleaseOutcome[] = []
    const seenLeases = new Set<PendingImageExecutionLease>()
    let didChange = false
    for (const lease of leases) {
      if (seenLeases.has(lease)) continue
      seenLeases.add(lease)
      const reason = leaseRejection(lease)
      if (reason) {
        outcomes.push({ resultNodeId: lease.resultNodeId, kind: 'rejected', reason })
        continue
      }
      const current = factsByNodeId.get(lease.resultNodeId)
      if (current?.taskId) taskIdsBefore.add(current.taskId)
      if (mode === 'cancelled') {
        factsByNodeId = new Map(factsByNodeId).set(
          lease.resultNodeId,
          freezeFact(lease.resultNodeId, 'cancelled', current?.taskId, current?.clientActionId),
        )
      } else {
        factsByNodeId = new Map(factsByNodeId)
        factsByNodeId.delete(lease.resultNodeId)
        leasesByNodeId.delete(lease.resultNodeId)
      }
      outcomes.push({ resultNodeId: lease.resultNodeId, kind: mode })
      didChange = true
    }
    const remainingTaskIds = activeTaskIds()
    const cancellableTaskIds = [...taskIdsBefore].filter((taskId) => !remainingTaskIds.has(taskId))
    if (didChange) publisher.publish()
    return Object.freeze({
      outcomes: freezeList(outcomes.map((outcome) => Object.freeze(outcome))),
      cancellableTaskIds: freezeList(cancellableTaskIds),
    })
  }

  function materializeBatch(
    slots: readonly Readonly<{
      lease: PendingImageExecutionLease
      result: PendingImageResolvedResult
    }>[],
  ): PendingImageMaterializationResult {
    const outcomes = new Array<PendingImageMaterializationOutcome | undefined>(slots.length)
    const candidates: Array<{
      index: number
      lease: PendingImageExecutionLease
      input: PendingImageDomainMutationInput
    }> = []
    const seenNodeIds = new Set<PendingImageResultNodeId>()

    slots.forEach((slot, index) => {
      const reason = leaseRejection(slot.lease)
      if (reason) {
        outcomes[index] = {
          resultNodeId: slot.lease.resultNodeId,
          kind: 'rejected',
          reason,
        }
        return
      }
      if (seenNodeIds.has(slot.lease.resultNodeId)) {
        outcomes[index] = {
          resultNodeId: slot.lease.resultNodeId,
          kind: 'rejected',
          reason: 'duplicate-slot',
        }
        return
      }
      seenNodeIds.add(slot.lease.resultNodeId)
      const taskId = factsByNodeId.get(slot.lease.resultNodeId)?.taskId
      if (!taskId) {
        throw new Error('Pending image execution must be submitted before materialization')
      }
      candidates.push({
        index,
        lease: slot.lease,
        input: { resultNodeId: slot.lease.resultNodeId, taskId, result: slot.result },
      })
    })

    if (candidates.length === 0) {
      return Object.freeze({
        didChange: false,
        changedNodeIds: freezeList([]),
        outcomes: freezeList(
          outcomes.filter((outcome): outcome is PendingImageMaterializationOutcome => !!outcome),
        ),
      })
    }

    if (
      !mutationPort ||
      !attachment ||
      !sessionIdentity ||
      mutationPort.attachment !== attachment ||
      mutationPort.attachment.epoch !== attachment.epoch ||
      mutationPort.sessionIdentity !== sessionIdentity ||
      attachment.sessionIdentity !== sessionIdentity
    ) {
      candidates.forEach(({ index, lease }) => {
        outcomes[index] = {
          resultNodeId: lease.resultNodeId,
          kind: 'rejected',
          reason: 'wrong-board',
        }
      })
      return Object.freeze({
        didChange: false,
        changedNodeIds: freezeList([]),
        outcomes: freezeList(
          outcomes.filter((outcome): outcome is PendingImageMaterializationOutcome => !!outcome),
        ),
      })
    }

    function assertMutationContract(result: PendingImageDomainMutationResult): void {
      const candidateIds = new Set(candidates.map(({ lease }) => lease.resultNodeId))
      const outcomeIds = new Set<PendingImageResultNodeId>()
      const materializedIds = new Set<PendingImageResultNodeId>()
      for (const outcome of result.outcomes) {
        if (!candidateIds.has(outcome.resultNodeId)) {
          throw new PendingImageMutationContractError(
            `Mutation returned an unexpected outcome for ${outcome.resultNodeId}`,
          )
        }
        if (outcomeIds.has(outcome.resultNodeId)) {
          throw new PendingImageMutationContractError(
            `Mutation returned duplicate outcomes for ${outcome.resultNodeId}`,
          )
        }
        outcomeIds.add(outcome.resultNodeId)
        if (outcome.kind === 'materialized') materializedIds.add(outcome.resultNodeId)
      }
      if (outcomeIds.size !== candidateIds.size) {
        throw new PendingImageMutationContractError(
          'Mutation must return exactly one outcome for every candidate',
        )
      }

      const changedIds = new Set(result.changedNodeIds)
      if (changedIds.size !== result.changedNodeIds.length) {
        throw new PendingImageMutationContractError('Mutation returned duplicate changed node ids')
      }
      const changedIdsMatch =
        changedIds.size === materializedIds.size &&
        [...changedIds].every((resultNodeId) => materializedIds.has(resultNodeId))
      if (!changedIdsMatch) {
        throw new PendingImageMutationContractError(
          'Mutation changed node ids must match materialized outcomes',
        )
      }
      if (result.didChange !== changedIds.size > 0) {
        throw new PendingImageMutationContractError(
          'Mutation didChange must match its changed node ids',
        )
      }
    }

    let mutationResult: PendingImageDomainMutationResult
    try {
      mutationResult = mutationPort.materializeBatch(candidates.map(({ input }) => input))
      assertMutationContract(mutationResult)
    } catch (cause) {
      candidates.forEach(({ index, lease }) => {
        const current = factsByNodeId.get(lease.resultNodeId)
        applyFact(
          lease,
          freezeFact(lease.resultNodeId, 'failed', current?.taskId, current?.clientActionId),
          false,
        )
        outcomes[index] = {
          resultNodeId: lease.resultNodeId,
          kind: 'failed',
          reason: 'mutation-error',
        }
      })
      publisher.publish()
      throw new PendingImageMaterializationError(
        cause,
        freezeList(
          outcomes.filter((outcome): outcome is PendingImageMaterializationOutcome => !!outcome),
        ),
      )
    }

    const mutationOutcomes = new Map(
      mutationResult.outcomes.map((outcome) => [outcome.resultNodeId, outcome]),
    )
    candidates.forEach(({ index, lease }) => {
      const mutationOutcome = mutationOutcomes.get(lease.resultNodeId)
      const current = factsByNodeId.get(lease.resultNodeId)
      const taskId = current?.taskId
      if (mutationOutcome?.kind === 'materialized') {
        applyFact(
          lease,
          freezeFact(lease.resultNodeId, 'succeeded', taskId, current?.clientActionId),
          false,
        )
        outcomes[index] = { resultNodeId: lease.resultNodeId, kind: 'materialized' }
        return
      }
      const reason = mutationOutcome?.reason ?? 'missing-node'
      applyFact(
        lease,
        freezeFact(lease.resultNodeId, 'cancelled', taskId, current?.clientActionId),
        false,
      )
      outcomes[index] = { resultNodeId: lease.resultNodeId, kind: 'rejected', reason }
    })
    publisher.publish()

    return Object.freeze({
      didChange: mutationResult.didChange,
      changedNodeIds: freezeList(mutationResult.changedNodeIds),
      outcomes: freezeList(
        outcomes.filter((outcome): outcome is PendingImageMaterializationOutcome => !!outcome),
      ),
    })
  }

  return {
    snapshot: publisher.snapshot,
    subscribe: publisher.subscribe,
    attachBoard,
    detachBoard,
    currentAttachment,
    isAttachmentCurrent,
    begin,
    hydratePendingLease,
    claimHydratedLease,
    markSubmitted,
    fail,
    cancel: (leases) => releaseMany(leases, 'cancelled'),
    release: (leases) => releaseMany(leases, 'released'),
    materializeBatch,
    isCurrent,
    factFor,
    captureActiveLeases,
  }
}
