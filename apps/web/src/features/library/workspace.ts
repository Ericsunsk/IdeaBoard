import { api } from '../../shared/api/client'
import {
  buildContractRequestHeaders,
  parseContractEnvelope,
  requireContractData,
} from '../../shared/api/contractKernel'
import { i18n } from '../../shared/i18n'

// Workspace-scoped library resource (issue #806).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/library`
// through the central `api.request` client (envelope `{ data, meta }` /
// `{ error: { code, message, details, request_id } }`). Pages and
// components must consume this module, never generated code directly.
//
// Migration note: `useLibraryWorkspace` and the floating island still
// read the legacy `user-library` generated client (integer ids). They
// move onto this module once `generated-query/public` regenerates from
// `openapi.public.yaml` (regeneration TODO, owned by the #802-804
// merger follow-up); the legacy integer-id surface is removed in #809.
// This module never accepts integer ids: malformed ids throw before any
// network call, matching the backend 404 posture.

const LIBRARY_ID_RE = /^(?:libf|liba|libm)_[A-Za-z0-9_-]+$/

function isLibraryId(value: unknown): value is string {
  return typeof value === 'string' && LIBRARY_ID_RE.test(value)
}

function libraryIdPrefix(value: string): string {
  return value.slice(0, value.indexOf('_'))
}

function assertLibraryId(value: string, prefix: 'libf' | 'liba' | 'libm', label: string): string {
  const trimmed = value.trim()
  if (!isLibraryId(trimmed) || libraryIdPrefix(trimmed) !== prefix) {
    throw new Error(`Invalid ${label}: expected ${prefix}_...`)
  }
  return trimmed
}

export function assertWorkspaceId(value: string): string {
  const trimmed = value.trim()
  if (!/^ws_[A-Za-z0-9_-]+$/.test(trimmed)) throw new Error('Invalid workspace id: expected ws_...')
  return trimmed
}

export function assertLibraryFolderId(value: string): string {
  return assertLibraryId(value, 'libf', 'library folder id')
}

export function assertLibraryAssetId(value: string): string {
  return assertLibraryId(value, 'liba', 'library asset id')
}

export function assertLibraryMaterializationId(value: string): string {
  return assertLibraryId(value, 'libm', 'library materialization id')
}

function encodeId(value: string): string {
  return encodeURIComponent(value)
}

export function workspaceLibraryFoldersSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/library/folders`
}

export function workspaceLibraryFolderSuffix(workspaceId: string, folderId: string): string {
  return `${workspaceLibraryFoldersSuffix(workspaceId)}/${encodeId(assertLibraryFolderId(folderId))}`
}

export function workspaceLibraryAssetsSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/library/assets`
}

export function workspaceLibraryAssetSuffix(workspaceId: string, assetId: string): string {
  return `${workspaceLibraryAssetsSuffix(workspaceId)}/${encodeId(assertLibraryAssetId(assetId))}`
}

export function workspaceLibraryResolveSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/library/asset-resolutions`
}

export function workspaceLibraryMaterializationsSuffix(workspaceId: string): string {
  return `/v1/workspaces/${encodeId(assertWorkspaceId(workspaceId))}/library/materializations`
}

export function workspaceLibraryMaterializationSuffix(
  workspaceId: string,
  materializationId: string,
): string {
  return `${workspaceLibraryMaterializationsSuffix(workspaceId)}/${encodeId(assertLibraryMaterializationId(materializationId))}`
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function requiredString(raw: unknown, field: string): string {
  if (typeof raw !== 'string' || !raw.trim()) throw new Error(`Invalid library payload: ${field}`)
  return raw
}

function optionalString(raw: unknown): string | undefined {
  return typeof raw === 'string' && raw ? raw : undefined
}

function requiredNumber(raw: unknown, field: string): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw)) {
    throw new Error(`Invalid library payload: ${field}`)
  }
  return raw
}

export type WorkspaceLibraryFolder = {
  id: string
  workspaceId: string
  name: string
  createdAt: string
  updatedAt: string
}

export function toWorkspaceLibraryFolder(raw: unknown): WorkspaceLibraryFolder {
  if (!isRecord(raw)) throw new Error('Invalid library folder payload')
  return {
    id: assertLibraryFolderId(requiredString(raw.id, 'id')),
    workspaceId: assertWorkspaceId(requiredString(raw.workspace_id, 'workspace_id')),
    name: requiredString(raw.name, 'name'),
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
  }
}

export type WorkspaceLibraryAssetProvider = 'unsplash' | 'pexels'

export function mapWorkspaceLibraryProvider(value: unknown): WorkspaceLibraryAssetProvider {
  if (value === 'unsplash' || value === 'pexels') return value
  throw new Error('Invalid library asset payload: provider')
}

export type WorkspaceLibraryAsset = {
  id: string
  workspaceId: string
  folderId?: string | undefined
  provider: WorkspaceLibraryAssetProvider
  sourceId: string
  sourceUrl: string
  previewUrl: string
  originalUrl: string
  title?: string | undefined
  author?: string | undefined
  width: number
  height: number
  createdAt: string
  updatedAt: string
}

export function toWorkspaceLibraryAsset(raw: unknown): WorkspaceLibraryAsset {
  if (!isRecord(raw)) throw new Error('Invalid library asset payload')
  const folderId = optionalString(raw.folder_id)
  if (folderId != null) assertLibraryFolderId(folderId)
  return {
    id: assertLibraryAssetId(requiredString(raw.id, 'id')),
    workspaceId: assertWorkspaceId(requiredString(raw.workspace_id, 'workspace_id')),
    folderId,
    provider: mapWorkspaceLibraryProvider(raw.provider),
    sourceId: requiredString(raw.source_id, 'source_id'),
    sourceUrl: requiredString(raw.source_url, 'source_url'),
    previewUrl: requiredString(raw.preview_url, 'preview_url'),
    originalUrl: requiredString(raw.original_url, 'original_url'),
    title: optionalString(raw.title),
    author: optionalString(raw.author),
    width: requiredNumber(raw.width, 'width'),
    height: requiredNumber(raw.height, 'height'),
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
  }
}

export type WorkspaceLibraryResolveStatus = 'resolved' | 'unresolved'

export function mapWorkspaceLibraryResolveStatus(value: unknown): WorkspaceLibraryResolveStatus {
  if (value === 'resolved' || value === 'unresolved') return value
  throw new Error('Invalid library resolve payload: status')
}

export type WorkspaceLibraryResolveResult = {
  provider: string
  sourceId?: string | undefined
  status: WorkspaceLibraryResolveStatus
  asset?: WorkspaceLibraryAsset | undefined
}

export function toWorkspaceLibraryResolveResult(raw: unknown): WorkspaceLibraryResolveResult {
  if (!isRecord(raw)) throw new Error('Invalid library resolve payload')
  const asset = raw.asset == null ? undefined : toWorkspaceLibraryAsset(raw.asset)
  return {
    provider: requiredString(raw.provider, 'provider'),
    sourceId: optionalString(raw.source_id),
    status: mapWorkspaceLibraryResolveStatus(raw.status),
    asset,
  }
}

export type WorkspaceLibraryMaterializationStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'

export function mapWorkspaceLibraryMaterializationStatus(
  value: unknown,
): WorkspaceLibraryMaterializationStatus {
  if (value === 'pending' || value === 'running' || value === 'completed' || value === 'failed') {
    return value
  }
  throw new Error('Invalid library materialization payload: status')
}

export type WorkspaceLibraryMaterialization = {
  id: string
  workspaceId: string
  libraryAssetId: string
  status: WorkspaceLibraryMaterializationStatus
  mediaAssetId?: string | undefined
  errorCode?: string | undefined
  errorMessage?: string | undefined
  replayed?: boolean | undefined
  createdAt: string
  updatedAt: string
}

export function toWorkspaceLibraryMaterialization(raw: unknown): WorkspaceLibraryMaterialization {
  if (!isRecord(raw)) throw new Error('Invalid library materialization payload')
  const mediaAssetId = optionalString(raw.media_asset_id)
  if (mediaAssetId != null && !/^asset_[A-Za-z0-9_-]+$/.test(mediaAssetId)) {
    throw new Error('Invalid library materialization payload: media_asset_id')
  }
  return {
    id: assertLibraryMaterializationId(requiredString(raw.id, 'id')),
    workspaceId: assertWorkspaceId(requiredString(raw.workspace_id, 'workspace_id')),
    libraryAssetId: assertLibraryAssetId(requiredString(raw.library_asset_id, 'library_asset_id')),
    status: mapWorkspaceLibraryMaterializationStatus(raw.status),
    mediaAssetId,
    errorCode: optionalString(raw.error_code),
    errorMessage: optionalString(raw.error_message),
    replayed: raw.replayed === true ? true : undefined,
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
  }
}

export type WorkspaceLibraryRequestOptions = {
  requestId?: string | undefined
  idempotencyKey?: string | undefined
  signal?: AbortSignal | null | undefined
}

function workspaceLibraryHeaders(
  options: WorkspaceLibraryRequestOptions,
): Record<string, string> {
  return buildContractRequestHeaders({
    requestId: options.requestId,
    idempotencyKey: options.idempotencyKey,
  })
}

export type WorkspaceLibraryPage<T> = {
  items: T[]
  nextCursor: string
  hasMore: boolean
}

function toWorkspaceLibraryPage<T>(
  response: unknown,
  mapItem: (raw: unknown) => T,
  fallback: string,
): WorkspaceLibraryPage<T> {
  const parsed = parseContractEnvelope<unknown[]>(response)
  if (!parsed.ok) {
    const error = new Error(parsed.error.message || fallback) as Error & {
      code?: string | undefined
    }
    error.code = parsed.error.code
    throw error
  }
  if (!Array.isArray(parsed.data)) throw new Error(fallback)
  const meta = parsed.meta ?? {}
  const nextCursor = typeof meta.next_cursor === 'string' ? meta.next_cursor : ''
  return { items: parsed.data.map(mapItem), nextCursor, hasMore: meta.has_more === true }
}

export type WorkspaceLibraryAssetInput = {
  provider: WorkspaceLibraryAssetProvider
  sourceId: string
  sourceUrl?: string | undefined
  previewUrl?: string | undefined
  originalUrl?: string | undefined
  title?: string | undefined
  author?: string | undefined
  width?: number | undefined
  height?: number | undefined
  folderId?: string | undefined
}

export const WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS = 20

export const workspaceLibraryApi = {
  listFolders: async (
    workspaceId: string,
    options: WorkspaceLibraryRequestOptions & { cursor?: string; pageSize?: number } = {},
  ): Promise<WorkspaceLibraryPage<WorkspaceLibraryFolder>> => {
    const query = new URLSearchParams()
    if (options.cursor) query.set('cursor', options.cursor)
    if (options.pageSize != null) query.set('page_size', String(options.pageSize))
    const suffix = workspaceLibraryFoldersSuffix(workspaceId) + (query.size > 0 ? `?${query}` : '')
    const response = await api.request<unknown>(suffix, {
      method: 'GET',
      headers: workspaceLibraryHeaders(options),
      signal: options.signal,
    })
    return toWorkspaceLibraryPage(response, toWorkspaceLibraryFolder, i18n.global.t('library.errors.loadFoldersFailed'))
  },

  createFolder: async (
    workspaceId: string,
    name: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryFolder> => {
    const response = await api.request<unknown>(workspaceLibraryFoldersSuffix(workspaceId), {
      method: 'POST',
      headers: workspaceLibraryHeaders(options),
      body: JSON.stringify({ data: { name } }),
      signal: options.signal,
    })
    return toWorkspaceLibraryFolder(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('library.errors.createFolderFailed'),
      ),
    )
  },

  renameFolder: async (
    workspaceId: string,
    folderId: string,
    name: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryFolder> => {
    const response = await api.request<unknown>(
      workspaceLibraryFolderSuffix(workspaceId, folderId),
      {
        method: 'PATCH',
        headers: workspaceLibraryHeaders(options),
        body: JSON.stringify({ data: { name } }),
        signal: options.signal,
      },
    )
    return toWorkspaceLibraryFolder(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('library.errors.renameFolderFailed'),
      ),
    )
  },

  deleteFolder: async (
    workspaceId: string,
    folderId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<void> => {
    await api.request<unknown>(workspaceLibraryFolderSuffix(workspaceId, folderId), {
      method: 'DELETE',
      headers: workspaceLibraryHeaders(options),
      signal: options.signal,
    })
  },

  listAssets: async (
    workspaceId: string,
    options: WorkspaceLibraryRequestOptions & {
      cursor?: string
      pageSize?: number
      folderId?: string
    } = {},
  ): Promise<WorkspaceLibraryPage<WorkspaceLibraryAsset>> => {
    const query = new URLSearchParams()
    if (options.cursor) query.set('cursor', options.cursor)
    if (options.pageSize != null) query.set('page_size', String(options.pageSize))
    if (options.folderId) query.set('folder_id', assertLibraryFolderId(options.folderId))
    const suffix = workspaceLibraryAssetsSuffix(workspaceId) + (query.size > 0 ? `?${query}` : '')
    const response = await api.request<unknown>(suffix, {
      method: 'GET',
      headers: workspaceLibraryHeaders(options),
      signal: options.signal,
    })
    return toWorkspaceLibraryPage(response, toWorkspaceLibraryAsset, i18n.global.t('library.errors.loadAssetsFailed'))
  },

  saveAsset: async (
    workspaceId: string,
    input: WorkspaceLibraryAssetInput,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryAsset> => {
    const response = await api.request<unknown>(workspaceLibraryAssetsSuffix(workspaceId), {
      method: 'POST',
      headers: workspaceLibraryHeaders(options),
      body: JSON.stringify({
        data: {
          provider: input.provider,
          source_id: input.sourceId,
          ...(input.sourceUrl == null ? {} : { source_url: input.sourceUrl }),
          ...(input.previewUrl == null ? {} : { preview_url: input.previewUrl }),
          ...(input.originalUrl == null ? {} : { original_url: input.originalUrl }),
          ...(input.title == null ? {} : { title: input.title }),
          ...(input.author == null ? {} : { author: input.author }),
          ...(input.width == null ? {} : { width: input.width }),
          ...(input.height == null ? {} : { height: input.height }),
          ...(input.folderId == null ? {} : { folder_id: assertLibraryFolderId(input.folderId) }),
        },
      }),
      signal: options.signal,
    })
    return toWorkspaceLibraryAsset(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('library.errors.saveAssetFailed'),
      ),
    )
  },

  moveAsset: async (
    workspaceId: string,
    assetId: string,
    folderId: string | null,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryAsset> => {
    const response = await api.request<unknown>(
      workspaceLibraryAssetSuffix(workspaceId, assetId),
      {
        method: 'PATCH',
        headers: workspaceLibraryHeaders(options),
        body: JSON.stringify({ data: { folder_id: folderId ?? '' } }),
        signal: options.signal,
      },
    )
    return toWorkspaceLibraryAsset(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('library.errors.moveAssetFailed'),
      ),
    )
  },

  deleteAsset: async (
    workspaceId: string,
    assetId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<void> => {
    await api.request<unknown>(workspaceLibraryAssetSuffix(workspaceId, assetId), {
      method: 'DELETE',
      headers: workspaceLibraryHeaders(options),
      signal: options.signal,
    })
  },

  resolveAssets: async (
    workspaceId: string,
    items: Array<{ provider: string; sourceId?: string; sourceUrl?: string }>,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryResolveResult[]> => {
    if (items.length > WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS) {
      throw new Error(i18n.global.t('library.errors.lookupStatusFailed'))
    }
    const response = await api.request<unknown>(workspaceLibraryResolveSuffix(workspaceId), {
      method: 'POST',
      headers: workspaceLibraryHeaders(options),
      body: JSON.stringify({
        data: {
          items: items.map((item) => ({
            provider: item.provider,
            ...(item.sourceId == null ? {} : { source_id: item.sourceId }),
            ...(item.sourceUrl == null ? {} : { source_url: item.sourceUrl }),
          })),
        },
      }),
      signal: options.signal,
    })
    const data = requireContractData<Record<string, unknown>>(
      response,
      i18n.global.t('library.errors.lookupStatusFailed'),
    )
    if (!Array.isArray(data.items)) throw new Error(i18n.global.t('library.errors.lookupStatusFailed'))
    return data.items.map(toWorkspaceLibraryResolveResult)
  },

  createMaterialization: async (
    workspaceId: string,
    libraryAssetId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryMaterialization> => {
    const response = await api.request<unknown>(
      workspaceLibraryMaterializationsSuffix(workspaceId),
      {
        method: 'POST',
        headers: workspaceLibraryHeaders(options),
        body: JSON.stringify({ data: { library_asset_id: assertLibraryAssetId(libraryAssetId) } }),
        signal: options.signal,
      },
    )
    return toWorkspaceLibraryMaterialization(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('library.errors.importAssetFailed'),
      ),
    )
  },

  getMaterialization: async (
    workspaceId: string,
    materializationId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryMaterialization> => {
    const response = await api.request<unknown>(
      workspaceLibraryMaterializationSuffix(workspaceId, materializationId),
      {
        method: 'GET',
        headers: workspaceLibraryHeaders(options),
        signal: options.signal,
      },
    )
    return toWorkspaceLibraryMaterialization(
      requireContractData<Record<string, unknown>>(
        response,
        i18n.global.t('library.errors.importAssetFailed'),
      ),
    )
  },
}
