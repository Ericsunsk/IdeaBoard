import {
  bindCanvasBoardGraphSession,
  type CanvasBoardGraphSession,
  type PendingImageDomainMutationPort,
  type PendingImageDomainMutationResult,
  type PendingImageResolvedResult,
  type PendingImageResultNodeId,
} from './CanvasBoardGraphSession'
import type { CanvasBoardAttachment, CanvasBoardSessionIdentity } from '../../board-attachment'
import { createPendingImageExecutionPublisher } from './pendingImageExecutionPublisher'
import {
  assertPendingImageMutationContract,
  mapPendingImageMaterializationOutcomes,
  preparePendingImageMaterialization,
  type PendingImageMaterializationSlot,
} from './pendingImageMaterialization'
import {
  releasePendingImageExecutions,
  type PendingImageExecutionReleaseContext,
} from './pendingImageExecutionRelease'
import {
  attachmentRejection,
  isAttachmentCurrent,
  leaseRejection,
} from './pendingImageExecutionValidation'

export type {
  PendingImageExecutionStatus,
  PendingImageExecutionFact,
  PendingImageExecutionLease,
  PendingImageHydrationIdentity,
  PendingImageExecutionSnapshot,
  PendingImageExecutionRejectionReason,
  PendingImageExecutionTransition,
  PendingImageSupersededExecution,
  PendingImageExecutionStartResult,
  PendingImageExecutionReleaseOutcome,
  PendingImageExecutionReleaseResult,
  PendingImageMaterializationOutcome,
  PendingImageMaterializationResult,
  PendingImageExecutionCoreOptions,
  PendingImageExecutionCore,
} from './pendingImageExecutionTypes'

import type {
  PendingImageExecutionStatus,
  PendingImageExecutionFact,
  PendingImageExecutionLease,
  PendingImageHydrationIdentity,
  PendingImageExecutionTransition,
  PendingImageExecutionStartResult,
  PendingImageMaterializationOutcome,
  PendingImageMaterializationResult,
  PendingImageExecutionCoreOptions,
  PendingImageExecutionCore,
} from './pendingImageExecutionTypes'

export class PendingImageMaterializationError extends Error {
  readonly outcomes: readonly PendingImageMaterializationOutcome[]

  constructor(cause: unknown, outcomes: readonly PendingImageMaterializationOutcome[]) {
    super('Pending image result materialization failed', { cause })
    this.name = 'PendingImageMaterializationError'
    this.outcomes = outcomes
  }
}

export { PendingImageMutationContractError } from './pendingImageMaterialization'

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
  const checkAttachmentCurrent = (candidate: CanvasBoardAttachment): boolean =>
    isAttachmentCurrent(candidate, attachment, sessionIdentity)
  const checkAttachmentRejection = (candidate: CanvasBoardAttachment) =>
    attachmentRejection(candidate, attachment, sessionIdentity)
  const checkLeaseRejection = (lease: PendingImageExecutionLease) =>
    leaseRejection(lease, attachment, sessionIdentity, leasesByNodeId, factsByNodeId, isActive)

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
    const attachmentReason = checkAttachmentRejection(targetAttachment)
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
    if (checkAttachmentRejection(targetAttachment)) return null
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
      checkLeaseRejection(lease) ||
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
      return lease && !checkLeaseRejection(lease) ? [lease] : []
    })
  }

  function isCurrent(lease: PendingImageExecutionLease, taskId?: string): boolean {
    if (checkLeaseRejection(lease)) return false
    return taskId === undefined || factsByNodeId.get(lease.resultNodeId)?.taskId === taskId.trim()
  }

  function applyFact(
    lease: PendingImageExecutionLease,
    fact: PendingImageExecutionFact,
    shouldPublish = true,
  ): PendingImageExecutionTransition {
    const reason = checkLeaseRejection(lease)
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
    const reason = checkLeaseRejection(lease)
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

  const releaseContext: PendingImageExecutionReleaseContext = {
    getLeaseRejection: checkLeaseRejection,
    getFact: (resultNodeId) => factsByNodeId.get(resultNodeId),
    setFact: (lease, fact) => {
      factsByNodeId = new Map(factsByNodeId).set(lease.resultNodeId, fact)
    },
    removeLease: (lease) => {
      factsByNodeId = new Map(factsByNodeId)
      factsByNodeId.delete(lease.resultNodeId)
      leasesByNodeId.delete(lease.resultNodeId)
    },
    activeTaskIds,
    createFact: (lease, status, current) =>
      freezeFact(lease.resultNodeId, status, current?.taskId, current?.clientActionId),
    publish: publisher.publish,
  }

  function materializeBatch(
    slots: readonly Readonly<{
      lease: PendingImageExecutionLease
      result: PendingImageResolvedResult
    }>[],
  ): PendingImageMaterializationResult {
    const { candidates, outcomes } = preparePendingImageMaterialization(
      slots as readonly PendingImageMaterializationSlot[],
      {
        getLeaseRejection: checkLeaseRejection,
        getTaskId: (resultNodeId) => factsByNodeId.get(resultNodeId)?.taskId,
      },
    )

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

    let mutationResult: PendingImageDomainMutationResult
    try {
      mutationResult = mutationPort.materializeBatch(candidates.map(({ input }) => input))
      assertPendingImageMutationContract(candidates, mutationResult)
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

    mapPendingImageMaterializationOutcomes(
      candidates,
      mutationResult,
      outcomes as Array<PendingImageMaterializationOutcome | undefined>,
      {
        getFact: (resultNodeId) => factsByNodeId.get(resultNodeId),
        applyFact: (lease, fact) => {
          applyFact(lease, fact, false)
        },
        createFact: (lease, status, current) =>
          freezeFact(lease.resultNodeId, status, current?.taskId, current?.clientActionId),
      },
    )
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
    isAttachmentCurrent: checkAttachmentCurrent,
    begin,
    hydratePendingLease,
    claimHydratedLease,
    markSubmitted,
    fail,
    cancel: (leases) =>
      releasePendingImageExecutions({ leases, mode: 'cancelled', context: releaseContext }),
    release: (leases) =>
      releasePendingImageExecutions({ leases, mode: 'released', context: releaseContext }),
    materializeBatch,
    isCurrent,
    factFor,
    captureActiveLeases,
  }
}
