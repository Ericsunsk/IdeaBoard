import * as v from 'valibot'
import {
  CANVAS_BOARD_STATE_SCHEMA_VERSION,
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  resolveWorkflowNodeOutputStatus,
  type CanvasBoardSnapshot,
  type CanvasBoardState,
  type WorkflowNodeOutputStatus,
} from './canvasNodes'
import { canvasBoardSnapshotSchema, canvasBoardStateSchema } from './canvasBoardStateSchema'
import { DEFAULT_FUSION_BLEND_POINT } from './fusionBlend'
import { DEFAULT_PROMPT_ASPECT_RATIO } from './promptAspectRatio'

export type BoardStateValidationIssue = {
  path: string
  message: string
  expected?: string
  received?: string
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

type RecordLike = Record<string, unknown>

type IssueLike = {
  message: string
  path?: readonly { key?: unknown }[]
  expected?: unknown
  received?: unknown
}

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

function isRecord(value: unknown): value is RecordLike {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function hasOwn(record: RecordLike, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(record, key)
}

function withDefault<T>(record: RecordLike, key: string, value: T): unknown {
  return hasOwn(record, key) && record[key] !== undefined ? record[key] : value
}

function withArrayDefault(record: RecordLike, key: string): unknown {
  return withDefault(record, key, [])
}

function canonicalSchemaVersion(value: unknown): unknown {
  if (value === undefined) return CANVAS_BOARD_STATE_SCHEMA_VERSION
  if (
    typeof value === 'number' &&
    Number.isInteger(value) &&
    value >= 0 &&
    value < CANVAS_BOARD_STATE_SCHEMA_VERSION
  ) {
    return CANVAS_BOARD_STATE_SCHEMA_VERSION
  }
  return value
}

function canonicalOutputImages(value: unknown): unknown {
  if (value === undefined) return []
  if (!Array.isArray(value)) return value
  return value.map((item) => {
    if (!isRecord(item)) return item
    return {
      ...item,
      ratio: withDefault(item, 'ratio', 1),
    }
  })
}

function canonicalOutputStatus(
  record: RecordLike,
  outputImages: unknown,
): WorkflowNodeOutputStatus | unknown {
  if (hasOwn(record, 'outputStatus') && record.outputStatus !== undefined) {
    return record.outputStatus
  }
  return resolveWorkflowNodeOutputStatus(
    undefined,
    Array.isArray(outputImages) ? outputImages : undefined,
  )
}

function canonicalPromptNode(node: unknown): unknown {
  if (!isRecord(node)) return node
  return {
    ...node,
    text: withDefault(node, 'text', ''),
    outputCount: withDefault(node, 'outputCount', DEFAULT_PROMPT_NODE_OUTPUT_COUNT),
    aspectRatio: withDefault(node, 'aspectRatio', DEFAULT_PROMPT_ASPECT_RATIO),
    inputImages: withArrayDefault(node, 'inputImages'),
  }
}

function canonicalImageNode(node: unknown): unknown {
  if (!isRecord(node)) return node
  const outputImages = canonicalOutputImages(node.outputImages)
  return {
    ...node,
    sourceImageUrl: withDefault(node, 'sourceImageUrl', ''),
    sourceImageRatio: withDefault(node, 'sourceImageRatio', 1),
    outputCount: withDefault(node, 'outputCount', DEFAULT_IMAGE_NODE_OUTPUT_COUNT),
    outputStatus: canonicalOutputStatus(node, outputImages),
    outputImages,
  }
}

function canonicalFusionBlendPoint(value: unknown): unknown {
  if (value === undefined) {
    return { ...DEFAULT_FUSION_BLEND_POINT }
  }
  return value
}

function canonicalFusionNode(node: unknown): unknown {
  if (!isRecord(node)) return node
  const outputImages = canonicalOutputImages(node.outputImages)
  return {
    ...node,
    prompt: withDefault(node, 'prompt', ''),
    outputCount: withDefault(node, 'outputCount', DEFAULT_FUSION_NODE_OUTPUT_COUNT),
    aspectRatio: withDefault(node, 'aspectRatio', DEFAULT_PROMPT_ASPECT_RATIO),
    blendPoint: canonicalFusionBlendPoint(node.blendPoint),
    inputImages: withArrayDefault(node, 'inputImages'),
    outputStatus: canonicalOutputStatus(node, outputImages),
    outputImages,
  }
}

function canonicalNodeArray(value: unknown, normalizeNode: (node: unknown) => unknown): unknown {
  if (!Array.isArray(value)) return value
  return value.map(normalizeNode)
}

function canonicalOptionalNonEmptyString(record: RecordLike, key: string): string | undefined {
  if (!hasOwn(record, key) || record[key] === undefined) return undefined
  return typeof record[key] === 'string' && record[key].length > 0 ? record[key] : undefined
}

function canonicalOptionalFiniteNumber(record: RecordLike, key: string): number | undefined {
  if (!hasOwn(record, key) || record[key] === undefined) return undefined
  return typeof record[key] === 'number' && Number.isFinite(record[key]) ? record[key] : undefined
}

function canonicalOptionalPositiveInteger(record: RecordLike, key: string): number | undefined {
  const value = canonicalOptionalFiniteNumber(record, key)
  return typeof value === 'number' && Number.isInteger(value) && value >= 1 ? value : undefined
}

function canonicalBoardSnapshotLike(snapshotLike: unknown, includeStateMetadata: boolean): unknown {
  if (!isRecord(snapshotLike)) return snapshotLike
  const canonical: RecordLike = {
    ...snapshotLike,
    schemaVersion: canonicalSchemaVersion(snapshotLike.schemaVersion),
    imageNodes: canonicalNodeArray(
      withArrayDefault(snapshotLike, 'imageNodes'),
      canonicalImageNode,
    ),
    promptNodes: canonicalNodeArray(
      withArrayDefault(snapshotLike, 'promptNodes'),
      canonicalPromptNode,
    ),
    fusionNodes: canonicalNodeArray(
      withArrayDefault(snapshotLike, 'fusionNodes'),
      canonicalFusionNode,
    ),
    connections: withArrayDefault(snapshotLike, 'connections'),
  }
  if (includeStateMetadata) {
    const previewSnapshotDataUrl = canonicalOptionalNonEmptyString(
      snapshotLike,
      'previewSnapshotDataUrl',
    )
    const previewSnapshotUpdatedAt = canonicalOptionalFiniteNumber(
      snapshotLike,
      'previewSnapshotUpdatedAt',
    )
    const previewSnapshotVersion = canonicalOptionalPositiveInteger(
      snapshotLike,
      'previewSnapshotVersion',
    )
    if (previewSnapshotDataUrl) canonical.previewSnapshotDataUrl = previewSnapshotDataUrl
    else delete canonical.previewSnapshotDataUrl
    if (previewSnapshotUpdatedAt !== undefined) {
      canonical.previewSnapshotUpdatedAt = previewSnapshotUpdatedAt
    } else {
      delete canonical.previewSnapshotUpdatedAt
    }
    if (previewSnapshotVersion !== undefined)
      canonical.previewSnapshotVersion = previewSnapshotVersion
    else delete canonical.previewSnapshotVersion
  }
  return canonical
}

function issuePath(issue: IssueLike): string {
  return (issue.path ?? [])
    .map((item) => item.key)
    .filter((key) => key !== undefined && key !== null && key !== '')
    .map(String)
    .join('.')
}

function issueText(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

function toValidationIssue(issue: IssueLike): BoardStateValidationIssue {
  return {
    path: issuePath(issue),
    message: issue.message,
    expected: issueText(issue.expected),
    received: issueText(issue.received),
  }
}

function toValidationErrorContext(
  issues: readonly IssueLike[],
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

function serializeJson(
  value: unknown,
  boundary: string,
): LatestBoardStateValidationResult<string> {
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

function normalizeWithLatestSchema<T>(
  schema: v.GenericSchema<unknown, T>,
  stateLike: unknown,
  includeStateMetadata: boolean,
  options: LatestBoardStateNormalizationOptions | undefined,
): T | null | LatestBoardStateValidationResult<T> {
  const boundary = options?.boundary ?? DEFAULT_VALIDATION_BOUNDARY
  const directParsed = v.safeParse(schema, stateLike)
  if (directParsed.success) {
    return successResult(stateLike, directParsed.output, options, boundary)
  }
  const parsed = v.safeParse(schema, canonicalBoardSnapshotLike(stateLike, includeStateMetadata))
  if (parsed.success) {
    return successResult(stateLike, parsed.output, options, boundary)
  }
  const error = toValidationErrorContext(parsed.issues, boundary)
  return options?.mode === 'result' ? { ok: false, error } : null
}

export function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
): CanvasBoardSnapshot | null
export function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  options: LatestBoardStateNullableOptions,
): CanvasBoardSnapshot | null
export function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  options: LatestBoardStateResultOptions,
): LatestBoardStateValidationResult<CanvasBoardSnapshot>
export function normalizeLatestCanvasBoardSnapshot(
  snapshotLike: unknown,
  options?: LatestBoardStateNormalizationOptions,
): CanvasBoardSnapshot | null | LatestBoardStateValidationResult<CanvasBoardSnapshot> {
  return normalizeWithLatestSchema(canvasBoardSnapshotSchema, snapshotLike, false, options)
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
  return normalizeWithLatestSchema(canvasBoardStateSchema, stateLike, true, options)
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
