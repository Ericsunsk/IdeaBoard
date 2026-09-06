import {
  createLibraryFolder,
  deleteLibraryAsset,
  deleteLibraryFolder,
  listLibraryAssets,
  listLibraryFolders,
  lookupLibraryAssets,
  materializeLibraryAsset,
  moveLibraryAsset,
  renameLibraryFolder,
  saveLibraryAsset,
} from '../../shared/api/generated-query/user-library/user-library'
import type {
  LibraryAsset,
  LibraryAssetInput,
  LibraryFolder,
  ListLibraryAssetsParams,
} from '../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../shared/api/apiResponse'
import { i18n } from '../../shared/i18n'

export type LibraryFolderView = LibraryFolder
export type LibraryAssetView = LibraryAsset
export type LibraryMaterializedMedia = {
  mediaAssetId: number
  url: string
  width: number
  height: number
}
export type LibraryAssetMaterializer = (assetId: number) => Promise<LibraryMaterializedMedia>

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

export function libraryAssetKey(provider: string, sourceID: string): string {
  return `${provider}:${sourceID}`
}

/** Returns a bandwidth-bounded URL for Library cards without changing the saved original. */
export function resolveLibraryAssetCardURL(
  asset: Pick<LibraryAsset, 'provider' | 'preview_url'>,
): string {
  const source = asset.preview_url.trim()
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

export function toLibraryAssetInput(reference: LibrarySearchReference): LibraryAssetInput {
  return {
    provider: reference.provider,
    source_id: reference.id,
    source_url: reference.source_url,
    preview_url: reference.preview_url,
    original_url: reference.original_url,
    title: reference.title,
    author: reference.author,
    width: reference.width,
    height: reference.height,
  }
}

export function getLibraryFolders(): Promise<LibraryFolder[]> {
  return resolveApiData(
    listLibraryFolders(),
    i18n.global.t('library.errors.loadFoldersFailed'),
  ).then((data) => data.folders)
}

export function getLibraryAssets(params: ListLibraryAssetsParams): Promise<{
  items: LibraryAsset[]
  nextCursor: string
  hasMore: boolean
}> {
  return resolveApiData(
    listLibraryAssets(params),
    i18n.global.t('library.errors.loadAssetsFailed'),
  ).then((data) => ({
    items: data.items,
    nextCursor: data.next_cursor ?? '',
    hasMore: data.has_more,
  }))
}

export function createFolder(name: string): Promise<LibraryFolder> {
  return resolveApiData(
    createLibraryFolder({ name }),
    i18n.global.t('library.errors.createFolderFailed'),
  ).then((data) => data)
}

export function renameFolder(folderId: number, name: string): Promise<LibraryFolder> {
  return resolveApiData(
    renameLibraryFolder(folderId, { name }),
    i18n.global.t('library.errors.renameFolderFailed'),
  )
}

function saveAsset(asset: LibraryAssetInput, folderId?: number, newFolderName?: string) {
  return resolveApiData(
    saveLibraryAsset({ asset, folder_id: folderId, new_folder_name: newFolderName }),
    i18n.global.t('library.errors.saveAssetFailed'),
  )
}

export function saveSearchReference(
  reference: LibrarySearchReference,
  folderId?: number,
  newFolderName?: string,
): Promise<LibraryAsset> {
  return saveAsset(toLibraryAssetInput(reference), folderId, newFolderName)
}

export function moveAsset(assetId: number, folderId: number): Promise<LibraryAsset> {
  return resolveApiData(
    moveLibraryAsset(assetId, { folder_id: folderId }),
    i18n.global.t('library.errors.moveAssetFailed'),
  )
}

export function removeAsset(assetId: number): Promise<void> {
  return resolveApiOk(
    deleteLibraryAsset(assetId),
    i18n.global.t('library.errors.removeAssetFailed'),
  )
}

export function removeFolder(folderId: number): Promise<void> {
  return resolveApiOk(
    deleteLibraryFolder(folderId),
    i18n.global.t('library.errors.deleteFolderFailed'),
  )
}

/** Turns an owned URL-backed Library asset into a managed Media asset for Canvas. */
export function materializeLibraryAssetForCanvas(
  assetId: number,
): Promise<LibraryMaterializedMedia> {
  return resolveApiData(
    materializeLibraryAsset(assetId),
    i18n.global.t('library.errors.importAssetFailed'),
  ).then((data) => ({
    mediaAssetId: data.media_asset_id,
    url: data.url,
    width: data.width,
    height: data.height,
  }))
}

export async function removeSearchReference(
  provider: 'unsplash' | 'pexels',
  sourceId: string,
): Promise<void> {
  const assets = await lookupSavedAssets([{ provider, source_id: sourceId }])
  if (assets.length > 0 && assets[0]?.id) {
    await removeAsset(assets[0].id)
  }
}

async function lookupSavedAssets(
  keys: Array<{ provider: 'unsplash' | 'pexels'; source_id: string }>,
): Promise<LibraryAsset[]> {
  return resolveApiData(
    lookupLibraryAssets({ keys }),
    i18n.global.t('library.errors.lookupStatusFailed'),
  ).then((data) => data.items)
}

export async function lookupSavedAssetKeys(
  keys: Array<{ provider: 'unsplash' | 'pexels'; source_id: string }>,
): Promise<Set<string>> {
  const assets = await lookupSavedAssets(keys)
  return new Set(assets.map((asset) => libraryAssetKey(asset.provider, asset.source_id)))
}
