import { api } from '../../shared/api/client'
import {
  buildContractRequestHeaders,
  parseContractEnvelope,
  requireContractData,
} from '../../shared/api/contractKernel'
import { getMediaAssetMetadata } from '../../shared/api/mediaAssetDelivery'
import { resolveAssetUrl } from '../../shared/config/runtimeUrls'
import { createUniqueId } from '../../shared/utils/ids'
import { i18n } from '../../shared/i18n'

// Workspace-scoped library resource (issue #806).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/library`
// through the central `api.request` client (envelope `{ data, meta }` /
// `{ error: { code, message, details, request_id } }`). Pages and
// components must consume this module, never generated code directly.
//
// This module is the SOLE library transport: the legacy user-scoped
// `resource.ts` (integer ids, `/user/library/*`) is deleted. Ids are
// opaque (`ws_` + canonical `lfolder_` / `lasset_` / `lmat_`);
//
// TODO(#806-regen, 2026-09-25): BLOCKED on generated-query/public regen —
// Library requests use the feature adapter so route and response validation
// stay owned by this context.
// while this module ships `/v1/...` suffixes; `orval.config.ts publicQuery`
// must point at `publicMutator` and regen (`api:generate`) is out of scope
// here. Keep this transport until then; do not force-fit (auto idempotency
// minting, resolve batch bound, snake_case -> camelCase validators stay).
// malformed ids throw before any network call, matching the backend 404
// posture. Payloads map snake_case to camelCase at this boundary;
// callers only see camelCase.
//
// Backend notes callers must respect:
// - Folders are plain workspace folders: no system/virtual taxonomy, no
//   `quick-saves`, no asset counts. The island synthesizes the virtual
//   `all` aggregate client-side; saving without a folder leaves the asset
//   unfiled (visible under `all`). Deleting a folder unfiles its assets.
// - Asset save is idempotent on `(workspace, provider, source_id)`.
// - Materialization is an async job (`pending -> running ->
//   completed|failed`); `materializeLibraryAssetForCanvas` polls the job
//   and resolves display media through the shared delivery boundary.

const LIBRARY_ID_RE = /^(?:lfolder|lasset|lmat)_[A-Za-z0-9_-]+$/

function isLibraryId(value: unknown): value is string {
  return typeof value === 'string' && LIBRARY_ID_RE.test(value)
}

function libraryIdPrefix(value: string): string {
  return value.slice(0, value.indexOf('_'))
}

const LIBRARY_FOLDER_PREFIXES = ['lfolder'] as const
const LIBRARY_ASSET_PREFIXES = ['lasset'] as const
const LIBRARY_MATERIALIZATION_PREFIXES = ['lmat'] as const

function assertLibraryId(
  value: string,
  prefixes: readonly string[],
  label: string,
  canonical: string,
): string {
  const trimmed = value.trim()
  if (!isLibraryId(trimmed) || !prefixes.includes(libraryIdPrefix(trimmed))) {
    throw new Error(`Invalid ${label}: expected ${canonical}_...`)
  }
  return trimmed
}

export function assertWorkspaceId(value: string): string {
  const trimmed = value.trim()
  if (!/^ws_[A-Za-z0-9_-]+$/.test(trimmed)) throw new Error('Invalid workspace id: expected ws_...')
  return trimmed
}

export function assertLibraryFolderId(value: string): string {
  return assertLibraryId(value, LIBRARY_FOLDER_PREFIXES, 'library folder id', 'lfolder')
}

export function assertLibraryAssetId(value: string): string {
  return assertLibraryId(value, LIBRARY_ASSET_PREFIXES, 'library asset id', 'lasset')
}

export function assertLibraryMaterializationId(value: string): string {
  return assertLibraryId(value, LIBRARY_MATERIALIZATION_PREFIXES, 'library materialization id', 'lmat')
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

// Mutating endpoints require an `Idempotency-Key` (per-endpoint scope, see
// openapi). Default to a scoped unique key so callers stay replay-safe
// without threading keys through every island/popover call.
function withLibraryIdempotency(
  options: WorkspaceLibraryRequestOptions,
  scope: string,
): WorkspaceLibraryRequestOptions {
  if (options.idempotencyKey) return options
  return { ...options, idempotencyKey: `${scope}:${createUniqueId()}` }
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
    const effective = withLibraryIdempotency(options, 'library:folder-create')
    const response = await api.request<unknown>(workspaceLibraryFoldersSuffix(workspaceId), {
      method: 'POST',
      headers: workspaceLibraryHeaders(effective),
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
    const effective = withLibraryIdempotency(options, 'library:folder-delete')
    await api.request<unknown>(workspaceLibraryFolderSuffix(workspaceId, folderId), {
      method: 'DELETE',
      headers: workspaceLibraryHeaders(effective),
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
    const effective = withLibraryIdempotency(options, 'library:asset-save')
    const response = await api.request<unknown>(workspaceLibraryAssetsSuffix(workspaceId), {
      method: 'POST',
      headers: workspaceLibraryHeaders(effective),
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
    const effective = withLibraryIdempotency(options, 'library:asset-delete')
    await api.request<unknown>(workspaceLibraryAssetSuffix(workspaceId, assetId), {
      method: 'DELETE',
      headers: workspaceLibraryHeaders(effective),
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
    const effective = withLibraryIdempotency(options, 'library:materialize')
    const response = await api.request<unknown>(
      workspaceLibraryMaterializationsSuffix(workspaceId),
      {
        method: 'POST',
        headers: workspaceLibraryHeaders(effective),
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

// ---------------------------------------------------------------------------
// Use-case layer (migrated from the deleted legacy `resource.ts`).
//
// Pure helpers keep their behavior; transport closes over an explicit
// workspace id resolved from `features/workspace` (never duplicated here).
// Callers pass `MaybeRefOrGetter`-style sources; this module resolves them
// per call so workspace switches never use a stale id.

/** Cross-surface save identity: `provider:source_id`. */
export function libraryAssetKey(provider: string, sourceID: string): string {
  return `${provider}:${sourceID}`
}

/** Returns a bandwidth-bounded URL for Library cards without changing the saved original. */
export function resolveLibraryAssetCardURL(
  asset: Pick<WorkspaceLibraryAsset, 'provider' | 'previewUrl'>,
): string {
  const source = asset.previewUrl.trim()
  if (!source) return ''
  try {
    const url = new URL(source)
    if (asset.provider === 'unsplash' && url.hostname.endsWith('images.unsplash.com')) {
      url.searchParams.set('auto', 'format')
      url.searchParams.set('fit', 'crop')
      url.searchParams.set('w', '480')
      url.searchParams.set('q', '75')
    } else if (asset.provider === 'pexels' && url.hostname.endsWith('images.pexels.com')) {
      url.searchParams.set('auto', 'compress')
      url.searchParams.set('cs', 'tinysrgb')
      url.searchParams.set('w', '480')
    }
    return url.toString()
  } catch {
    return source
  }
}

/**
 * Provider reference handed between Search/Feed and Library. Snake_case is
 * the DnD wire contract (shared with search `domain/authAction`); the save
 * input below maps it to camelCase at this boundary.
 */
export type LibrarySearchReference = {
  provider: 'unsplash' | 'pexels'
  id: string
  source_url: string
  preview_url: string
  original_url: string
  title: string
  author: string
  width: number
  height: number
}

export function toWorkspaceLibraryAssetInput(
  reference: LibrarySearchReference,
  folderId?: string,
): WorkspaceLibraryAssetInput {
  if (folderId != null) assertLibraryFolderId(folderId)
  return {
    provider: reference.provider,
    sourceId: reference.id,
    sourceUrl: reference.source_url,
    previewUrl: reference.preview_url,
    originalUrl: reference.original_url,
    title: reference.title,
    author: reference.author,
    width: reference.width,
    height: reference.height,
    ...(folderId == null ? {} : { folderId }),
  }
}

/** Fail-closed workspace resolution: throws before any network call when unset. */
export function requireWorkspaceId(value: string | null | undefined): string {
  if (value == null || !value.trim()) throw new Error('Library workspace is unavailable.')
  return assertWorkspaceId(value)
}

const LIBRARY_FOLDER_PAGE_SIZE = 100
const LIBRARY_FOLDER_MAX_PAGES = 10

/** Loads every workspace folder (cursor loop, bounded) for folder-facts snapshots. */
export async function listAllWorkspaceLibraryFolders(
  workspaceId: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<WorkspaceLibraryFolder[]> {
  const folders: WorkspaceLibraryFolder[] = []
  let cursor: string | undefined
  for (let page = 0; page < LIBRARY_FOLDER_MAX_PAGES; page += 1) {
    const result = await workspaceLibraryApi.listFolders(workspaceId, {
      ...options,
      ...(cursor == null ? {} : { cursor }),
      pageSize: LIBRARY_FOLDER_PAGE_SIZE,
    })
    folders.push(...result.items)
    if (!result.hasMore || !result.nextCursor) return folders
    cursor = result.nextCursor
  }
  return folders
}

export type WorkspaceLibrarySavedKey = {
  provider: string
  sourceId: string
}

/**
 * Saved-state lookup for the Search feed: resolves references against saved
 * workspace assets only (nothing is fetched) and returns `provider:source_id`
 * keys for the resolved ones. Chunked to the backend batch bound.
 */
export async function lookupWorkspaceSavedAssetKeys(
  workspaceId: string,
  keys: WorkspaceLibrarySavedKey[],
  options: WorkspaceLibraryRequestOptions = {},
): Promise<Set<string>> {
  const saved = new Set<string>()
  for (let index = 0; index < keys.length; index += WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS) {
    const chunk = keys.slice(index, index + WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS)
    if (chunk.length === 0) continue
    const results = await workspaceLibraryApi.resolveAssets(
      workspaceId,
      chunk.map((key) => ({ provider: key.provider, sourceId: key.sourceId })),
      options,
    )
    for (const result of results) {
      if (result.status !== 'resolved' || !result.asset) continue
      saved.add(libraryAssetKey(result.asset.provider, result.asset.sourceId))
    }
  }
  return saved
}

/** Saves a Search reference; creates the folder first when a new name is given. */
export async function saveWorkspaceSearchReference(
  workspaceId: string,
  reference: LibrarySearchReference,
  folderId?: string,
  newFolderName?: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<WorkspaceLibraryAsset> {
  let targetFolderId = folderId
  const trimmedName = newFolderName?.trim()
  if (trimmedName) {
    const folder = await workspaceLibraryApi.createFolder(workspaceId, trimmedName, options)
    targetFolderId = folder.id
  }
  return workspaceLibraryApi.saveAsset(
    workspaceId,
    toWorkspaceLibraryAssetInput(reference, targetFolderId),
    options,
  )
}

/** Removes a saved Search reference (resolve, then delete the owned asset). */
export async function removeWorkspaceSearchReference(
  workspaceId: string,
  provider: 'unsplash' | 'pexels',
  sourceId: string,
  options: WorkspaceLibraryRequestOptions = {},
): Promise<void> {
  const results = await workspaceLibraryApi.resolveAssets(
    workspaceId,
    [{ provider, sourceId }],
    options,
  )
  const asset = results.find((result) => result.status === 'resolved' && result.asset)?.asset
  if (!asset) return
  await workspaceLibraryApi.deleteAsset(workspaceId, asset.id, options)
}

export type LibraryMaterializedMedia = {
  mediaAssetId: string
  url: string
  width: number
  height: number
}

export type LibraryAssetMaterializer = (assetId: string) => Promise<LibraryMaterializedMedia>

export type LibraryMaterializeOptions = WorkspaceLibraryRequestOptions & {
  pollAttempts?: number | undefined
  pollDelayMs?: number | undefined
}

function materializeDelay(ms: number, signal?: AbortSignal | null): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException('The request was aborted.', 'AbortError'))
      return
    }
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    function onAbort() {
      clearTimeout(timer)
      reject(new DOMException('The request was aborted.', 'AbortError'))
    }
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

/**
 * Turns an owned workspace Library asset into a managed Media asset for
 * Canvas: creates the materialization job, polls it to a terminal state,
 * then resolves display media through the shared delivery boundary.
 */
export async function materializeLibraryAssetForCanvas(
  workspaceId: string,
  assetId: string,
  options: LibraryMaterializeOptions = {},
): Promise<LibraryMaterializedMedia> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  const job = await workspaceLibraryApi.createMaterialization(workspaceId, assetId, options)
  const snapshot = await pollLibraryMaterialization(workspaceId, job.id, options)
  if (snapshot.status === 'failed') {
    throw new Error(snapshot.errorMessage?.trim() || fallback)
  }
  if (snapshot.status !== 'completed') throw new Error(fallback)
  return resolveMaterializedMedia(snapshot.mediaAssetId, options)
}

async function pollLibraryMaterialization(
  workspaceId: string,
  materializationId: string,
  options: LibraryMaterializeOptions,
): Promise<WorkspaceLibraryMaterialization> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  const attempts = options.pollAttempts ?? 20
  const delayMs = options.pollDelayMs ?? 1000
  let snapshot = await workspaceLibraryApi.getMaterialization(
    workspaceId,
    materializationId,
    options,
  )
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    if (snapshot.status === 'completed' || snapshot.status === 'failed') return snapshot
    await materializeDelay(delayMs, options.signal)
    snapshot = await workspaceLibraryApi.getMaterialization(workspaceId, materializationId, options)
  }
  if (snapshot.status === 'completed' || snapshot.status === 'failed') return snapshot
  throw new Error(fallback)
}

async function resolveMaterializedMedia(
  mediaAssetId: string | undefined,
  options: LibraryMaterializeOptions,
): Promise<LibraryMaterializedMedia> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  if (!mediaAssetId) throw new Error(fallback)
  const metadata = await getMediaAssetMetadata(mediaAssetId, {
    ...(options.signal == null ? {} : { signal: options.signal }),
  })
  const originalUrl = metadata.original?.url?.trim() ? metadata.original.url.trim() : ''
  const previewUrl = metadata.derivatives['large-preview']?.url?.trim()
    ? metadata.derivatives['large-preview'].url.trim()
    : ''
  const deliveryUrl = originalUrl || previewUrl
  if (!deliveryUrl) throw new Error(fallback)
  if (!Number.isFinite(metadata.width) || !Number.isFinite(metadata.height)) {
    throw new Error(fallback)
  }
  return {
    mediaAssetId,
    url: resolveAssetUrl(deliveryUrl),
    width: metadata.width,
    height: metadata.height,
  }
}
