import type { CanvasBoardState } from './canvasNodes'
import { normalizeLatestCanvasBoardState } from './latestBoardState'

export type BoardStateEnvelopeContract = CanvasBoardState

export function parseBoardStateEnvelopeContract(
  envelope: unknown,
): BoardStateEnvelopeContract | null {
  return normalizeLatestCanvasBoardState(envelope, { boundary: 'remote-envelope-contract' })
}
