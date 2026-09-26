import {
  createWorkspaceLibraryFolder as createWorkspaceLibraryFolderTransport,
  createWorkspaceLibraryMaterialization as createWorkspaceLibraryMaterializationTransport,
  deleteWorkspaceLibraryAsset as deleteWorkspaceLibraryAssetTransport,
  deleteWorkspaceLibraryFolder as deleteWorkspaceLibraryFolderTransport,
  getWorkspaceLibraryMaterialization as getWorkspaceLibraryMaterializationTransport,
  listWorkspaceLibraryAssets as listWorkspaceLibraryAssetsTransport,
  listWorkspaceLibraryFolders as listWorkspaceLibraryFoldersTransport,
  moveWorkspaceLibraryAsset as moveWorkspaceLibraryAssetTransport,
  renameWorkspaceLibraryFolder as renameWorkspaceLibraryFolderTransport,
  resolveWorkspaceLibraryAssets as resolveWorkspaceLibraryAssetsTransport,
  saveWorkspaceLibraryAsset as saveWorkspaceLibraryAssetTransport,
} from '../../shared/api/generated-query/public/workspace-library/workspace-library'
import {
  buildContractRequestHeaders,
  requireContractData,
} from '../../shared/api/contractKernel'
import { getWorkspaceMediaAssetMetadata } from '../../shared/api/mediaAssetDelivery'
import { resolveAssetUrl } from '../../shared/config/runtimeUrls'
import { createUniqueId } from '../../shared/utils/ids'
import { i18n } from '../../shared/i18n'

// Workspace-scoped library resource (issue #806).
//
// Talks to the v1 endpoints under `/api/v1/workspaces/{id}/library`
// through the generated `public/workspace-library` hooks (`/api/v1/...`
// via `publicMutator`). Pages and components must consume this module,
// never generated code directly.
//
// This module is the SOLE library transport: the legacy user-scoped
// `resource.ts` (integer ids, `/user/library/*`) is deleted. Ids are
// opaque (`ws_` + canonical `lfolder_` / `lasset_` / `lmat_`);
// malformed ids throw before any network call, matching the backend 404
// posture. Payloads map snake_case to camelCase at this boundary;
// callers only see camelCase.
//
// Transport note: the generated hooks own routes/envelopes; this adapter
// keeps the opaque-id asserts, auto idempotency minting, resolve batch
// bound, cursor loops, and snake_case -> camelCase validators that have
// no generated equivalent.
//
// Id decision (tree truth wins): canonical `l*` everywhere. Backend
// `contract.MintPublicID` mints `lfolder_` / `lasset_` / `lmat_`
// (`identifiers.go`, `openapi.public.yaml`, generated `LibraryFolderId` /
// `LibraryAssetId` / `LibraryMaterializationId` models); zero `lib*`
// shapes exist anywhere in the tree. `lib*` spellings are rejected here.
// Display media for materialized assets resolves through the shared
// workspace media delivery boundary with the caller's explicit workspace.
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

function toGeneratedOptions(options: WorkspaceLibraryRequestOptions): {
  headers: Record<string, string>
  signal?: AbortSignal | undefined
} {
  return {
    headers: workspaceLibraryHeaders(options),
    ...(options.signal == null ? {} : { signal: options.signal }),
  }
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
  page: { data: unknown; meta?: { next_cursor?: string | null; has_more?: boolean } | null },
  mapItem: (raw: unknown) => T,
  fallback: string,
): WorkspaceLibraryPage<T> {
  if (page == null || !Array.isArray((page as { data: unknown }).data)) throw new Error(fallback)
  const meta = page.meta ?? {}
  const nextCursor = typeof meta.next_cursor === 'string' ? meta.next_cursor : ''
  return {
    items: (page.data as unknown[]).map(mapItem),
    nextCursor,
    hasMore: meta.has_more === true,
  }
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
    const query: { cursor?: string; page_size?: number } = {}
    if (options.cursor) query.cursor = options.cursor
    if (options.pageSize != null) query.page_size = options.pageSize
    const page = await listWorkspaceLibraryFoldersTransport(
      assertWorkspaceId(workspaceId),
      query,
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryPage(page, toWorkspaceLibraryFolder, i18n.global.t('library.errors.loadFoldersFailed'))
  },

  createFolder: async (
    workspaceId: string,
    name: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryFolder> => {
    const effective = withLibraryIdempotency(options, 'library:folder-create')
    const envelope = await createWorkspaceLibraryFolderTransport(
      assertWorkspaceId(workspaceId),
      { data: { name } },
      toGeneratedOptions(effective),
    )
    return toWorkspaceLibraryFolder(
      requireContractData<Record<string, unknown>>(
        envelope,
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
    const envelope = await renameWorkspaceLibraryFolderTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryFolderId(folderId),
      { data: { name } },
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryFolder(
      requireContractData<Record<string, unknown>>(
        envelope,
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
    await deleteWorkspaceLibraryFolderTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryFolderId(folderId),
      toGeneratedOptions(effective),
    )
  },

  listAssets: async (
    workspaceId: string,
    options: WorkspaceLibraryRequestOptions & {
      cursor?: string
      pageSize?: number
      folderId?: string
    } = {},
  ): Promise<WorkspaceLibraryPage<WorkspaceLibraryAsset>> => {
    const query: { cursor?: string; page_size?: number; folder_id?: string } = {}
    if (options.cursor) query.cursor = options.cursor
    if (options.pageSize != null) query.page_size = options.pageSize
    if (options.folderId) query.folder_id = assertLibraryFolderId(options.folderId)
    const page = await listWorkspaceLibraryAssetsTransport(
      assertWorkspaceId(workspaceId),
      query,
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryPage(page, toWorkspaceLibraryAsset, i18n.global.t('library.errors.loadAssetsFailed'))
  },

  saveAsset: async (
    workspaceId: string,
    input: WorkspaceLibraryAssetInput,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryAsset> => {
    const effective = withLibraryIdempotency(options, 'library:asset-save')
    const data: { provider: 'unsplash' | 'pexels'; source_id: string } & Record<string, unknown> = {
      provider: input.provider,
      source_id: input.sourceId,
    }
    if (input.sourceUrl != null) data.source_url = input.sourceUrl
    if (input.previewUrl != null) data.preview_url = input.previewUrl
    if (input.originalUrl != null) data.original_url = input.originalUrl
    if (input.title != null) data.title = input.title
    if (input.author != null) data.author = input.author
    if (input.width != null) data.width = input.width
    if (input.height != null) data.height = input.height
    if (input.folderId != null) data.folder_id = assertLibraryFolderId(input.folderId)
    const envelope = await saveWorkspaceLibraryAssetTransport(
      assertWorkspaceId(workspaceId),
      { data },
      toGeneratedOptions(effective),
    )
    return toWorkspaceLibraryAsset(
      requireContractData<Record<string, unknown>>(
        envelope,
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
    const envelope = await moveWorkspaceLibraryAssetTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryAssetId(assetId),
      // Wire-identical to the retired transport: null rides as '' (unfiled).
      { data: { folder_id: folderId ?? '' } },
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryAsset(
      requireContractData<Record<string, unknown>>(
        envelope,
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
    await deleteWorkspaceLibraryAssetTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryAssetId(assetId),
      toGeneratedOptions(effective),
    )
  },

  resolveAssets: async (
    workspaceId: string,
    items: Array<{ provider: string; sourceId?: string; sourceUrl?: string }>,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryResolveResult[]> => {
    if (items.length > WORKSPACE_LIBRARY_RESOLVE_MAX_ITEMS) {
      throw new Error(i18n.global.t('library.errors.lookupStatusFailed'))
    }
    const envelope = await resolveWorkspaceLibraryAssetsTransport(
      assertWorkspaceId(workspaceId),
      {
        data: {
          items: items.map((item) => {
            const entry: { provider: string } & Record<string, unknown> = {
              provider: item.provider,
            }
            if (item.sourceId != null) entry.source_id = item.sourceId
            if (item.sourceUrl != null) entry.source_url = item.sourceUrl
            return entry as { provider: 'unsplash' | 'pexels'; source_id?: string; source_url?: string }
          }),
        },
      },
      toGeneratedOptions(options),
    )
    const result = requireContractData<{ items: unknown }>(
      envelope,
      i18n.global.t('library.errors.lookupStatusFailed'),
    )
    if (!Array.isArray(result.items)) throw new Error(i18n.global.t('library.errors.lookupStatusFailed'))
    return result.items.map(toWorkspaceLibraryResolveResult)
  },

  createMaterialization: async (
    workspaceId: string,
    libraryAssetId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryMaterialization> => {
    const effective = withLibraryIdempotency(options, 'library:materialize')
    const envelope = await createWorkspaceLibraryMaterializationTransport(
      assertWorkspaceId(workspaceId),
      { data: { library_asset_id: assertLibraryAssetId(libraryAssetId) } },
      toGeneratedOptions(effective),
    )
    return toWorkspaceLibraryMaterialization(
      requireContractData<Record<string, unknown>>(
        envelope,
        i18n.global.t('library.errors.importAssetFailed'),
      ),
    )
  },

  getMaterialization: async (
    workspaceId: string,
    materializationId: string,
    options: WorkspaceLibraryRequestOptions = {},
  ): Promise<WorkspaceLibraryMaterialization> => {
    const envelope = await getWorkspaceLibraryMaterializationTransport(
      assertWorkspaceId(workspaceId),
      assertLibraryMaterializationId(materializationId),
      toGeneratedOptions(options),
    )
    return toWorkspaceLibraryMaterialization(
      requireContractData<Record<string, unknown>>(
        envelope,
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
  return resolveMaterializedMedia(workspaceId, snapshot.mediaAssetId, options)
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
  workspaceId: string,
  mediaAssetId: string | undefined,
  options: LibraryMaterializeOptions,
): Promise<LibraryMaterializedMedia> {
  const fallback = i18n.global.t('library.errors.importAssetFailed')
  if (!mediaAssetId) throw new Error(fallback)
  const metadata = await getWorkspaceMediaAssetMetadata(workspaceId, mediaAssetId, {
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
