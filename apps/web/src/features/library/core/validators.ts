import { toMediaDisplay, type MediaDisplay } from '../../../core/media/mediaDisplay'
import {
  isExternalImageProvider,
  type ExternalImageProvider,
} from '../../../core/contracts/externalImageProviders'
// Workspace-scoped library domain (issue #844; split from `../workspace`).
//
// Pure opaque-id asserts, snake_case -> camelCase mappers, and save-input
// shaping. No i18n, no network, no generated code. Transport lives in
// `../session/resource`; save/lookup/resolve orchestration in
// `../session/libraryUseCases`. `../workspace` re-exports this module so the
// `features/library` barrel and `./workspace` deep imports stay stable.
//
// Id decision (tree truth wins): canonical `l*` everywhere. Backend
// `contract.MintPublicID` mints `lfolder_` / `lasset_`
// (`identifiers.go`, `openapi.public.yaml`, generated `LibraryFolderId` /
// `LibraryAssetId` models); zero `lib*` shapes exist anywhere in the tree.
// `lib*` spellings are rejected here. (The retired `lmat_`
// materialization job id is gone with the cut public job API; only
// `lfolder_` / `lasset_` remain.)

import {
  assertWorkspaceId as assertSharedWorkspaceId,
  requireWorkspaceId as requireSharedWorkspaceId,
} from '../../../core/utils/workspaceId'
import { opaqueIdPattern } from '../../../core/utils/opaqueIds'
import { isRecord } from '../../../core/utils/guards'

const LIBRARY_ID_RE = opaqueIdPattern(['lfolder', 'lasset'])

function isLibraryId(value: unknown): value is string {
  return typeof value === 'string' && LIBRARY_ID_RE.test(value)
}

function libraryIdPrefix(value: string): string {
  return value.slice(0, value.indexOf('_'))
}

const LIBRARY_FOLDER_PREFIXES = ['lfolder'] as const
const LIBRARY_ASSET_PREFIXES = ['lasset'] as const

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
  // Intentional barrel pass-through (no message change): Canvas/Search must
  // resolve workspace ids through the library barrel, never `shared/` deep
  // imports; the canonical primitive stays `core/utils/workspaceId`.
  return assertSharedWorkspaceId(value)
}

export function assertLibraryFolderId(value: string): string {
  return assertLibraryId(value, LIBRARY_FOLDER_PREFIXES, 'library folder id', 'lfolder')
}

export function assertLibraryAssetId(value: string): string {
  return assertLibraryId(value, LIBRARY_ASSET_PREFIXES, 'library asset id', 'lasset')
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

export type WorkspaceLibraryAssetProvider = ExternalImageProvider

export function mapWorkspaceLibraryProvider(value: unknown): WorkspaceLibraryAssetProvider {
  if (isExternalImageProvider(value)) return value
  throw new Error('Invalid library asset payload: provider')
}

export type WorkspaceLibraryExternalAsset = {
  source: 'external'
  id: string
  workspaceId: string
  folderId?: string | undefined
  provider: WorkspaceLibraryAssetProvider
  sourceId: string
  sourceUrl: string
  previewUrl: string
  originalUrl: string
  title?: string | undefined
  /** Display-only attribution; never save identity. */
  author?: string | undefined
  width: number
  height: number
  createdAt: string
  updatedAt: string
  display?: MediaDisplay | undefined
}

export type WorkspaceLibraryMediaAsset = {
  source: 'media'
  id: string
  workspaceId: string
  folderId?: string | undefined
  mediaAssetId: string
  title?: string | undefined
  /** Display-only attribution; never save identity. */
  author?: string | undefined
  /** Paired: both numbers when known, both null when unknown. */
  width: number | null
  /** Paired: both numbers when known, both null when unknown. */
  height: number | null
  createdAt: string
  updatedAt: string
  display?: MediaDisplay | undefined
}

/**
 * Library Source: `source` discriminates
 * external provider references from managed media. Media rows carry the Media
 * Asset identity with no URL keys; task lineage (`task_id`,
 * `task_result_index`) and operator identity are provenance/server-internal
 * and never part of the read identity.
 */
export type WorkspaceLibraryAsset = WorkspaceLibraryExternalAsset | WorkspaceLibraryMediaAsset

function mapWorkspaceLibrarySource(value: unknown): 'external' | 'media' {
  if (value === 'external' || value === 'media') return value
  throw new Error('Invalid library asset payload: source')
}

function assertMediaAssetId(value: string): string {
  const trimmed = value.trim()
  if (!opaqueIdPattern(['asset']).test(trimmed)) {
    throw new Error('Invalid library asset payload: expected media asset_...')
  }
  return trimmed
}

function requiredMediaAssetId(raw: unknown): string {
  if (typeof raw !== 'string' || !raw.trim()) {
    throw new Error('Invalid library asset payload: media_asset_id')
  }
  return assertMediaAssetId(raw)
}

/** Paired nullable dimensions: both numbers when known, both null when unknown. */
function requiredPairedDimensions(raw: Record<string, unknown>): {
  width: number | null
  height: number | null
} {
  const { width, height } = raw
  if (width === null && height === null) return { width: null, height: null }
  if (typeof width === 'number' && typeof height === 'number') {
    if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
      throw new Error('Invalid library asset payload: width/height')
    }
    return { width, height }
  }
  throw new Error('Invalid library asset payload: width/height must pair number|null')
}

function toWorkspaceLibraryMediaAsset(
  raw: Record<string, unknown>,
  shared: Pick<WorkspaceLibraryMediaAsset, 'id' | 'workspaceId' | 'folderId'>,
): WorkspaceLibraryMediaAsset {
  // Media rows must not carry external URL/provider keys, even empty ones.
  for (const key of ['provider', 'source_id', 'source_url', 'preview_url', 'original_url']) {
    if (raw[key] !== undefined) {
      throw new Error(`Invalid library asset payload: media must not carry ${key}`)
    }
  }
  const { width, height } = requiredPairedDimensions(raw)
  return {
    source: 'media',
    ...shared,
    mediaAssetId: requiredMediaAssetId(raw.media_asset_id),
    title: optionalString(raw.title),
    author: optionalString(raw.author),
    width,
    height,
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
    display: toMediaDisplay(raw.display),
  }
}

function toWorkspaceLibraryExternalAsset(
  raw: Record<string, unknown>,
  shared: Pick<WorkspaceLibraryExternalAsset, 'id' | 'workspaceId' | 'folderId'>,
): WorkspaceLibraryExternalAsset {
  const width = requiredNumber(raw.width, 'width')
  const height = requiredNumber(raw.height, 'height')
  if (width < 0 || height < 0) throw new Error('Invalid library asset payload: width/height')
  return {
    source: 'external',
    ...shared,
    provider: mapWorkspaceLibraryProvider(raw.provider),
    sourceId: requiredString(raw.source_id, 'source_id'),
    sourceUrl: requiredString(raw.source_url, 'source_url'),
    previewUrl: requiredString(raw.preview_url, 'preview_url'),
    originalUrl: requiredString(raw.original_url, 'original_url'),
    title: optionalString(raw.title),
    author: optionalString(raw.author),
    width,
    height,
    createdAt: requiredString(raw.created_at, 'created_at'),
    updatedAt: requiredString(raw.updated_at, 'updated_at'),
    display: toMediaDisplay(raw.display),
  }
}

export function toWorkspaceLibraryAsset(raw: unknown): WorkspaceLibraryAsset {
  if (!isRecord(raw)) throw new Error('Invalid library asset payload')
  const folderId = optionalString(raw.folder_id)
  if (folderId != null) assertLibraryFolderId(folderId)
  const shared = {
    id: assertLibraryAssetId(requiredString(raw.id, 'id')),
    workspaceId: assertWorkspaceId(requiredString(raw.workspace_id, 'workspace_id')),
    folderId,
  }
  // `source` is required: backend always emits the discriminator and rejects
  // a missing source with 422; absent `source` fails closed here.
  const source = mapWorkspaceLibrarySource(raw.source)
  if (source === 'media') return toWorkspaceLibraryMediaAsset(raw, shared)
  return toWorkspaceLibraryExternalAsset(raw, shared)
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

export type WorkspaceLibraryPage<T> = {
  items: T[]
  nextCursor: string
  hasMore: boolean
}

export function toWorkspaceLibraryPage<T>(
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

export type WorkspaceLibraryExternalAssetInput = {
  source: 'external'
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

export type WorkspaceLibraryMediaAssetInput = {
  source: 'media'
  mediaAssetId: string
  title?: string | undefined
  folderId?: string | undefined
}

/**
 * Discriminated save input matching the wire `source`: external references
 * carry provider/source identity plus URLs, media saves carry only the
 * uploaded Media Asset identity (dimensions project server-side; no URL
 * keys, no task lineage, no operator identity, author display-only).
 */
export type WorkspaceLibraryAssetInput =
  WorkspaceLibraryExternalAssetInput | WorkspaceLibraryMediaAssetInput

/** Save identity across surfaces: external `provider:source_id`, media `media:<id>`. */
export type LibraryAssetIdentity =
  | { source: 'external'; provider: string; sourceId: string }
  | { source: 'media'; mediaAssetId: string }

export function libraryAssetKeyFor(identity: LibraryAssetIdentity): string {
  return identity.source === 'media'
    ? `media:${identity.mediaAssetId}`
    : `${identity.provider}:${identity.sourceId}`
}

/**
 * Provider reference handed between Search/Feed and Library. Snake_case is
 * the DnD wire contract (shared with search `domain/authAction`); the save
 * input below maps it to camelCase at this boundary.
 */
export type LibrarySearchReference = {
  provider: ExternalImageProvider
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
): WorkspaceLibraryExternalAssetInput {
  if (folderId != null) assertLibraryFolderId(folderId)
  return {
    source: 'external',
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

export function toWorkspaceLibraryMediaAssetInput(
  mediaAssetId: string,
  meta: { title?: string | undefined } = {},
  folderId?: string,
): WorkspaceLibraryMediaAssetInput {
  if (folderId != null) assertLibraryFolderId(folderId)
  return {
    source: 'media',
    mediaAssetId: assertMediaAssetId(mediaAssetId),
    ...(meta.title == null || !meta.title.trim() ? {} : { title: meta.title }),
    ...(folderId == null ? {} : { folderId }),
  }
}

/** Fail-closed workspace resolution: throws before any network call when unset. */
export function requireWorkspaceId(value: string | null | undefined): string {
  return assertWorkspaceId(requireSharedWorkspaceId(value, 'Library workspace is unavailable.'))
}
