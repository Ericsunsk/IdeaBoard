import { resolveImageNodeFrameSize, type ImageNodeItem } from '../../canvas-board/domain'

export type CanvasLibraryImportMaterialization = Readonly<{
  mediaAssetId: number
  url: string
  width: number
  height: number
}>

export function createPendingLibraryImportFact(libraryAssetId: number) {
  return { state: 'pending' as const, libraryAssetId }
}

export function hasPendingLibraryImport(
  snapshot: ImageNodeItem | undefined,
  libraryAssetId: number,
): snapshot is ImageNodeItem & {
  libraryImport: Readonly<{ state: 'pending'; libraryAssetId: number }>
} {
  const fact = snapshot?.libraryImport
  return fact?.state === 'pending' && fact.libraryAssetId === libraryAssetId
}

export function retryLibraryImport(snapshot: ImageNodeItem): ImageNodeItem | null {
  const fact = snapshot.libraryImport
  if (fact?.state !== 'failed') return null
  return {
    ...snapshot,
    libraryImport: createPendingLibraryImportFact(fact.libraryAssetId),
  }
}

export function failLibraryImport(
  snapshot: ImageNodeItem,
  libraryAssetId: number,
  failureMessage: string,
): ImageNodeItem | null {
  if (!hasPendingLibraryImport(snapshot, libraryAssetId)) return null
  return {
    ...snapshot,
    libraryImport: { state: 'failed', libraryAssetId, failureMessage },
  }
}

export function materializeLibraryImport(
  snapshot: ImageNodeItem,
  libraryAssetId: number,
  materialized: CanvasLibraryImportMaterialization,
  fallbackRatio: number,
): ImageNodeItem | null {
  if (!hasPendingLibraryImport(snapshot, libraryAssetId)) return null
  const width = Number(materialized.width)
  const height = Number(materialized.height)
  const ratio =
    Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
      ? height / width
      : fallbackRatio
  const { libraryImport: _fact, ...readySnapshot } = snapshot
  return {
    ...readySnapshot,
    width: resolveImageNodeFrameSize(ratio, true).width,
    sourceImageUrl: materialized.url.trim(),
    sourceMediaAssetId: materialized.mediaAssetId,
    sourceImageRatio: ratio,
    outputStatus: 'ready',
    outputImages: [],
  }
}
