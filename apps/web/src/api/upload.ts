import {
  archiveWorkspaceMediaAsset as archiveWorkspaceMediaAssetTransport,
  completeWorkspaceUpload as completeWorkspaceUploadTransport,
  createWorkspaceUpload as createWorkspaceUploadTransport,
  getWorkspaceUpload as getWorkspaceUploadTransport,
  stageWorkspaceUploadBytes as stageWorkspaceUploadBytesTransport,
} from './generated-query/public/workspace-media/workspace-media'
import { uploadAvatar as uploadAvatarTransport } from './generated-query/public/user/user'
import type { CompleteUploadRequest, CreateUploadRequest } from './generated-query/public/model'
import { buildContractRequestHeaders, newRequestId, requireContractData } from './contractKernel'
import { withSignal } from '../core/utils/request'
import { assertOpaqueId, isOpaqueIdWithPrefix } from '../core/utils/opaqueIds'
import { isRecord } from '../core/utils/guards'
import { workspaceUploadIdempotencyKey } from './idempotencyKey'
import { i18n } from '../core/i18n'

export const UPLOAD_OPERATION_TIMEOUT_MS = 5 * 60 * 1000

export type UploadImageMetadata = {
  url: string
  asset_id: string
  width?: number
  height?: number
  mime?: string
  ext?: string
}

export type UploadAvatarMetadata = {
  avatar: string
  width?: number
  height?: number
  mime?: string
  ext?: string
}

function avatarFileExtension(file: File): string | undefined {
  const name = file.name.trim()
  const dot = name.lastIndexOf('.')
  if (dot < 0 || dot === name.length - 1) return undefined
  const ext = name.slice(dot).toLowerCase()
  return /^\.[a-z0-9]{1,8}$/.test(ext) ? ext : undefined
}

export const uploadApi = {
  upload: async (
    file: File,
    workspaceId: string,
    options?: { boardId?: string | undefined },
  ): Promise<UploadImageMetadata> => {
    const workspace = workspaceId.trim()
    if (!workspace) throw new Error(i18n.global.t('common.errors.uploadFailed'))
    const bytes = await file.arrayBuffer()
    const subtle = globalThis.crypto?.subtle
    if (!subtle) throw new Error(i18n.global.t('common.errors.uploadFailed'))
    const digest = await subtle.digest('SHA-256', bytes)
    const checksumSha256 = [...new Uint8Array(digest)]
      .map((byte) => byte.toString(16).padStart(2, '0'))
      .join('')
    const mimeType = file.type || 'application/octet-stream'
    const session = await workspaceUploadApi.createSession(
      workspace,
      {
        filename: file.name,
        mimeType,
        sizeBytes: file.size,
        checksumSha256,
        mediaKind: 'image',
        // #936: bind Canvas uploads to their board when the board has a
        // durable identity; unsaved boards stay unbound (owner-only).
        ...(options?.boardId != null ? { boardId: options.boardId } : {}),
      },
      { idempotencyKey: workspaceUploadIdempotencyKey('create') },
    )
    await workspaceUploadApi.stageBytes(workspace, session.id, file, {
      contentType: mimeType,
      idempotencyKey: workspaceUploadIdempotencyKey('bytes'),
    })
    const completed = await workspaceUploadApi.completeSession(
      workspace,
      session.id,
      { checksumSha256, sizeBytes: file.size, mimeType, mediaKind: 'image' },
      { idempotencyKey: workspaceUploadIdempotencyKey('complete') },
    )
    const asset = completed.asset
    if (!asset.id?.trim()) throw new Error(i18n.global.t('common.errors.uploadProcessingFailed'))
    return {
      url: asset.contentUrl,
      asset_id: asset.id,
      width: asset.width,
      height: asset.height,
      mime: asset.mimeType,
    }
  },
  uploadAvatar: async (file: File): Promise<UploadAvatarMetadata> => {
    // v1 avatar surface (`PUT /api/v1/user/avatar`, raw image bytes, never
    // JSON/multipart). Requires the Idempotency-Key header (scope
    // avatar:upload); each user action mints its own key.
    const envelope = await uploadAvatarTransport(file, {
      headers: {
        ...buildContractRequestHeaders({
          requestId: newRequestId(),
          idempotencyKey: `avatar:upload:${newRequestId()}`,
        }),
        ...(file.type.trim() ? { 'Content-Type': file.type.trim() } : {}),
      },
      timeout: UPLOAD_OPERATION_TIMEOUT_MS,
    })
    const result = requireContractData<{ avatar_url: string }>(
      envelope,
      i18n.global.t('common.errors.avatarUploadFailed'),
    )
    if (!result.avatar_url?.trim()) {
      throw new Error(i18n.global.t('common.errors.avatarUploadFailed'))
    }
    // The v1 result carries only the served URL; mime/ext ride from the
    // staged File (client-known facts). Width/height stay absent (the v1
    // surface does not return dimensions).
    const metadata: UploadAvatarMetadata = { avatar: result.avatar_url }
    if (file.type.trim()) metadata.mime = file.type.trim()
    const ext = avatarFileExtension(file)
    if (ext != null) metadata.ext = ext
    return metadata
  },
}

// ---------------------------------------------------------------------------
// Workspace upload sessions + media assets (issue #803, Public Data Plane).
//
// Session declare/stage/complete and media archive ride the generated
// `public/workspace-media` hooks (`/api/v1/...` via `publicMutator`);
// metadata reads go through `mediaAssetDelivery` on the same hooks.
// This module keeps the opaque-id asserts, slot guards, storage-leak
// guards, and per-operation idempotency plumbing that have no generated
// equivalent. Callers consume this module, never generated code directly.
// Board/Task/Library adoption of these URLs (asset references by
// `asset_...` only) is #805/#806.

export type WorkspaceUploadSessionStatus =
  'pending' | 'transferring' | 'completed' | 'failed' | 'expired'

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

export function assertWorkspaceId(value: string): string {
  // Intentional local twin of `core/utils/workspaceId.assertWorkspaceId`:
  // parity-pinned by `opaqueIds.unit.spec.ts`; the canonical primitive stays
  // in `core/utils/workspaceId` + `core/utils/opaqueIds`.
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

// Route builders kept for readers that cannot take the generated hooks:
// `mediaAssetDelivery` (out of scope) builds delivery URLs from these
// suffixes. All other routes moved to the generated URL helpers
// (`getCreateWorkspaceUploadUrl`, ...); see the contract spec.
function workspaceMediaAssetsSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/media-assets`
}

function workspaceMediaAssetSuffix(workspaceId: string, assetId: string): string {
  return `${workspaceMediaAssetsSuffix(workspaceId)}/${encodeId(assertWorkspaceAssetId(assetId))}`
}

export function workspaceMediaAssetContentSuffix(workspaceId: string, assetId: string): string {
  return `${workspaceMediaAssetSuffix(workspaceId, assetId)}/content`
}

export type WorkspaceMediaDerivativeVariant = 'thumb' | 'preview' | 'large-preview'

export function workspaceMediaAssetDerivativeSuffix(
  workspaceId: string,
  assetId: string,
  variant: WorkspaceMediaDerivativeVariant,
): string {
  return `${workspaceMediaAssetSuffix(workspaceId, assetId)}/derivatives/${encodeId(variant)}`
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
  /**
   * Optional board the upload is captured for (#936: restricted upload
   * target). Only `board_...` shapes are sent; anything else (unsaved
   * canvas boards, local ids) stays unbound so the asset is owner-only
   * until another source grants wider access. The server verifies the
   * creator's board write access — never trusted from the client.
   */
  boardId?: string | undefined
}

/** Mirrors the backend `board_` public-id shape (prefix + alnum/`-`/`_` suffix). */
export function isBoardPublicIdShape(raw: unknown): raw is string {
  return typeof raw === 'string' && isOpaqueIdWithPrefix(raw.trim(), 'board')
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
    const data: CreateUploadRequest['data'] = {
      filename: input.filename,
      mime_type: input.mimeType,
      size_bytes: input.sizeBytes,
      checksum_sha256: input.checksumSha256,
    }
    data.media_kind = (input.mediaKind ?? 'image') as NonNullable<
      CreateUploadRequest['data']['media_kind']
    >
    if (isBoardPublicIdShape(input.boardId)) data.board_id = input.boardId.trim()
    const envelope = await createWorkspaceUploadTransport(
      assertWorkspaceId(workspaceId),
      { data },
      {
        headers: workspaceRequestHeaders(options),
        ...withSignal(options.signal),
      },
    )
    return toWorkspaceUploadSession(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('common.errors.uploadFailed'),
      ),
    )
  },

  getSession: async (
    workspaceId: string,
    uploadId: string,
    options: WorkspaceRequestOptions = {},
  ): Promise<WorkspaceUploadSession> => {
    const envelope = await getWorkspaceUploadTransport(
      assertWorkspaceId(workspaceId),
      assertUploadId(uploadId),
      {
        headers: workspaceRequestHeaders(options),
        ...withSignal(options.signal),
      },
    )
    return toWorkspaceUploadSession(
      requireContractData<Record<string, unknown>>(
        envelope,
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
    const data: CompleteUploadRequest['data'] = {
      checksum_sha256: input.checksumSha256,
      size_bytes: input.sizeBytes,
    }
    if (input.mimeType != null) data.mime_type = input.mimeType
    if (input.mediaKind != null) {
      data.media_kind = input.mediaKind as NonNullable<CompleteUploadRequest['data']['media_kind']>
    }
    if (input.width != null) data.width = input.width
    if (input.height != null) data.height = input.height
    const envelope = await completeWorkspaceUploadTransport(
      assertWorkspaceId(workspaceId),
      assertUploadId(uploadId),
      { data },
      {
        headers: workspaceRequestHeaders(options),
        ...withSignal(options.signal),
      },
    )
    const result = requireContractData<Record<string, unknown>>(
      envelope,
      i18n.global.t('common.errors.uploadFailed'),
    )
    if (!isRecord(result.upload) || !isRecord(result.asset)) {
      throw new Error(i18n.global.t('common.errors.uploadFailed'))
    }
    return {
      upload: toWorkspaceUploadSession(result.upload),
      asset: toWorkspaceMediaAsset(result.asset),
    }
  },

  /**
   * Stages the declared bytes at the session slot (`PUT .../uploads/{id}/bytes`).
   * Raw bytes body (never JSON); the response stays the JSON session envelope.
   * Retryable: 422 validation mismatches change no server state. The caller
   * passes the declared MIME as `contentType` so the byte PUT echoes the
   * declaration; per-operation idempotency uses the `upload:bytes` scope.
   */
  stageBytes: async (
    workspaceId: string,
    uploadId: string,
    body: Blob,
    options: WorkspaceRequestOptions & { contentType?: string | undefined } = {},
  ): Promise<WorkspaceUploadSession> => {
    const envelope = await stageWorkspaceUploadBytesTransport(
      assertWorkspaceId(workspaceId),
      assertUploadId(uploadId),
      body,
      {
        headers: {
          ...workspaceRequestHeaders(options),
          ...(options.contentType?.trim() ? { 'Content-Type': options.contentType.trim() } : {}),
        },
        ...withSignal(options.signal),
      },
    )
    return toWorkspaceUploadSession(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('common.errors.uploadFailed'),
      ),
    )
  },
}

export const workspaceMediaApi = {
  archiveAsset: async (
    workspaceId: string,
    assetId: string,
    options: WorkspaceRequestOptions = {},
  ): Promise<WorkspaceMediaAsset> => {
    const envelope = await archiveWorkspaceMediaAssetTransport(
      assertWorkspaceId(workspaceId),
      assertWorkspaceAssetId(assetId),
      {
        headers: workspaceRequestHeaders(options),
        ...withSignal(options.signal),
      },
    )
    return toWorkspaceMediaAsset(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('common.errors.requestIncomplete'),
      ),
    )
  },
}
