import type { CanvasBoardAttachment, CanvasBoardSessionIdentity } from '../../board-attachment'
import type {
  PendingImageExecutionFact,
  PendingImageExecutionLease,
} from './types'

export function attachmentRejection(
  candidate: CanvasBoardAttachment,
  currentAttachment: CanvasBoardAttachment | null,
  currentSessionIdentity: CanvasBoardSessionIdentity | null,
): 'stale-attachment' | 'wrong-board' | null {
  if (!currentAttachment || candidate.boardId !== currentAttachment.boardId) {
    return 'wrong-board'
  }
  return candidate === currentAttachment && candidate.sessionIdentity === currentSessionIdentity
    ? null
    : 'stale-attachment'
}

export function isAttachmentCurrent(
  candidate: CanvasBoardAttachment,
  currentAttachment: CanvasBoardAttachment | null,
  currentSessionIdentity: CanvasBoardSessionIdentity | null,
): boolean {
  return candidate === currentAttachment && candidate.sessionIdentity === currentSessionIdentity
}

export function leaseRejection(
  lease: PendingImageExecutionLease,
  currentAttachment: CanvasBoardAttachment | null,
  currentSessionIdentity: CanvasBoardSessionIdentity | null,
  leasesByNodeId: ReadonlyMap<PendingImageLeaseNodeId, PendingImageExecutionLease>,
  factsByNodeId: ReadonlyMap<PendingImageLeaseNodeId, PendingImageExecutionFact>,
  isActive: (fact: PendingImageExecutionFact | undefined) => boolean,
): 'stale-execution' | 'wrong-board' | null {
  if (!currentAttachment || lease.attachment.boardId !== currentAttachment.boardId) {
    return 'wrong-board'
  }
  if (
    lease.attachment !== currentAttachment ||
    lease.attachment.sessionIdentity !== currentSessionIdentity ||
    leasesByNodeId.get(lease.resultNodeId) !== lease ||
    !isActive(factsByNodeId.get(lease.resultNodeId))
  ) {
    return 'stale-execution'
  }
  return null
}

type PendingImageLeaseNodeId = PendingImageExecutionLease['resultNodeId']
