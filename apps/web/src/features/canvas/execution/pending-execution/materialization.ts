import type {
  PendingImageDomainMutationInput,
  PendingImageDomainMutationOutcome,
  PendingImageDomainMutationResult,
} from './boardGraphSession'
import type {
  PendingImageExecutionFact,
  PendingImageExecutionLease,
  PendingImageMaterializationOutcome,
  PendingImageMaterializationSlot,
} from './types'

export type { PendingImageMaterializationSlot } from './types'

export class PendingImageMutationContractError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'PendingImageMutationContractError'
  }
}

export type PendingImageMaterializationCandidate = Readonly<{
  index: number
  lease: PendingImageExecutionLease
  input: PendingImageDomainMutationInput
}>

type CandidatePreparationContext = Readonly<{
  getLeaseRejection: (lease: PendingImageExecutionLease) => 'stale-execution' | 'wrong-board' | null
  getTaskId: (resultNodeId: PendingImageExecutionLease['resultNodeId']) => string | undefined
}>

export type PendingImageMaterializationPreparation = Readonly<{
  candidates: readonly PendingImageMaterializationCandidate[]
  outcomes: (PendingImageMaterializationOutcome | undefined)[]
}>

export function preparePendingImageMaterialization(
  slots: readonly PendingImageMaterializationSlot[],
  context: CandidatePreparationContext,
): PendingImageMaterializationPreparation {
  const outcomes = new Array<PendingImageMaterializationOutcome | undefined>(slots.length)
  const candidates: PendingImageMaterializationCandidate[] = []
  const seenNodeIds = new Set<PendingImageExecutionLease['resultNodeId']>()

  slots.forEach((slot, index) => {
    const reason = context.getLeaseRejection(slot.lease)
    if (reason) {
      outcomes[index] = { resultNodeId: slot.lease.resultNodeId, kind: 'rejected', reason }
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
    const taskId = context.getTaskId(slot.lease.resultNodeId)
    if (!taskId) {
      throw new Error('Pending image execution must be submitted before materialization')
    }
    candidates.push({
      index,
      lease: slot.lease,
      input: { resultNodeId: slot.lease.resultNodeId, taskId, result: slot.result },
    })
  })

  return Object.freeze({ candidates: Object.freeze(candidates), outcomes })
}

export function assertPendingImageMutationContract(
  candidates: readonly PendingImageMaterializationCandidate[],
  result: PendingImageDomainMutationResult,
): void {
  const candidateIds = new Set(candidates.map(({ lease }) => lease.resultNodeId))
  const outcomeIds = new Set<PendingImageExecutionLease['resultNodeId']>()
  const materializedIds = new Set<PendingImageExecutionLease['resultNodeId']>()
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

type OutcomeMappingContext = Readonly<{
  getFact: (
    resultNodeId: PendingImageExecutionLease['resultNodeId'],
  ) => PendingImageExecutionFact | undefined
  applyFact: (lease: PendingImageExecutionLease, fact: PendingImageExecutionFact) => void
  createFact: (
    lease: PendingImageExecutionLease,
    status: 'succeeded' | 'cancelled',
    current: PendingImageExecutionFact | undefined,
  ) => PendingImageExecutionFact
}>

export function mapPendingImageMaterializationOutcomes(
  candidates: readonly PendingImageMaterializationCandidate[],
  mutationResult: PendingImageDomainMutationResult,
  outcomes: (PendingImageMaterializationOutcome | undefined)[],
  context: OutcomeMappingContext,
): void {
  const mutationOutcomes = new Map<string, PendingImageDomainMutationOutcome>(
    mutationResult.outcomes.map((outcome) => [outcome.resultNodeId, outcome]),
  )
  candidates.forEach(({ index, lease }) => {
    const mutationOutcome = mutationOutcomes.get(lease.resultNodeId)
    const current = context.getFact(lease.resultNodeId)
    if (mutationOutcome?.kind === 'materialized') {
      context.applyFact(lease, context.createFact(lease, 'succeeded', current))
      outcomes[index] = { resultNodeId: lease.resultNodeId, kind: 'materialized' }
      return
    }
    const reason = mutationOutcome?.reason ?? 'missing-node'
    context.applyFact(lease, context.createFact(lease, 'cancelled', current))
    outcomes[index] = { resultNodeId: lease.resultNodeId, kind: 'rejected', reason }
  })
}
