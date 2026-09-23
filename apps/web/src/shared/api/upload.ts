import {
  uploadUserBoardAsset,
  uploadUserProfileAvatar,
} from './generated-query/user-assets/user-assets'
import type { UploadAvatarMetadata, UploadImageMetadata } from './generated-query/model'
import { resolveApiData } from './apiResponse'
import { api } from './client'
import {
  buildContractRequestHeaders,
  isOpaqueId,
  opaqueIdPrefix,
  requireContractData,
  type OpaqueIdPrefix,
} from './contractKernel'
import { i18n } from '../i18n'

export const UPLOAD_OPERATION_TIMEOUT_MS = 5 * 60 * 1000

function getUploadRequestOptions(): RequestInit {
  return { timeout: UPLOAD_OPERATION_TIMEOUT_MS } as RequestInit
}

export const uploadApi = {
  upload: async (file: File, boardId: string): Promise<UploadImageMetadata> => {
    const normalizedBoardId = boardId.trim()
    const data = await resolveApiData(
      uploadUserBoardAsset(normalizedBoardId, { file }, getUploadRequestOptions()),
      i18n.global.t('common.errors.uploadFailed'),
    )
    if (!data.url?.trim()) throw new Error(i18n.global.t('common.errors.uploadFailed'))
    if (!data.asset_id?.trim()) {
      throw new Error(i18n.global.t('common.errors.uploadProcessingFailed'))
    }
    return data
  },
  uploadAvatar: async (file: File): Promise<UploadAvatarMetadata> => {
    const data = await resolveApiData(
      uploadUserProfileAvatar({ file }, getUploadRequestOptions()),
      i18n.global.t('common.errors.avatarUploadFailed'),
    )
    if (!data.avatar?.trim()) throw new Error(i18n.global.t('common.errors.avatarUploadFailed'))
    return data
  },
}

// ---------------------------------------------------------------------------
// Workspace upload sessions + media assets (issue #803, Public Data Plane).
//
// Canonical session URLs live under `/api/v1/workspaces/{workspace_id}`.
// The builders below return transport suffixes WITHOUT the `/api` prefix
// because `api.request` already mounts `API_BASE_URL` (`/api`); MSW and
// browser assertions use the full `/api/v1/...` form.
//
// TODO(regenerate): move onto `generated-query/public` hooks now that
// `openapi.public.yaml` carries the uploads/media paths (post #802-804
// merger); `api.request` keeps the transport central until then.
// Board/Task/Library adoption of these URLs (asset references by
// `asset_...` only) is #805/#806.

export type WorkspaceUploadSessionStatus =
  | 'pending'
  | 'transferring'
  | 'completed'
  | 'failed'
  | 'expired'

const WORKSPACE_UPLOAD_SESSION_STATUSES: readonly WorkspaceUploadSessionStatus[] = [
  'pending',
  'transferring',
  'completed',
  'failed',
  'expired',
]

export function mapWorkspaceUploadSessionStatus(value: unknown): WorkspaceUploadSessionStatus {
  if (
    typeof value === 'string' &&
    (WORKSPACE_UPLOAD_SESSION_STATUSES as readonly string[]).includes(value)
  ) {
    return value as WorkspaceUploadSessionStatus
  }
  throw new Error(`Unknown upload session status: ${String(value)}`)
}

function assertOpaqueId(value: string, prefix: OpaqueIdPrefix, label: string): string {
  const trimmed = value.trim()
  if (!isOpaqueId(trimmed) || opaqueIdPrefix(trimmed) !== prefix) {
    throw new Error(`Invalid ${label}: expected ${prefix}_...`)
  }
  return trimmed
}

export function assertWorkspaceId(value: string): string {
  return assertOpaqueId(value, 'ws', 'workspace id')
}

export function assertUploadId(value: string): string {
  return assertOpaqueId(value, 'upload', 'upload id')
}

export function assertWorkspaceAssetId(value: string): string {
  return assertOpaqueId(value, 'asset', 'asset id')
}

function encodeId(value: string): string {
  return encodeURIComponent(value)
}

// Transport suffixes (no `/api` prefix; see module note).

export function workspaceUploadsSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/uploads`
}

export function workspaceUploadSessionSuffix(workspaceId: string, uploadId: string): string {
  return `${workspaceUploadsSuffix(workspaceId)}/${encodeId(assertUploadId(uploadId))}`
}

export function workspaceUploadCompleteSuffix(workspaceId: string, uploadId: string): string {
  return `${workspaceUploadSessionSuffix(workspaceId, uploadId)}/complete`
}

export function workspaceMediaAssetsSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/media-assets`
}

export function workspaceMediaAssetSuffix(workspaceId: string, assetId: string): string {
  return `${workspaceMediaAssetsSuffix(workspaceId)}/${encodeId(assertWorkspaceAssetId(assetId))}`
}

export function workspaceMediaAssetContentSuffix(workspaceId: string, assetId: string): string {
  return `${workspaceMediaAssetSuffix(workspaceId, assetId)}/content`
}

export type WorkspaceMediaDerivativeVariant =
  | 'thumb'
  | 'preview'
  | 'large-preview'
  | 'processing-input'

export function workspaceMediaAssetDerivativeSuffix(
  workspaceId: string,
  assetId: string,
  variant: WorkspaceMediaDerivativeVariant,
): string {
  return `${workspaceMediaAssetSuffix(workspaceId, assetId)}/derivatives/${encodeId(variant)}`
}

export function workspaceMediaAssetArchiveSuffix(workspaceId: string, assetId: string): string {
  return `${workspaceMediaAssetSuffix(workspaceId, assetId)}/archive`
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requiredString(raw: unknown, field: string): string {
  if (typeof raw !== 'string' || !raw.trim()) throw new Error(`Invalid upload payload: ${field}`)
  return raw
}

export type WorkspaceUploadTarget = {
  kind: string
  slot: string
  expiresAt: string
}

export type WorkspaceUploadSession = {
  id: string
  workspaceId: string
  status: WorkspaceUploadSessionStatus
  filename: string
  mimeType: string
  sizeBytes: number
  checksumSha256: string
  mediaKind: string
  uploadTarget: WorkspaceUploadTarget
  assetId?: string | undefined
  createdAt: string
  updatedAt: string
  expiresAt: string
}

export function toWorkspaceUploadSession(raw: unknown): WorkspaceUploadSession {
  if (!isRecord(raw)) throw new Error('Invalid upload session payload')
  const id = assertUploadId(requiredString(raw.id, 'id'))
  const workspaceId = assertWorkspaceId(requiredString(raw.workspace_id, 'workspace_id'))
  const target = isRecord(raw.upload_target) ? raw.upload_target : null
  if (!target) throw new Error('Invalid upload session payload: upload_target')
  const session: WorkspaceUploadSession = {
    id,
    workspaceId,
    status: mapWorkspaceUploadSessionStatus(raw.status),
    filename: requiredString(raw.filename, 'filename'),
    mimeType: requiredString(raw.mime_type, 'mime_type'),
    sizeBytes: typeof raw.size_bytes === 'number' ? raw.size_bytes : Number.NaN,
    checksumSha256: requiredString(raw.checksum_sha256, 'checksum_sha256'),
    mediaKind: requiredString(raw.media_kind, 'media_kind'),
    uploadTarget: {
      kind: requiredString(target.kind, 'upload_target.kind'),
      slot: requiredString(target.slot, 'upload_target.slot'),
      expiresAt: requiredString(target.expires_at, 'upload_target.expires_at'),
    },
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
    expiresAt: requiredString(raw.expires_at, 'expires_at'),
  }
  if (Number.isNaN(session.sizeBytes)) throw new Error('Invalid upload payload: size_bytes')
  // The slot is opaque server coordination: it must equal the session id
  // and never carry storage material (bucket/secret/path/key).
  if (session.uploadTarget.slot !== session.id) {
    throw new Error('Invalid upload payload: upload_target.slot')
  }
  for (const leaked of ['bucket', 'secret', 'storage_key', 'storageKey']) {
    if (session.uploadTarget.slot.includes(leaked)) {
      throw new Error('Invalid upload payload: upload_target.slot')
    }
  }
  if (typeof raw.asset_id === 'string' && raw.asset_id.trim()) {
    session.assetId = assertWorkspaceAssetId(raw.asset_id)
  }
  return session
}

export type WorkspaceMediaAsset = {
  id: string
  workspaceId: string
  status: string
  mediaKind: string
  mimeType: string
  width: number
  height: number
  sizeBytes: number
  checksumSha256: string
  contentUrl: string
  archivedAt?: string | undefined
  createdAt: string
  updatedAt: string
}

export function toWorkspaceMediaAsset(raw: unknown): WorkspaceMediaAsset {
  if (!isRecord(raw)) throw new Error('Invalid media asset payload')
  const asset: WorkspaceMediaAsset = {
    id: assertWorkspaceAssetId(requiredString(raw.id, 'id')),
    workspaceId: assertWorkspaceId(requiredString(raw.workspace_id, 'workspace_id')),
    status: requiredString(raw.status, 'status'),
    mediaKind: requiredString(raw.media_kind, 'media_kind'),
    mimeType: requiredString(raw.mime_type, 'mime_type'),
    width: typeof raw.width === 'number' ? raw.width : Number.NaN,
    height: typeof raw.height === 'number' ? raw.height : Number.NaN,
    sizeBytes: typeof raw.size_bytes === 'number' ? raw.size_bytes : Number.NaN,
    checksumSha256: requiredString(raw.checksum_sha256, 'checksum_sha256'),
    contentUrl: requiredString(raw.content_url, 'content_url'),
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
  }
  if (Number.isNaN(asset.width) || Number.isNaN(asset.height) || Number.isNaN(asset.sizeBytes)) {
    throw new Error('Invalid media asset payload: dimensions')
  }
  // Content URLs carry asset_... only; storage keys never leave the server.
  if (!asset.contentUrl.includes(asset.id) || /storage[_-]?key/i.test(asset.contentUrl)) {
    throw new Error('Invalid media asset payload: content_url')
  }
  if (typeof raw.archived_at === 'string' && raw.archived_at.trim()) {
    asset.archivedAt = raw.archived_at
  }
  return asset
}

export type WorkspaceUploadCreateInput = {
  filename: string
  mimeType: string
  sizeBytes: number
  checksumSha256: string
  mediaKind?: string | undefined
}

export type WorkspaceUploadCompleteInput = {
  checksumSha256: string
  sizeBytes: number
  mimeType?: string | undefined
  mediaKind?: string | undefined
  width?: number | undefined
  height?: number | undefined
}

export type WorkspaceRequestOptions = {
  idempotencyKey?: string | undefined
  requestId?: string | undefined
  signal?: AbortSignal | null | undefined
}

function workspaceRequestHeaders(options: WorkspaceRequestOptions): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: options.requestId,
    idempotencyKey: options.idempotencyKey,
  })
}

export const workspaceUploadApi = {
  createSession: async (
    workspaceId: string,
    input: WorkspaceUploadCreateInput,
    options: WorkspaceRequestOptions = {},
  ): Promise<WorkspaceUploadSession> => {
    const response = await api.request<unknown>(workspaceUploadsSuffix(workspaceId), {
      method: 'POST',
      headers: workspaceRequestHeaders(options),
      body: JSON.stringify({
        data: {
          filename: input.filename,
          mime_type: input.mimeType,
          size_bytes: input.sizeBytes,
          checksum_sha256: input.checksumSha256,
          media_kind: input.mediaKind ?? 'image',
        },
      }),
      signal: options.signal,
    })
    return toWorkspaceUploadSession(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('common.errors.uploadFailed'),
      ),
    )
  },

  getSession: async (
    workspaceId: string,
    uploadId: string,
    options: WorkspaceRequestOptions = {},
  ): Promise<WorkspaceUploadSession> => {
    const response = await api.request<unknown>(
      workspaceUploadSessionSuffix(workspaceId, uploadId),
      {
        method: 'GET',
        headers: workspaceRequestHeaders(options),
        signal: options.signal,
      },
    )
    return toWorkspaceUploadSession(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('common.errors.uploadFailed'),
      ),
    )
  },

  completeSession: async (
    workspaceId: string,
    uploadId: string,
    input: WorkspaceUploadCompleteInput,
    options: WorkspaceRequestOptions = {},
  ): Promise<{ upload: WorkspaceUploadSession; asset: WorkspaceMediaAsset }> => {
    const response = await api.request<unknown>(
      workspaceUploadCompleteSuffix(workspaceId, uploadId),
      {
        method: 'POST',
        headers: workspaceRequestHeaders(options),
        body: JSON.stringify({
          data: {
            checksum_sha256: input.checksumSha256,
            size_bytes: input.sizeBytes,
            ...(input.mimeType == null ? {} : { mime_type: input.mimeType }),
            ...(input.mediaKind == null ? {} : { media_kind: input.mediaKind }),
            ...(input.width == null ? {} : { width: input.width }),
            ...(input.height == null ? {} : { height: input.height }),
          },
        }),
        signal: options.signal,
      },
    )
    const data = requireContractData<Record<string, unknown>>(
      response,
      i18n.global.t('common.errors.uploadFailed'),
    )
    if (!isRecord(data.upload) || !isRecord(data.asset)) {
      throw new Error(i18n.global.t('common.errors.uploadFailed'))
    }
    return { upload: toWorkspaceUploadSession(data.upload), asset: toWorkspaceMediaAsset(data.asset) }
  },
}

export const workspaceMediaApi = {
  archiveAsset: async (
    workspaceId: string,
    assetId: string,
    options: WorkspaceRequestOptions = {},
  ): Promise<WorkspaceMediaAsset> => {
    const response = await api.request<unknown>(
      workspaceMediaAssetArchiveSuffix(workspaceId, assetId),
      {
        method: 'POST',
        headers: workspaceRequestHeaders(options),
        signal: options.signal,
      },
    )
    return toWorkspaceMediaAsset(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('common.errors.requestIncomplete'),
      ),
    )
  },
}
