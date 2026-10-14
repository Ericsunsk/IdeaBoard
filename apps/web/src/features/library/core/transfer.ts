import type { WorkspaceLibraryAsset } from './validators'

export type LibraryAssetView = WorkspaceLibraryAsset

export type CanvasLibraryImportDescriptor = {
  assetId: string
  title: string
  previewUrl: string
  width: number
  height: number
}

export function toCanvasLibraryImportDescriptor(
  asset: WorkspaceLibraryAsset,
): CanvasLibraryImportDescriptor {
  return {
    assetId: asset.id,
    title: asset.title ?? '',
    previewUrl: asset.previewUrl,
    width: asset.width,
    height: asset.height,
  }
}
