import { assertLatestBoardSnapshot, assertLatestBoardState, normalizeLatestBoardState, type LatestBoardStateNullableOptions, type LatestBoardStateResultOptions, type LatestBoardStateValidationResult } from '../../core/document/latestBoardState'
import { BOARD_STATE_SCHEMA_VERSION, type BoardSnapshot, type BoardState } from '../../core/document/nodes'
import { type BoardStateEnvelopeContract } from '../../core/schema/boardStateEnvelopeContract'

export type BoardStateSerializationTarget =
  'local-state' | 'local-preview-fallback' | 'session-snapshot' | 'remote-envelope'

export type BoardStateSerializationResult<Target extends BoardStateSerializationTarget> =
  Target extends 'session-snapshot'
    ? BoardSnapshot
    : Target extends 'remote-envelope'
      ? BoardStateEnvelopeContract
      : BoardState

export function serializeBoardState<Target extends BoardStateSerializationTarget>(
  state: BoardState,
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

export function deserializeRemoteBoardStateEnvelope(envelope: unknown): BoardState | null {
  if (!envelope || typeof envelope !== 'object') return null
  return normalizeLatestBoardState(envelope)
}

export function normalizeBoardState(stateLike: unknown): BoardState | null
export function normalizeBoardState(
  stateLike: unknown,
  options: LatestBoardStateNullableOptions,
): BoardState | null
export function normalizeBoardState(
  stateLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<BoardState>
export function normalizeBoardState(
  stateLike: unknown,
  options?: LatestBoardStateNullableOptions | LatestBoardStateResultOptions,
): BoardState | null | LatestBoardStateValidationResult<BoardState> {
  if (options?.mode === 'result') {
    return normalizeLatestBoardState(stateLike, options)
  }
  return options
    ? normalizeLatestBoardState(stateLike, options)
    : normalizeLatestBoardState(stateLike)
}

function normalizeLocalState(state: BoardState): BoardState {
  return assertLatestBoardState(state, 'local-state')
}

function toRemoteEnvelope(state: BoardState): BoardStateEnvelopeContract {
  return assertLatestBoardState(state, 'remote-envelope')
}

function toSessionSnapshot(state: BoardState): BoardSnapshot {
  const normalized = assertLatestBoardState(state, 'session-snapshot')
  return assertLatestBoardSnapshot(
    {
      schemaVersion: BOARD_STATE_SCHEMA_VERSION,
      imageNodes: normalized.imageNodes,
      promptNodes: normalized.promptNodes,
      fusionNodes: normalized.fusionNodes,
      connections: normalized.connections,
      groups: normalized.groups,
      mediaRefs: normalized.mediaRefs,
    },
    'session-snapshot',
  )
}

function stripPreviewSnapshot(state: BoardState): BoardState {
  const normalized = assertLatestBoardState(state, 'local-preview-fallback')
  return assertLatestBoardState(
    {
      schemaVersion: BOARD_STATE_SCHEMA_VERSION,
      imageNodes: normalized.imageNodes,
      promptNodes: normalized.promptNodes,
      fusionNodes: normalized.fusionNodes,
      connections: normalized.connections,
      groups: normalized.groups,
      mediaRefs: normalized.mediaRefs,
      updatedAt: normalized.updatedAt,
    },
    'local-preview-fallback',
  )
}
