// Canvas task contract mapping (issue #859).
//
// Pure `CanvasTaskRequestData` -> v1 mapping split out of the workspace
// task port: submit input mapping, quote input mapping, and the fail-closed
// guards. No network, no SSE, no Vue: readiness fetching lives in
// `canvasTaskReadiness`, waiting in `canvasTaskWaiting`, cancel/binding in
// `canvasTaskCancellation`.
//
// Catalog rule (#856): services validate against the shared `SERVICE_SLUGS`
// catalog and resolutions parse through the shared `parseOutputResolution`
// catalog. There is no local service/resolution enum here.

import { isOpaqueId, opaqueIdPrefix } from '../../../../../api/contractKernel'
import type { CreateWorkspaceTaskInput } from '../../../../../api/workspaceTaskContract'
import { SERVICE_SLUGS, type Service } from '../../../../../core/contracts/imageServices'
import {
  parseOutputResolution,
  type OutputResolution,
} from '../../../../../core/contracts/generationContract'
import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'

export type CanvasWorkspaceTaskUnmappableReason =
  'prompt' | 'service' | 'media-urls' | 'media-ids' | 'params'

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

/** Shared transport-error inspection for the readiness/cancel seams. */
export function errorStatus(error: unknown): number | undefined {
  const status = (error as { status?: unknown } | null)?.status
  return typeof status === 'number' ? status : undefined
}

/** Shared transport-error inspection for the readiness/cancel seams. */
export function errorCode(error: unknown): unknown {
  return (error as { code?: unknown } | null)?.code
}

// Submit mapping: CanvasTaskRequestData -> CreateWorkspaceTaskInput.
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

function assertService(service: unknown): Service {
  if (typeof service === 'string' && (SERVICE_SLUGS as readonly string[]).includes(service)) {
    return service as Service
  }
  throw new CanvasWorkspaceTaskUnmappableError(
    'service',
    `Unsupported canvas task service ${JSON.stringify(service)}: v1 service accepts only ${SERVICE_SLUGS.join(', ')}.`,
  )
}

/** v1 prompt bound in bytes (mirrors `workspacetasks.MaxWorkspaceTaskPromptBytes`). */
const MAX_WORKSPACE_TASK_PROMPT_BYTES = 8 * 1024

const PROMPT_IGNORED_SERVICES: ReadonlySet<Service> = new Set(['cutout', 'upscale'])

function promptByteLength(prompt: string): number {
  return new TextEncoder().encode(prompt).length
}

function toWorkspacePrompt(service: Service, prompt: unknown): string | undefined {
  // Prompt-ignored services never send user text (binary media operations,
  // legacy parity with the canonical compile drop).
  if (PROMPT_IGNORED_SERVICES.has(service)) return undefined
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
// `output_resolution` is canonicalized separately below: it is the single
// client resolution source of truth (ADR-0012 rule 11).
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

function toWorkspaceParams(
  params: CanvasTaskRequestData['params'],
): Record<string, unknown> | undefined {
  if (params == null) return undefined
  const next: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(params)) {
    if (key === 'mask_asset_id') {
      const maskId = toWorkspaceMaskAssetId(value)
      if (maskId != null) next[key] = maskId
      continue
    }
    if (key === 'output_resolution') {
      // Single ingress canonicalization point: the normalized value is
      // written back so body params and the derived top-level billing
      // resolution agree exactly (never two authoritative values).
      const canonical = normalizeCanvasOutputResolution(value)
      if (canonical != null) next[key] = canonical
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

function toWorkspaceQuantity(params: CanvasTaskRequestData['params']): number | undefined {
  const raw = params?.output_count
  if (raw == null) return undefined
  const count = Number(raw)
  if (!Number.isInteger(count) || count < 1) return 1
  return count
}

// ONE canonical normalized resolution at ingress (ADR-0012 rule 11):
// `params.output_resolution` is the client source of truth. Both the body
// params (written back above) and the top-level billing `resolution`
// derive from this single canonicalizer, so they agree exactly by
// construction. Parsing rides the shared resolution catalog
// (`parseOutputResolution`: case-insensitive `1k`/`2k`/`4k`); unknown
// values fail closed before any network call, so no task is created and
// no hold is taken.
export function normalizeCanvasOutputResolution(value: unknown): OutputResolution | undefined {
  if (value == null) return undefined
  if (typeof value === 'string' && value.trim() === '') return undefined
  const parsed = parseOutputResolution(value)
  if (parsed) return parsed
  throw new CanvasWorkspaceTaskUnmappableError(
    'params',
    `Unsupported canvas task resolution ${JSON.stringify(value)}: params.output_resolution accepts 1k, 2k, or 4k only.`,
  )
}

function toWorkspaceResolution(params: CanvasTaskRequestData['params']): string | undefined {
  return normalizeCanvasOutputResolution(params?.output_resolution)
}

export function toWorkspaceCreateInput(request: CanvasTaskRequestData): CreateWorkspaceTaskInput {
  const service = assertService(request.service)
  const prompt = toWorkspacePrompt(service, request.prompt)
  if (nonEmptyStrings(request.image_urls).length > 0) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-urls',
      'Canvas task image URLs have no v1 mapping: inputs accept asset_... references only. Resubmit from an asset-backed node so media_asset_ids is populated.',
    )
  }
  if (
    (Array.isArray(request.image_asset_ids) && request.image_asset_ids.length > 0) ||
    (Array.isArray(request.reference_image_asset_ids) &&
      request.reference_image_asset_ids.length > 0)
  ) {
    throw new CanvasWorkspaceTaskUnmappableError(
      'media-ids',
      'Canvas numeric asset ids have no v1 mapping: inputs accept asset_... references only. Resubmit from an asset-backed node so media_asset_ids is populated.',
    )
  }
  const mediaAssetIds = assertOpaqueAssetIds(nonEmptyStrings(request.media_asset_ids))
  const params = toWorkspaceParams(request.params)
  const quantity = toWorkspaceQuantity(request.params)
  const resolution = toWorkspaceResolution(request.params)
  // NOTE: legacy `priority` (interactive hint) has no v1 field and is
  // dropped; `client_action_id` becomes the header-only Idempotency-Key.
  return {
    service,
    ...(mediaAssetIds.length > 0 ? { media_asset_ids: mediaAssetIds } : {}),
    ...(prompt == null ? {} : { prompt }),
    ...(params == null ? {} : { params }),
    ...(quantity == null ? {} : { quantity }),
    ...(resolution == null ? {} : { resolution }),
  }
}

export function toWorkspaceQuoteInput(request: CanvasTaskRequestData): {
  service: Service
  quantity?: number | undefined
  resolution?: string | undefined
} {
  // Quotes are price-only: prompt/media never ride a quote, so readiness
  // preflight never throws unmappable for them (submit does, fail-closed).
  const service = assertService(request.service)
  const quantity = toWorkspaceQuantity(request.params)
  const resolution = toWorkspaceResolution(request.params)
  return {
    service,
    ...(quantity == null ? {} : { quantity }),
    ...(resolution == null ? {} : { resolution }),
  }
}
