import type {
  PendingImageExecutionFact,
  PendingImageExecutionLease,
  PendingImageExecutionReleaseOutcome,
  PendingImageExecutionReleaseResult,
} from './pendingImageExecutionTypes'

export type PendingImageExecutionReleaseContext = Readonly<{
  getLeaseRejection: (lease: PendingImageExecutionLease) => 'stale-execution' | 'wrong-board' | null
  getFact: (
    resultNodeId: PendingImageExecutionLease['resultNodeId'],
  ) => PendingImageExecutionFact | undefined
  setFact: (lease: PendingImageExecutionLease, fact: PendingImageExecutionFact) => void
  removeLease: (lease: PendingImageExecutionLease) => void
  activeTaskIds: () => ReadonlySet<string>
  createFact: (
    lease: PendingImageExecutionLease,
    status: 'cancelled',
    current: PendingImageExecutionFact | undefined,
  ) => PendingImageExecutionFact
  publish: () => void
}>

type ReleaseManyOptions = Readonly<{
  leases: readonly PendingImageExecutionLease[]
  mode: 'cancelled' | 'released'
  context: PendingImageExecutionReleaseContext
}>

function freezeList<T>(items: readonly T[]): readonly T[] {
  return Object.freeze([...items])
}

export function releasePendingImageExecutions({
  leases,
  mode,
  context,
}: ReleaseManyOptions): PendingImageExecutionReleaseResult {
  const taskIdsBefore = new Set<string>()
  const outcomes: PendingImageExecutionReleaseOutcome[] = []
  const seenLeases = new Set<PendingImageExecutionLease>()
  let didChange = false

  for (const lease of leases) {
    if (seenLeases.has(lease)) continue
    seenLeases.add(lease)
    const reason = context.getLeaseRejection(lease)
    if (reason) {
      outcomes.push({ resultNodeId: lease.resultNodeId, kind: 'rejected', reason })
      continue
    }
    const current = context.getFact(lease.resultNodeId)
    if (current?.taskId) taskIdsBefore.add(current.taskId)
    if (mode === 'cancelled') {
      context.setFact(lease, context.createFact(lease, 'cancelled', current))
    } else {
      context.removeLease(lease)
    }
    outcomes.push({ resultNodeId: lease.resultNodeId, kind: mode })
    didChange = true
  }

  const remainingTaskIds = context.activeTaskIds()
  const cancellableTaskIds = [...taskIdsBefore].filter((taskId) => !remainingTaskIds.has(taskId))
  if (didChange) context.publish()
  return Object.freeze({
    outcomes: freezeList(outcomes.map((outcome) => Object.freeze(outcome))),
    cancellableTaskIds: freezeList(cancellableTaskIds),
  })
}
