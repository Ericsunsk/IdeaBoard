import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  assertLatestCanvasBoardSnapshot,
  assertLatestCanvasBoardState,
  normalizeLatestCanvasBoardState,
  type LatestBoardStateNullableOptions,
  type LatestBoardStateResultOptions,
  type LatestBoardStateValidationResult,
  type BoardStateEnvelopeContract,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
} from '../domain'

export type BoardStateSerializationTarget =
  'local-state' | 'local-preview-fallback' | 'session-snapshot' | 'remote-envelope'

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
  return normalizeLatestCanvasBoardState(envelope)
}

export function normalizeCanvasBoardState(stateLike: unknown): CanvasBoardState | null
export function normalizeCanvasBoardState(
  stateLike: unknown,
  options: LatestBoardStateNullableOptions,
): CanvasBoardState | null
export function normalizeCanvasBoardState(
  stateLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<CanvasBoardState>
export function normalizeCanvasBoardState(
  stateLike: unknown,
  options?: LatestBoardStateNullableOptions | LatestBoardStateResultOptions,
): CanvasBoardState | null | LatestBoardStateValidationResult<CanvasBoardState> {
  if (options?.mode === 'result') {
    return normalizeLatestCanvasBoardState(stateLike, options)
  }
  return options
    ? normalizeLatestCanvasBoardState(stateLike, options)
    : normalizeLatestCanvasBoardState(stateLike)
}

function normalizeLocalState(state: CanvasBoardState): CanvasBoardState {
  return assertLatestCanvasBoardState(state, 'local-state')
}

function toRemoteEnvelope(state: CanvasBoardState): BoardStateEnvelopeContract {
  return assertLatestCanvasBoardState(state, 'remote-envelope')
}

function toSessionSnapshot(state: CanvasBoardState): CanvasBoardSnapshot {
  const normalized = assertLatestCanvasBoardState(state, 'session-snapshot')
  return assertLatestCanvasBoardSnapshot(
    {
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
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

function stripPreviewSnapshot(state: CanvasBoardState): CanvasBoardState {
  const normalized = assertLatestCanvasBoardState(state, 'local-preview-fallback')
  return assertLatestCanvasBoardState(
    {
      schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION,
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
