import { resolveImageNodeFrameSize, type ImageNodeItem } from '../../../board'

export type CanvasLibraryImportMaterialization = Readonly<{
  mediaAssetId: string
  url: string
  width: number
  height: number
}>

export function createPendingLibraryImportFact(libraryAssetId: string) {
  return { state: 'pending' as const, libraryAssetId }
}

export function hasPendingLibraryImport(
  snapshot: ImageNodeItem | undefined,
  libraryAssetId: string,
): snapshot is ImageNodeItem & {
  libraryImport: Readonly<{ state: 'pending'; libraryAssetId: string }>
} {
  const fact = snapshot?.libraryImport
  return fact?.state === 'pending' && fact.libraryAssetId === libraryAssetId
}

export function retryLibraryImport(snapshot: ImageNodeItem): ImageNodeItem | null {
  const fact = snapshot.libraryImport
  if (fact?.state !== 'failed') return null
  // Legacy numeric facts cannot resume: the workspace API never accepts
  // integer ids (they converge to 404), so only opaque ids retry.
  if (typeof fact.libraryAssetId !== 'string') return null
  return {
    ...snapshot,
    libraryImport: createPendingLibraryImportFact(fact.libraryAssetId),
  }
}

export function failLibraryImport(
  snapshot: ImageNodeItem,
  libraryAssetId: string,
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
  libraryAssetId: string,
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
  // The workspace media identity (`asset_...`) has no numeric legacy form,
  // so no `sourceMediaAssetId` is recorded here: persisting one would fail
  // board snapshot validation. The node keeps working as a URL-addressed
  // image; a durable media link needs the board media-identity migration
  // (Phase 4 residual).
  const { libraryImport: _fact, ...readySnapshot } = snapshot
  return {
    ...readySnapshot,
    sourceMediaAssetId: undefined,
    width: resolveImageNodeFrameSize(ratio, true).width,
    sourceImageUrl: materialized.url.trim(),
    sourceImageRatio: ratio,
    outputStatus: 'ready',
    outputImages: [],
  }
}
