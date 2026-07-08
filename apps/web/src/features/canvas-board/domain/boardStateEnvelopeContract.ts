import * as v from 'valibot'
import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  type BoardStateMediaRef,
  type CanvasBoardState,
} from './canvasNodes'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'
import { normalizeLatestCanvasBoardState } from './latestBoardState'

export type BoardStateEnvelopeContract = CanvasBoardState

const finiteNumberSchema = v.pipe(v.number(), v.finite())
const nonEmptyStringSchema = v.pipe(v.string(), v.minLength(1))
const positiveIntegerSchema = v.pipe(finiteNumberSchema, v.integer(), v.minValue(1))

export const boardStateMediaRefEnvelopeSchema = v.strictObject({
  mediaAssetId: positiveIntegerSchema,
  refType: v.picklist(['board', 'board_node']),
  refId: v.pipe(nonEmptyStringSchema, v.maxLength(128)),
  role: v.optional(nonEmptyStringSchema),
}) satisfies v.GenericSchema<unknown, BoardStateMediaRef>

export const boardStateEnvelopeContractSchema = canvasBoardStateSchema satisfies v.GenericSchema<
  unknown,
  BoardStateEnvelopeContract
>

export const boardStateEnvelopeMetadataSchema = v.strictObject({
  schemaVersion: v.literal(CANVAS_BOARD_STATE_SCHEMA_VERSION),
  previewSnapshotDataUrl: v.optional(nonEmptyStringSchema),
  previewSnapshotUpdatedAt: v.optional(finiteNumberSchema),
  previewSnapshotVersion: v.optional(positiveIntegerSchema),
  updatedAt: finiteNumberSchema,
})

export function parseBoardStateEnvelopeContract(
  envelope: unknown,
): BoardStateEnvelopeContract | null {
  return normalizeLatestCanvasBoardState(envelope, { boundary: 'remote-envelope-contract' })
}

export function assertBoardStateEnvelopeContract(envelope: unknown): BoardStateEnvelopeContract {
  return v.parse(boardStateEnvelopeContractSchema, envelope)
}
