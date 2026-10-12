import * as v from 'valibot'
import type { BoardSnapshot, BoardState } from './nodes'
import { boardSnapshotSchema, boardStateSchema } from '../schema/stateSchema'

export type BoardStateValidationIssue = {
  path: string
  message: string
  expected?: string | undefined
  received?: string | undefined
}

export type BoardStateValidationErrorContext = {
  code: 'INVALID_BOARD_STATE'
  boundary: string
  isRecoverable: true
  issues: BoardStateValidationIssue[]
}

export type LatestBoardStateValidationResult<T> =
  { isOk: true; value: T } | { isOk: false; error: BoardStateValidationErrorContext }

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
  readonly isRecoverable = true
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
    isRecoverable: true,
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
    isRecoverable: true,
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
    return { isOk: true, value: JSON.stringify(value) ?? 'null' }
  } catch (error) {
    return { isOk: false, error: toSerializationErrorContext(error, boundary) }
  }
}

function serializableEquality(
  left: unknown,
  right: unknown,
  boundary: string,
): LatestBoardStateValidationResult<boolean> {
  const leftJson = serializeJson(left, boundary)
  if (!leftJson.isOk) return leftJson
  const rightJson = serializeJson(right, boundary)
  if (!rightJson.isOk) return rightJson
  return { isOk: true, value: leftJson.value === rightJson.value }
}

function successResult<T>(
  input: unknown,
  output: T,
  options: LatestBoardStateNormalizationOptions | undefined,
  boundary: string,
): T | null | LatestBoardStateValidationResult<T> {
  const equality = serializableEquality(input, output, boundary)
  if (!equality.isOk) return options?.mode === 'result' ? equality : null
  const value = equality.value ? (input as T) : output
  return options?.mode === 'result' ? { isOk: true, value } : value
}

function normalizeWithLatestSchema<T>(
  schema: v.GenericSchema<unknown, T>,
  stateLike: unknown,
  options: LatestBoardStateNormalizationOptions | undefined,
): T | null | LatestBoardStateValidationResult<T> {
  const boundary = options?.boundary ?? DEFAULT_VALIDATION_BOUNDARY
  const directParsed = v.safeParse(schema, stateLike)
  if (directParsed.success) {
    return successResult(stateLike, directParsed.output, options, boundary)
  }
  const error = toValidationErrorContext(directParsed.issues, boundary)
  return options?.mode === 'result' ? { isOk: false, error } : null
}

function normalizeLatestBoardSnapshot(snapshotLike: unknown): BoardSnapshot | null
function normalizeLatestBoardSnapshot(
  snapshotLike: unknown,
  options: LatestBoardStateNullableOptions,
): BoardSnapshot | null
function normalizeLatestBoardSnapshot(
  snapshotLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<BoardSnapshot>
function normalizeLatestBoardSnapshot(
  snapshotLike: unknown,
  options?: LatestBoardStateNormalizationOptions,
): BoardSnapshot | null | LatestBoardStateValidationResult<BoardSnapshot> {
  return normalizeWithLatestSchema(boardSnapshotSchema, snapshotLike, options)
}

export function normalizeLatestBoardState(stateLike: unknown): BoardState | null
export function normalizeLatestBoardState(
  stateLike: unknown,
  options: LatestBoardStateNullableOptions,
): BoardState | null
export function normalizeLatestBoardState(
  stateLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<BoardState>
export function normalizeLatestBoardState(
  stateLike: unknown,
  options?: LatestBoardStateNormalizationOptions,
): BoardState | null | LatestBoardStateValidationResult<BoardState> {
  return normalizeWithLatestSchema(boardStateSchema, stateLike, options)
}

export function assertLatestBoardSnapshot(
  snapshotLike: unknown,
  boundary = DEFAULT_VALIDATION_BOUNDARY,
): BoardSnapshot {
  const result = normalizeLatestBoardSnapshot(snapshotLike, { mode: 'result', boundary })
  if (!result.isOk) throw new InvalidBoardStateError(result.error)
  return result.value
}

export function assertLatestBoardState(
  stateLike: unknown,
  boundary = DEFAULT_VALIDATION_BOUNDARY,
): BoardState {
  const result = normalizeLatestBoardState(stateLike, { mode: 'result', boundary })
  if (!result.isOk) throw new InvalidBoardStateError(result.error)
  return result.value
}
