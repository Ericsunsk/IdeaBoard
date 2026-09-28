// Workspace-scoped library domain (issue #844; split from `../workspace`).
//
// Pure opaque-id asserts, snake_case -> camelCase mappers, and save-input
// shaping. No i18n, no network, no generated code. Transport lives in
// `../infrastructure/resource`; save/lookup/resolve orchestration in
// `../application/libraryUseCases`. `../workspace` re-exports this module so the
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
} from '../../../shared/utils/workspaceId'
import { opaqueIdPattern } from '../../../shared/utils/opaqueIds'

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
  // imports; the canonical primitive stays `shared/utils/workspaceId`.
  return assertSharedWorkspaceId(value)
}

export function assertLibraryFolderId(value: string): string {
  return assertLibraryId(value, LIBRARY_FOLDER_PREFIXES, 'library folder id', 'lfolder')
}

export function assertLibraryAssetId(value: string): string {
  return assertLibraryId(value, LIBRARY_ASSET_PREFIXES, 'library asset id', 'lasset')
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
  return assertWorkspaceId(requireSharedWorkspaceId(value, 'Library workspace is unavailable.'))
}
