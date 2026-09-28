// Canvas workspace task transport (W2 stream 1, Public Data Plane v1).
//
// Explicit semantic adapter between the canvas execution DTO
// (`domain/execution-contracts/canvasTaskRequest`) and the versioned
// workspace task surface (`/api/v1/workspaces/{workspace_id}/tasks*` via the
// central `shared/api/workspaceTasks` resource). Canvas pages/components
// consume this module (or the `canvasTaskSubmissionAdapter` seam above it);
// nothing in canvas imports generated query wrappers directly, and `shared`
// stays feature-free.
//
// Semantic mappings (#809 audit):
// - submit: `SubmitTaskData` prompt/feature/image inputs/params ->
//   `CreateWorkspaceTaskInput` workflow_kind/media_asset_ids/prompt
//   (+params incl. `mask_asset_id`, quantity, resolution).
//   `client_action_id` becomes the header-only `Idempotency-Key` (never
//   sent in the v1 body); the prompt rides `data.prompt` and is covered
//   by the server idempotency hash (same key + different prompt -> 409).
// - readiness: `ready/message/billing.sufficient` <- task-quotes snapshot.
//   Quotes are price-only: no wallet balances ride the snapshot, so
//   `available_credits`/`reserved_credits` map to 0 and sufficiency is
//   inferred from the absence of 402 (documented below). There is no v1
//   matrix endpoint; matrix fans out to one quote per request.
// - wait: waiter image-URL extraction <- asset-ref outputs (`output_refs`).
//   Asset refs resolve through the media browser URL; remote refs resolve
//   direct. v1 tasks carry no `execution_metrics`, so the legacy
//   finalize-failure pending detection is unavailable (failed surfaces
//   immediately; documented residual).
// - cancel: needs the workspace id, resolved per task (see registry).
//
// Fail-closed rules: anything without a faithful v1 mapping throws
// `CanvasWorkspaceTaskUnmappableError` BEFORE any network call (never a
// silent drop, never a cryptic 422). Remaining guards and their reasons:
// `prompt` (over-8KB-byte prompts), `feature` (unknown slugs), `media-urls`
// (request/param image URLs incl. mask data-URLs: bytes cannot ride params,
// upload via the session flow first), `media-ids` (numeric asset ids and
// malformed opaque refs incl. `mask_asset_id`), `params` (unknown keys incl.
// `params.prompt`, storage-URL values). Missing workspace selection throws
// `CanvasWorkspaceUnavailableError` with guidance.

import type { ApiClientError } from '../../../../../shared/api/client'
import { isOpaqueId, opaqueIdPrefix } from '../../../../../shared/api/contractKernel'
import { taskCancellationIdempotencyKey, workspaceTaskIdempotencyKey } from '../../../../../shared/api/idempotencyKey'
import {
  cancelWorkspaceTask,
  createWorkspaceTask,
  getWorkspaceTask,
  quoteWorkspaceTask,
  waitWorkspaceTaskTerminal,
  type CreateWorkspaceTaskInput,
  type WorkspaceTaskItem,
  type WorkspaceTaskOutputRef,
} from '../../../../../shared/api/workspaceTasks'
import {
  WORKFLOW_FEATURE_SLUGS,
  type WorkflowFeature,
} from '../../../../../shared/domain/workflowFeatures'
import { i18n } from '../../../../../shared/i18n'
import { resolveWorkspaceMediaAssetDerivativeURL } from '../../../../../shared/api/mediaAssetDelivery'
import { notifyUserCreditsInvalidation } from '../../../../../shared/query/userCreditsInvalidation'
import type { TaskImageResult } from '../../../../../shared/query/taskResult'
import { sanitizeApiErrorMessage } from '../../../../../shared/utils/errorMessages'
import { requireWorkspaceId as requireSharedWorkspaceId } from '../../../../../shared/utils/workspaceId'
import { getWorkspaceSelectionStore } from '../../../../workspace'
import type {
  SubmitTaskData,
  SubmitTaskReadinessData,
} from '../../../domain/execution-contracts/canvasTaskRequest'
import {
  createTaskCancelledError,
  createTaskResultFailureError,
  TaskResultPendingError,
} from '../task-runner/taskResultErrors'
import { insufficientCreditsPreflightError } from '../task-runner/canvasBillingPreflight'

export const CANVAS_WORKSPACE_REQUIRED_MESSAGE =
  'Select a workspace before running image tasks.'

export class CanvasWorkspaceUnavailableError extends Error {
  readonly kind = 'workspace-unavailable' as const

  constructor(message: string = CANVAS_WORKSPACE_REQUIRED_MESSAGE) {
    super(message)
    this.name = 'CanvasWorkspaceUnavailableError'
  }
}

export function isCanvasWorkspaceUnavailableError(
  error: unknown,
): error is CanvasWorkspaceUnavailableError {
  return error instanceof CanvasWorkspaceUnavailableError
}

export type CanvasWorkspaceTaskUnmappableReason =
  | 'prompt'
  | 'feature'
  | 'media-urls'
  | 'media-ids'
  | 'params'

export class CanvasWorkspaceTaskUnmappableError extends Error {
  readonly kind = 'task-unmappable' as const
  readonly reason: CanvasWorkspaceTaskUnmappableReason

  constructor(reason: CanvasWorkspaceTaskUnmappableReason, message: string) {
    super(message)
    this.name = 'CanvasWorkspaceTaskUnmappableError'
    this.reason = reason
  }
}

export function isCanvasWorkspaceTaskUnmappableError(
  error: unknown,
): error is CanvasWorkspaceTaskUnmappableError {
  return error instanceof CanvasWorkspaceTaskUnmappableError
}

/** Single source of truth: the `features/workspace` selection store. */
export function requireCanvasWorkspaceId(): string {
  return requireSharedWorkspaceId(
    getWorkspaceSelectionStore().getSelectedId(),
    () => new CanvasWorkspaceUnavailableError(),
  )
}

// Task -> workspace registry: cancel needs the workspace the task was
// submitted under, which may differ from the current selection after a
// workspace switch. Populated at submit; cancel falls back to the current
// selection and fails closed (warn + skip) when neither resolves.
const taskWorkspaceIds = new Map<string, string>()

export function registerCanvasWorkspaceTask(taskId: string, workspaceId: string): void {
  const task = taskId.trim()
  const workspace = workspaceId.trim()
  if (task && workspace) taskWorkspaceIds.set(task, workspace)
}

function resolveCanvasTaskWorkspaceId(taskId: string): string | null {
  return taskWorkspaceIds.get(taskId.trim()) ?? null
}

/** Test seam: drop task -> workspace bindings (specs only). */
export function resetCanvasWorkspaceTaskRegistryForTests(): void {
  taskWorkspaceIds.clear()
}

function errorStatus(error: unknown): number | undefined {
  const status = (error as { status?: unknown } | null)?.status
  return typeof status === 'number' ? status : undefined
}

function errorCode(error: unknown): unknown {
  return (error as { code?: unknown } | null)?.code
}

function isWorkspaceTaskGone(error: unknown): boolean {
  if (errorStatus(error) === 404) return true
  const code = errorCode(error)
  return code === 404 || code === 'task_not_found'
}

function isTaskStateConflict(error: unknown): boolean {
  if (errorStatus(error) === 409) return true
  return errorCode(error) === 'task_state_conflict'
}

// Submit mapping: SubmitTaskData -> CreateWorkspaceTaskInput.
//
// Prompt contract (mirrors the backend v1 `data.prompt` input,
// `workspacetasks.MaxWorkspaceTaskPromptBytes` = 8KB, legacy parity):
// - generation/inpaint require a prompt; render/variations/fusion/outpaint
//   carry an optional prompt; cutout/upscale ignore it (binary media
//   operations: user text is dropped client-side, matching the canonical
//   compile drop).
// - The client enforces ONLY the 8KB byte bound (fail-closed `prompt`
//   before any network call). Required-policy empties are NOT rejected
//   here: the server 422 (`validation_failed`) stays authoritative so the
//   client never duplicates plan-policy validation.
// - Mask contract: `params.mask_asset_id` carries an opaque `asset_...`
//   mask reference minted by the workspace upload-session flow (see
//   `../media/inpaintMask`). Shape is validated here (opaque id only;
//   data-URL/storage-URL masks fail closed with upload guidance); mask
//   policy (inpaint-only, inpaint-requires-mask) stays server-side 422.
// ---------------------------------------------------------------------------

function assertWorkflowKind(feature: unknown): WorkflowFeature {
  if (typeof feature === 'string' && (WORKFLOW_FEATURE_SLUGS as readonly string[]).includes(feature)) {
    return feature as WorkflowFeature
  }
  throw new CanvasWorkspaceTaskUnmappableError(
    'feature',
    `Unsupported canvas task feature ${JSON.stringify(feature)}: v1 workflow_kind accepts only ${WORKFLOW_FEATURE_SLUGS.join(', ')}.`,
  )
}

/** v1 prompt bound in bytes (mirrors `workspacetasks.MaxWorkspaceTaskPromptBytes`). */
const MAX_WORKSPACE_TASK_PROMPT_BYTES = 8 * 1024

const PROMPT_IGNORED_FEATURES: ReadonlySet<WorkflowFeature> = new Set(['cutout', 'upscale'])

function promptByteLength(prompt: string): number {
  return new TextEncoder().encode(prompt).length
}

function toWorkspacePrompt(feature: WorkflowFeature, prompt: unknown): string | undefined {
  // Prompt-ignored features never send user text (binary media operations,
  // legacy parity with the canonical compile drop).
  if (PROMPT_IGNORED_FEATURES.has(feature)) return undefined
  const text = `${prompt ?? ''}`.trim()
  // Blank prompts ride as absent: required-policy empties (generation,
  // inpaint) are rejected server-side with 422 `validation_failed`, which
  // stays authoritative instead of duplicating plan-policy checks here.
  if (!text) return undefined
  if (promptByteLength(text) > MAX_WORKSPACE_TASK_PROMPT_BYTES) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'prompt',
      `Canvas task prompt exceeds the v1 ${MAX_WORKSPACE_TASK_PROMPT_BYTES}-byte bound: shorten the prompt instead of submitting truncated text.`,
    )
  }
  return text
}

function nonEmptyStrings(values: unknown): string[] {
  if (!Array.isArray(values)) return []
  return values.map((value) => `${value ?? ''}`.trim()).filter(Boolean)
}

function assertOpaqueAssetIds(values: readonly string[]): string[] {
  const seen = new Set<string>()
  for (const value of values) {
    if (!isOpaqueId(value) || opaqueIdPrefix(value) !== 'asset') {
      throw new CanvasWorkspaceTaskUnmappableError(
        'media-ids',
        `Invalid canvas task media asset id ${JSON.stringify(value)}: v1 media_asset_ids accepts asset_... references only.`,
      )
    }
    seen.add(value)
  }
  return [...seen]
}

// v1-safe param keys (mirrors backend ValidateParams minus everything the
// durable contract forbids). Legacy media URL keys, numeric asset id keys,
// and unknown keys fail closed here so callers never see a cryptic 422.
const WORKSPACE_SAFE_PARAM_KEYS: ReadonlySet<string> = new Set([
  'custom_width',
  'custom_height',
  'original_image_width',
  'original_image_height',
  'ratio',
  'quality',
  'output_count',
  'output_resolution',
  'prompt_upsampling',
  'variation',
  'outpaint',
  'upscale',
  'fusion',
  'use_mask',
])

const LEGACY_MEDIA_PARAM_KEYS: ReadonlySet<string> = new Set([
  'image_url',
  'image_urls',
  'reference_image_urls',
  'mask_image_url',
  'mask_image_urls',
  'original_image_url',
])

const NUMERIC_ASSET_PARAM_KEYS: ReadonlySet<string> = new Set([
  'image_asset_ids',
  'reference_image_asset_ids',
  'original_image_asset_id',
  'mask_image_asset_id',
])

const STORAGE_URL_MARKERS = [
  'http://',
  'https://',
  's3://',
  'storage_key',
  'ws-uploads',
  'builtin-local',
  'bucket',
]

function containsStorageUrlMarker(value: unknown): boolean {
  if (typeof value === 'string') {
    const lowered = value.trim().toLowerCase()
    return STORAGE_URL_MARKERS.some((marker) => lowered.includes(marker))
  }
  if (Array.isArray(value)) return value.some(containsStorageUrlMarker)
  if (typeof value === 'object' && value !== null) {
    return Object.values(value as Record<string, unknown>).some(containsStorageUrlMarker)
  }
  return false
}

function toWorkspaceMaskAssetId(value: unknown): string | undefined {
  // The only durable mask path: an opaque `asset_...` reference minted by
  // the upload-session flow. Mask policy (inpaint-only,
  // inpaint-requires-mask) is enforced server-side; only the shape is
  // validated here.
  if (value == null) return undefined
  if (typeof value !== 'string' || !value.trim()) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-ids',
      'Invalid canvas inpaint mask reference: params.mask_asset_id accepts an asset_... id only.',
    )
  }
  const maskId = value.trim()
  if (maskId.toLowerCase().startsWith('data:') || containsStorageUrlMarker(maskId)) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-urls',
      'Canvas inpaint masks cannot ride task params as data-URLs or storage URLs: upload the mask through the workspace upload-session flow first, then resubmit with params.mask_asset_id.',
    )
  }
  if (!isOpaqueId(maskId) || opaqueIdPrefix(maskId) !== 'asset') {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-ids',
      `Invalid canvas inpaint mask reference ${JSON.stringify(maskId)}: params.mask_asset_id accepts asset_... references only.`,
    )
  }
  return maskId
}

function toWorkspaceParams(params: SubmitTaskData['params']): Record<string, unknown> | undefined {
  if (params == null) return undefined
  const next: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(params)) {
    if (key === 'mask_asset_id') {
      const maskId = toWorkspaceMaskAssetId(value)
      if (maskId != null) next[key] = maskId
      continue
    }
    if (LEGACY_MEDIA_PARAM_KEYS.has(key)) {
      if (key === 'mask_image_url' || key === 'mask_image_urls') {
        throw new CanvasWorkspaceTaskUnmappableError(
          'media-urls',
          'Canvas inpaint masks have no v1 mapping: masks must be uploaded as workspace media assets first; data-URL masks cannot ride durable task params.',
        )
      }
      throw new CanvasWorkspaceTaskUnmappableError(
        'media-urls',
        `Canvas task param ${JSON.stringify(key)} has no v1 mapping: durable task params must never contain media URLs (media rides inputs.media_asset_ids).`,
      )
    }
    if (NUMERIC_ASSET_PARAM_KEYS.has(key)) {
      throw new CanvasWorkspaceTaskUnmappableError(
        'media-ids',
        `Canvas task param ${JSON.stringify(key)} has no v1 mapping: numeric asset ids bypass the v1 asset gate. Resubmit from an asset-backed node so media_asset_ids is populated.`,
      )
    }
    if (!WORKSPACE_SAFE_PARAM_KEYS.has(key)) {
      throw new CanvasWorkspaceTaskUnmappableError(
        'params',
        `Unsupported canvas task param ${JSON.stringify(key)}: v1 rejects unknown params fields.`,
      )
    }
    if (containsStorageUrlMarker(value)) {
      throw new CanvasWorkspaceTaskUnmappableError(
        'params',
        `Canvas task param ${JSON.stringify(key)} has no v1 mapping: params must never contain storage URLs.`,
      )
    }
    next[key] = value
  }
  return Object.keys(next).length > 0 ? next : undefined
}

function toWorkspaceQuantity(params: SubmitTaskData['params']): number | undefined {
  const raw = params?.output_count
  if (raw == null) return undefined
  const count = Number(raw)
  if (!Number.isInteger(count) || count < 1) return 1
  return count
}

function toWorkspaceResolution(params: SubmitTaskData['params']): string | undefined {
  const raw = params?.output_resolution
  if (typeof raw !== 'string' || !raw.trim()) return undefined
  return raw.trim()
}

export function toWorkspaceCreateInput(request: SubmitTaskData): CreateWorkspaceTaskInput {
  const workflowKind = assertWorkflowKind(request.feature)
  const prompt = toWorkspacePrompt(workflowKind, request.prompt)
  if (nonEmptyStrings(request.image_urls).length > 0) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-urls',
      'Canvas task image URLs have no v1 mapping: inputs accept asset_... references only. Resubmit from an asset-backed node so media_asset_ids is populated.',
    )
  }
  if (
    (Array.isArray(request.image_asset_ids) && request.image_asset_ids.length > 0) ||
    (Array.isArray(request.reference_image_asset_ids) && request.reference_image_asset_ids.length > 0)
  ) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-ids',
      'Canvas numeric asset ids have no v1 mapping: inputs accept asset_... references only. Resubmit from an asset-backed node so media_asset_ids is populated.',
    )
  }
  const mediaAssetIds = assertOpaqueAssetIds(nonEmptyStrings(request.media_asset_ids))
  const params = toWorkspaceParams(request.params)
  // NOTE: legacy `priority` (interactive hint) has no v1 field and is
  // dropped; `client_action_id` becomes the header-only Idempotency-Key.
  return {
    workflow_kind: workflowKind,
    ...(mediaAssetIds.length > 0 ? { media_asset_ids: mediaAssetIds } : {}),
    ...(prompt == null ? {} : { prompt }),
    ...(params == null ? {} : { params }),
    ...(toWorkspaceQuantity(request.params) == null
      ? {}
      : { quantity: toWorkspaceQuantity(request.params) as number }),
    ...(toWorkspaceResolution(request.params) == null
      ? {}
      : { resolution: toWorkspaceResolution(request.params) as string }),
  }
}

export async function submitCanvasWorkspaceTask(
  request: SubmitTaskData,
  options?: { idempotencyKey?: string | undefined },
): Promise<{ taskId: string; workspaceId: string }> {
  const workspaceId = requireCanvasWorkspaceId()
  const input = toWorkspaceCreateInput(request)
  const clientActionId = `${request.client_action_id ?? ''}`.trim()
  const idempotencyKey =
    options?.idempotencyKey?.trim() || clientActionId || workspaceTaskIdempotencyKey('create')
  const item = await createWorkspaceTask(workspaceId, input, { idempotencyKey })
  registerCanvasWorkspaceTask(item.id, item.workspace_id)
  notifyUserCreditsInvalidation({ source: 'task-hold', taskId: item.id })
  return { taskId: item.id, workspaceId: item.workspace_id }
}

// ---------------------------------------------------------------------------
// Readiness mapping: SubmitTaskData -> task-quotes -> SubmitTaskReadinessData.
// ---------------------------------------------------------------------------

export function toWorkspaceQuoteInput(request: SubmitTaskData): {
  workflow_kind: WorkflowFeature
  quantity?: number | undefined
  resolution?: string | undefined
} {
  // Quotes are price-only: prompt/media never ride a quote, so readiness
  // preflight never throws unmappable for them (submit does, fail-closed).
  const workflowKind = assertWorkflowKind(request.feature)
  return {
    workflow_kind: workflowKind,
    ...(toWorkspaceQuantity(request.params) == null
      ? {}
      : { quantity: toWorkspaceQuantity(request.params) as number }),
    ...(toWorkspaceResolution(request.params) == null
      ? {}
      : { resolution: toWorkspaceResolution(request.params) as string }),
  }
}

export function toSubmitTaskReadiness(
  snapshot: {
    unit_credits: number
    total_credits: number
    price_version_id: number
    price_version: number
    quantity: number
  },
  feature: WorkflowFeature,
): SubmitTaskReadinessData {
  // Quote snapshots carry no wallet balances: sufficiency is inferred from
  // the absence of 402 (a 402 quote throws insufficient-credits instead of
  // returning data). available/reserved map to 0; quote consumers render
  // unit/total only.
  return {
    ready: true,
    feature,
    billing: {
      sufficient: true,
      unit_credits: snapshot.unit_credits,
      total_credits: snapshot.total_credits,
      available_credits: 0,
      reserved_credits: 0,
      price_version_id: snapshot.price_version_id,
      price_version: snapshot.price_version,
      quantity: snapshot.quantity,
    },
  }
}

function toNotReady(
  feature: WorkflowFeature,
  message: string,
  errorCode: string,
): SubmitTaskReadinessData {
  return { ready: false, message, error_code: errorCode, feature }
}

export async function fetchCanvasWorkspaceTaskReadiness(
  request: SubmitTaskData,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<SubmitTaskReadinessData> {
  const workspaceId = requireCanvasWorkspaceId()
  const input = toWorkspaceQuoteInput(request)
  try {
    const snapshot = await quoteWorkspaceTask(
      workspaceId,
      input,
      options?.signal == null ? undefined : { signal: options.signal },
    )
    return toSubmitTaskReadiness(snapshot, input.workflow_kind)
  } catch (error) {
    const apiError = error as ApiClientError
    const code = apiError?.code
    if (code === 'task_insufficient_credits') throw insufficientCreditsPreflightError()
    if (errorStatus(error) === 422 || code === 'validation_failed') {
      const message =
        error instanceof Error && error.message
          ? error.message
          : i18n.global.t('common.errors.billingInfoUnavailable')
      return toNotReady(input.workflow_kind, message, 'validation_failed')
    }
    throw error
  }
}

export async function fetchCanvasWorkspaceTaskReadinessMatrix(
  requests: readonly SubmitTaskData[],
  options?: { signal?: AbortSignal | null | undefined },
): Promise<SubmitTaskReadinessData[]> {
  if (requests.length === 0) return []
  // No v1 matrix endpoint: one quote per request, order-preserved, first
  // rejection wins (matches the legacy matrix throw-on-transport-failure).
  return Promise.all(requests.map((request) => fetchCanvasWorkspaceTaskReadiness(request, options)))
}

// ---------------------------------------------------------------------------
// Wait mapping: workspace task terminal -> TaskImageResult[].
// ---------------------------------------------------------------------------

const STORAGE_OUTPUT_MARKERS = ['ws-uploads', 'builtin-local', 'storage_key']

function toWorkspaceImageResults(
  workspaceId: string,
  outputs: readonly WorkspaceTaskOutputRef[],
): TaskImageResult[] {
  const results: TaskImageResult[] = []
  for (const ref of outputs) {
    if (ref.kind === 'asset') {
      const assetId = `${ref.asset_id ?? ''}`.trim()
      if (!assetId) continue
      // Explicit workspace binding: the task may settle under a workspace
      // that differs from the current selection (registry path), so the
      // delivery URL must not resolve the workspace implicitly.
      results.push({
        url: resolveWorkspaceMediaAssetDerivativeURL(workspaceId, assetId, 'preview'),
        assetId,
        ...(Number.isInteger(ref.result_index) && ref.result_index >= 0
          ? { resultIndex: ref.result_index }
          : {}),
      })
      continue
    }
    if (ref.kind === 'remote') {
      // v1 refs carry no width/height/mime: downstream resolves display
      // size from the URL. Storage internals never become browser URLs.
      const url = `${ref.url ?? ''}`.trim()
      if (!url) continue
      if (STORAGE_OUTPUT_MARKERS.some((marker) => url.toLowerCase().includes(marker))) continue
      results.push({
        url,
        ...(Number.isInteger(ref.result_index) && ref.result_index >= 0
          ? { resultIndex: ref.result_index }
          : {}),
      })
    }
  }
  return results
}

export function extractWorkspaceTaskImageResults(
  workspaceId: string,
  outputs: readonly WorkspaceTaskOutputRef[] | undefined,
): TaskImageResult[] {
  // Role parity with the legacy extractor (`role === 'image'`), relaxed to
  // accept blank roles: v1 output_refs are image outputs by construction of
  // the canvas flows, and several carry no role fact.
  const imageRefs = (outputs ?? []).filter(
    (ref) => !ref.role?.trim() || ref.role === 'image',
  )
  return toWorkspaceImageResults(workspaceId, imageRefs)
}

function settleWorkspaceTaskItem(taskId: string, item: WorkspaceTaskItem): TaskImageResult[] | null {
  const workspaceId = item.workspace_id
  if (item.status === 'completed') {
    const results = extractWorkspaceTaskImageResults(workspaceId, item.output_refs)
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'completed' })
    if (results.length > 0) return results
    throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImage'))
  }
  if (item.status === 'failed') {
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'failed' })
    // v1 tasks carry no execution_metrics: the legacy finalize-failure
    // pending detection is unavailable, so finalize failures surface as
    // failed immediately (documented residual).
    throw createTaskResultFailureError({ error_msg: item.error_msg })
  }
  if (item.status === 'cancelled') {
    const results = extractWorkspaceTaskImageResults(workspaceId, item.output_refs)
    notifyUserCreditsInvalidation({ source: 'task-settlement', taskId, status: 'cancelled' })
    // cancelled_with_results converges to cancelled on the v1 surface with
    // outputs carrying the results (legacy #676 behavior preserved).
    if (results.length > 0) return results
    throw createTaskCancelledError({
      cancel_reason: item.cancel_reason,
      error_msg: item.error_msg,
    })
  }
  return null
}

function resolveWaitWorkspaceId(taskId: string): string {
  return (
    resolveCanvasTaskWorkspaceId(taskId) ?? requireCanvasWorkspaceId()
  )
}

export async function waitCanvasWorkspaceTaskImageResults(
  taskId: string,
  options?: { timeoutMs?: number | undefined },
): Promise<TaskImageResult[]> {
  const workspaceId = resolveWaitWorkspaceId(taskId)
  const readOptions = options?.timeoutMs == null ? {} : { timeoutMs: options.timeoutMs }
  let fast: WorkspaceTaskItem | undefined
  try {
    fast = await getWorkspaceTask(workspaceId, taskId)
  } catch (error) {
    if (isWorkspaceTaskGone(error)) throw error
    // Non-404 fast-path read failures fall through to the terminal wait
    // (parity with the legacy fast path, which fell back to SSE wait).
  }
  if (fast) {
    // Terminal settlement errors propagate; only live tasks continue below.
    const settled = settleWorkspaceTaskItem(taskId, fast)
    if (settled) return settled
  }
  let item: WorkspaceTaskItem
  try {
    item = await waitWorkspaceTaskTerminal(workspaceId, taskId, readOptions)
  } catch (error) {
    if (isWorkspaceTaskGone(error)) throw error
    if (error instanceof TaskResultPendingError) throw error
    throw new TaskResultPendingError(
      'timeout',
      error instanceof Error ? error.message : '',
    )
  }
  // Settlement errors (failure/cancelled/no-image) propagate unwrapped: only
  // waiter transport rejections map to the timeout pending kind above.
  const settled = settleWorkspaceTaskItem(taskId, item)
  if (settled) return settled
  throw new TaskResultPendingError('timeout')
}

// ---------------------------------------------------------------------------
// Cancel mapping: task id -> workspace-scoped cancel.
// ---------------------------------------------------------------------------

export async function cancelCanvasWorkspaceTask(taskId: string, reason?: string): Promise<void> {
  const trimmed = taskId.trim()
  if (!trimmed) return
  const workspaceId =
    resolveCanvasTaskWorkspaceId(trimmed) ?? getWorkspaceSelectionStore().getSelectedId()
  if (!workspaceId) {
    // Fail closed: never cancel into an unknown workspace. The lifecycle
    // reports this via its final-failure path.
    throw new CanvasWorkspaceUnavailableError(
      'Cannot cancel the task without a workspace selection.',
    )
  }
  try {
    await cancelWorkspaceTask(
      workspaceId,
      trimmed,
      reason?.trim() ? { reason: reason.trim() } : undefined,
      { idempotencyKey: taskCancellationIdempotencyKey(trimmed) },
    )
  } catch (error) {
    // Idempotent cancel: gone (404 converged hidden) and already-terminal
    // (409 state conflict) settle silently instead of retry-looping.
    if (isWorkspaceTaskGone(error) || isTaskStateConflict(error)) return
    const message = error instanceof Error ? error.message : ''
    throw new Error(
      sanitizeApiErrorMessage(message, i18n.global.t('common.errors.taskCancelFailed')),
      { cause: error },
    )
  }
}
