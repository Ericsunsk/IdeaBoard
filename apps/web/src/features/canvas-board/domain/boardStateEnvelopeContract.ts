import type { CanvasBoardState } from './nodes'
import { normalizeLatestCanvasBoardState } from './latestBoardState'

export type BoardStateEnvelopeContract = CanvasBoardState

export function parseBoardStateEnvelopeContract(
  envelope: unknown,
): BoardStateEnvelopeContract | null {
  return normalizeLatestCanvasBoardState(envelope, { boundary: 'remote-envelope-contract' })
}
