import * as v from 'valibot'
import {
  boardStateEnvelopeContractSchema,
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  canvasBoardSnapshotSchema,
  parseBoardStateEnvelopeContract,
  type BoardStateEnvelopeContract,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
} from '../domain'

export type BoardStateSerializationTarget =
  | 'local-state'
  | 'local-preview-fallback'
  | 'session-snapshot'
  | 'remote-envelope'

export type BoardStateSerializationResult<Target extends BoardStateSerializationTarget> =
  Target extends 'session-snapshot'
    ? CanvasBoardSnapshot
    : Target extends 'remote-envelope'
      ? BoardStateEnvelopeContract
      : CanvasBoardState

export function serializeCanvasBoardState<Target extends BoardStateSerializationTarget>(
  state: CanvasBoardState,
  target: Target,
): BoardStateSerializationResult<Target> {
  switch (target) {
    case 'local-preview-fallback':
      return stripPreviewSnapshot(state) as BoardStateSerializationResult<Target>
    case 'session-snapshot':
      return toSessionSnapshot(state) as BoardStateSerializationResult<Target>
    case 'remote-envelope':
      return toRemoteEnvelope(state) as BoardStateSerializationResult<Target>
    case 'local-state':
    default:
      return normalizeLocalState(state) as BoardStateSerializationResult<Target>
  }
}

export function deserializeRemoteBoardStateEnvelope(envelope: unknown): CanvasBoardState | null {
  if (!envelope || typeof envelope !== 'object') return null
  return parseBoardStateEnvelopeContract(envelope)
}

export function normalizeCanvasBoardState(stateLike: unknown): CanvasBoardState | null {
  const parsed = v.safeParse(boardStateEnvelopeContractSchema, stateLike)
  return parsed.success ? parsed.output : null
}

function normalizeLocalState(state: CanvasBoardState): CanvasBoardState {
  return v.parse(boardStateEnvelopeContractSchema, state)
}

function toRemoteEnvelope(state: CanvasBoardState): BoardStateEnvelopeContract {
  return v.parse(boardStateEnvelopeContractSchema, state)
}

function toSessionSnapshot(state: CanvasBoardState): CanvasBoardSnapshot {
  return v.parse(canvasBoardSnapshotSchema, {
    schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
    imageNodes: state.imageNodes,
    promptNodes: state.promptNodes,
    fusionNodes: state.fusionNodes,
    connections: state.connections,
    mediaRefs: state.mediaRefs,
  })
}

function stripPreviewSnapshot(state: CanvasBoardState): CanvasBoardState {
  return v.parse(boardStateEnvelopeContractSchema, {
    schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
    imageNodes: state.imageNodes,
    promptNodes: state.promptNodes,
    fusionNodes: state.fusionNodes,
    connections: state.connections,
    mediaRefs: state.mediaRefs,
    updatedAt: state.updatedAt,
  })
}
