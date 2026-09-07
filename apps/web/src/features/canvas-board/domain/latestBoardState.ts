import * as v from 'valibot'
import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
} from './canvasNodes'
import { canvasBoardSnapshotSchema, canvasBoardStateSchema } from './canvasBoardStateSchema'

export type BoardStateValidationIssue = {
  path: string
  message: string
  expected?: string | undefined
  received?: string | undefined
}

export type BoardStateValidationErrorContext = {
  code: 'INVALID_BOARD_STATE'
  boundary: string
  recoverable: true
  issues: BoardStateValidationIssue[]
}

export type LatestBoardStateValidationResult<T> =
  { ok: true; value: T } | { ok: false; error: BoardStateValidationErrorContext }

export type LatestBoardStateNullableOptions = {
  mode?: 'nullable'
  boundary?: string
}

export type LatestBoardStateResultOptions = {
  mode: 'result'
  boundary?: string
}

export type LatestBoardStateNormalizationOptions =
  LatestBoardStateNullableOptions | LatestBoardStateResultOptions

const DEFAULT_VALIDATION_BOUNDARY = 'board-state'

export class InvalidBoardStateError extends Error {
  readonly code = 'INVALID_BOARD_STATE'
  readonly boundary: string
  readonly recoverable = true
  readonly issues: BoardStateValidationIssue[]

  constructor(context: BoardStateValidationErrorContext) {
    super(`${context.code}:${context.boundary}`)
    this.name = 'InvalidBoardStateError'
    this.boundary = context.boundary
    this.issues = context.issues
  }
}

function issuePath(issue: v.BaseIssue<unknown>): string {
  return (issue.path ?? [])
    .map((item) => item.key)
    .filter((key) => key !== undefined && key !== null && key !== '')
    .map(String)
    .join('.')
}

function issueText(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

function toValidationIssue(issue: v.BaseIssue<unknown>): BoardStateValidationIssue {
  return {
    path: issuePath(issue),
    message: issue.message,
    expected: issueText(issue.expected),
    received: issueText(issue.received),
  }
}

function toValidationErrorContext(
  issues: readonly v.BaseIssue<unknown>[],
  boundary: string,
): BoardStateValidationErrorContext {
  return {
    code: 'INVALID_BOARD_STATE',
    boundary,
    recoverable: true,
    issues: issues.map(toValidationIssue),
  }
}

function toSerializationErrorContext(
  error: unknown,
  boundary: string,
): BoardStateValidationErrorContext {
  return {
    code: 'INVALID_BOARD_STATE',
    boundary,
    recoverable: true,
    issues: [
      {
        path: '$',
        message: 'BoardState must be JSON serializable',
        received: error instanceof Error ? error.message : String(error),
      },
    ],
  }
}

function serializeJson(value: unknown, boundary: string): LatestBoardStateValidationResult<string> {
  try {
    return { ok: true, value: JSON.stringify(value) ?? 'null' }
  } catch (error) {
    return { ok: false, error: toSerializationErrorContext(error, boundary) }
  }
}

function serializableEquality(
  left: unknown,
  right: unknown,
  boundary: string,
): LatestBoardStateValidationResult<boolean> {
  const leftJson = serializeJson(left, boundary)
  if (!leftJson.ok) return leftJson
  const rightJson = serializeJson(right, boundary)
  if (!rightJson.ok) return rightJson
  return { ok: true, value: leftJson.value === rightJson.value }
}

function successResult<T>(
  input: unknown,
  output: T,
  options: LatestBoardStateNormalizationOptions | undefined,
  boundary: string,
): T | null | LatestBoardStateValidationResult<T> {
  const equality = serializableEquality(input, output, boundary)
  if (!equality.ok) return options?.mode === 'result' ? equality : null
  const value = equality.value ? (input as T) : output
  return options?.mode === 'result' ? { ok: true, value } : value
}

function isMigrationRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function migrateV3ImageNode(node: unknown): unknown {
  // Legacy v3 key — intentionally not renamed.
  if (!isMigrationRecord(node) || !('adjustmentRecipe' in node)) return node
  const { adjustmentRecipe, ...rest } = node
  if (adjustmentRecipe === undefined) return rest
  return { ...rest, retouchRecipe: adjustmentRecipe }
}

/**
 * v3 → v4: `imageNodes[].adjustmentRecipe` renamed to `retouchRecipe`.
 * Only v3 payloads are rewritten; anything else passes through to strict
 * validation unchanged.
 */
const PRE_RETOUCH_RECIPE_SCHEMA_VERSION = 3

function migrateLegacyBoardState(stateLike: unknown): unknown {
  if (!isMigrationRecord(stateLike)) return stateLike
  if (stateLike.schemaVersion !== PRE_RETOUCH_RECIPE_SCHEMA_VERSION) return stateLike
  const imageNodes = Array.isArray(stateLike.imageNodes)
    ? stateLike.imageNodes.map(migrateV3ImageNode)
    : stateLike.imageNodes
  return { ...stateLike, schemaVersion: CANVAS_BOARD_STATE_SCHEMA_VERSION, imageNodes }
}

function normalizeWithLatestSchema<T>(
  schema: v.GenericSchema<unknown, T>,
  stateLike: unknown,
  options: LatestBoardStateNormalizationOptions | undefined,
): T | null | LatestBoardStateValidationResult<T> {
  const boundary = options?.boundary ?? DEFAULT_VALIDATION_BOUNDARY
  const migrated = migrateLegacyBoardState(stateLike)
  const directParsed = v.safeParse(schema, migrated)
  if (directParsed.success) {
    return successResult(migrated, directParsed.output, options, boundary)
  }
  const error = toValidationErrorContext(directParsed.issues, boundary)
  return options?.mode === 'result' ? { ok: false, error } : null
}

function normalizeLatestCanvasBoardSnapshot(snapshotLike: unknown): CanvasBoardSnapshot | null
function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  options: LatestBoardStateNullableOptions,
): CanvasBoardSnapshot | null
function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<CanvasBoardSnapshot>
function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  options?: LatestBoardStateNormalizationOptions,
): CanvasBoardSnapshot | null | LatestBoardStateValidationResult<CanvasBoardSnapshot> {
  return normalizeWithLatestSchema(canvasBoardSnapshotSchema, snapshotLike, options)
}

export function normalizeLatestCanvasBoardState(stateLike: unknown): CanvasBoardState | null
export function normalizeLatestCanvasBoardState(
  stateLike: unknown,
  options: LatestBoardStateNullableOptions,
): CanvasBoardState | null
export function normalizeLatestCanvasBoardState(
  stateLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<CanvasBoardState>
export function normalizeLatestCanvasBoardState(
  stateLike: unknown,
  options?: LatestBoardStateNormalizationOptions,
): CanvasBoardState | null | LatestBoardStateValidationResult<CanvasBoardState> {
  return normalizeWithLatestSchema(canvasBoardStateSchema, stateLike, options)
}

export function assertLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  boundary = DEFAULT_VALIDATION_BOUNDARY,
): CanvasBoardSnapshot {
  const result = normalizeLatestCanvasBoardSnapshot(snapshotLike, { mode: 'result', boundary })
  if (!result.ok) throw new InvalidBoardStateError(result.error)
  return result.value
}

export function assertLatestCanvasBoardState(
  stateLike: unknown,
  boundary = DEFAULT_VALIDATION_BOUNDARY,
): CanvasBoardState {
  const result = normalizeLatestCanvasBoardState(stateLike, { mode: 'result', boundary })
  if (!result.ok) throw new InvalidBoardStateError(result.error)
  return result.value
}
