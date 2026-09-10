import type { BoardState } from '../document/nodes'
import { normalizeLatestBoardState } from '../document/latestBoardState'

export type BoardStateEnvelopeContract = BoardState

export function parseBoardStateEnvelopeContract(
  envelope: unknown,
): BoardStateEnvelopeContract | null {
  return normalizeLatestBoardState(envelope, { boundary: 'remote-envelope-contract' })
}
