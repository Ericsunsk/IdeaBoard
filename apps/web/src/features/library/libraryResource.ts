import {
  createLibraryFolder,
  deleteLibraryAsset,
  deleteLibraryFolder,
  listLibraryAssets,
  listLibraryFolders,
  lookupLibraryAssets,
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

export type LibraryFolderView = LibraryFolder
export type LibraryAssetView = LibraryAsset

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
  return resolveApiData(listLibraryFolders(), '加载收藏夹失败').then((data) => data.folders)
}

export function getLibraryAssets(params: ListLibraryAssetsParams): Promise<{
  items: LibraryAsset[]
  nextCursor: string
  hasMore: boolean
}> {
  return resolveApiData(listLibraryAssets(params), '加载素材失败').then((data) => ({
    items: data.items,
    nextCursor: data.next_cursor ?? '',
    hasMore: data.has_more,
  }))
}

export function createFolder(name: string): Promise<LibraryFolder> {
  return resolveApiData(createLibraryFolder({ name }), '创建收藏夹失败').then((data) => data)
}

export function renameFolder(folderId: number, name: string): Promise<LibraryFolder> {
  return resolveApiData(renameLibraryFolder(folderId, { name }), '重命名收藏夹失败')
}

export function saveAsset(asset: LibraryAssetInput, folderId?: number, newFolderName?: string) {
  return resolveApiData(
    saveLibraryAsset({ asset, folder_id: folderId, new_folder_name: newFolderName }),
    '保存素材失败',
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
  return resolveApiData(moveLibraryAsset(assetId, { folder_id: folderId }), '移动素材失败')
}

export function removeAsset(assetId: number): Promise<void> {
  return resolveApiOk(deleteLibraryAsset(assetId), '移除素材失败')
}

export function removeFolder(folderId: number): Promise<void> {
  return resolveApiOk(deleteLibraryFolder(folderId), '删除收藏夹失败')
}

export async function lookupSavedAssets(
  keys: Array<{ provider: 'unsplash' | 'pexels'; source_id: string }>,
): Promise<LibraryAsset[]> {
  return resolveApiData(lookupLibraryAssets({ keys }), '查询收藏状态失败').then((data) => data.items)
}

export async function lookupSavedAssetKeys(
  keys: Array<{ provider: 'unsplash' | 'pexels'; source_id: string }>,
): Promise<Set<string>> {
  const assets = await lookupSavedAssets(keys)
  return new Set(assets.map((asset) => libraryAssetKey(asset.provider, asset.source_id)))
}
