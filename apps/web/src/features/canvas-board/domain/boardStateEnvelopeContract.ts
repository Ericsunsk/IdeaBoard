import * as v from 'valibot'
import type { CanvasBoardState } from './canvasNodes'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'
import { normalizeLatestCanvasBoardState } from './latestBoardState'

export type BoardStateEnvelopeContract = CanvasBoardState

export const boardStateEnvelopeContractSchema = canvasBoardStateSchema satisfies v.GenericSchema<
  unknown,
  BoardStateEnvelopeContract
>

export function parseBoardStateEnvelopeContract(
  envelope: unknown,
): BoardStateEnvelopeContract | null {
  return normalizeLatestCanvasBoardState(envelope, { boundary: 'remote-envelope-contract' })
}
