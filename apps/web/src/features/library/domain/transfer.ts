import { isExternalImageProvider } from '../../../shared/domain/externalImageProviders'
import type { LibrarySearchReference, WorkspaceLibraryAsset } from './validators'
import { assertLibraryAssetId } from './validators'

export type LibraryAssetView = WorkspaceLibraryAsset

export const SEARCH_REFERENCE_MIME = 'application/x-ideaboard-search-reference'
export const LIBRARY_FOLDER_MOVE_MIME = 'application/x-ideaboard-library-folder-move'
export const CANVAS_LIBRARY_IMPORT_MIME = 'application/x-ideaboard-canvas-library-import'

type SearchReferencePayload = {
  type: 'search-reference'
  reference: LibrarySearchReference
}

type LibraryFolderMovePayload = {
  type: 'library-folder-move'
  assetId: string
}

export type CanvasLibraryImportDescriptor = {
  assetId: string
  title: string
  previewUrl: string
  width: number
  height: number
}

type CanvasLibraryImportPayload = {
  type: 'canvas-library-import'
  asset: CanvasLibraryImportDescriptor
}

function setPayload(dataTransfer: DataTransfer, mime: string, payload: unknown): void {
  dataTransfer.setData(mime, JSON.stringify(payload))
}

export function writeSearchReferenceTransfer(
  dataTransfer: DataTransfer,
  reference: LibrarySearchReference,
): void {
  setPayload(dataTransfer, SEARCH_REFERENCE_MIME, {
    type: 'search-reference',
    reference,
  } satisfies SearchReferencePayload)
  dataTransfer.effectAllowed = 'copy'
}

export function writeLibraryFolderMoveTransfer(dataTransfer: DataTransfer, assetId: string): void {
  setPayload(dataTransfer, LIBRARY_FOLDER_MOVE_MIME, {
    type: 'library-folder-move',
    assetId,
  } satisfies LibraryFolderMovePayload)
  dataTransfer.effectAllowed = 'copyMove'
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

export function writeCanvasLibraryImportTransfer(
  dataTransfer: DataTransfer,
  asset: CanvasLibraryImportDescriptor,
): void {
  setPayload(dataTransfer, CANVAS_LIBRARY_IMPORT_MIME, {
    type: 'canvas-library-import',
    asset,
  } satisfies CanvasLibraryImportPayload)
  dataTransfer.effectAllowed = 'copyMove'
}

function readPayload(dataTransfer: DataTransfer | null | undefined, mime: string): unknown {
  if (!dataTransfer) return null
  const raw = dataTransfer.getData(mime)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function isSearchReference(value: unknown): value is LibrarySearchReference {
  if (!value || typeof value !== 'object') return false
  const reference = value as Partial<LibrarySearchReference>
  return (
    isExternalImageProvider(reference.provider) &&
    typeof reference.id === 'string' &&
    typeof reference.source_url === 'string' &&
    typeof reference.preview_url === 'string' &&
    typeof reference.original_url === 'string'
  )
}

function isValidLibraryAssetID(value: unknown): value is string {
  if (typeof value !== 'string') return false
  try {
    assertLibraryAssetId(value)
    return true
  } catch {
    return false
  }
}

function isCanvasLibraryImportDescriptor(value: unknown): value is CanvasLibraryImportDescriptor {
  if (!value || typeof value !== 'object') return false
  const asset = value as Partial<CanvasLibraryImportDescriptor>
  return (
    isValidLibraryAssetID(asset.assetId) &&
    typeof asset.title === 'string' &&
    typeof asset.previewUrl === 'string' &&
    typeof asset.width === 'number' &&
    Number.isInteger(asset.width) &&
    asset.width >= 0 &&
    typeof asset.height === 'number' &&
    Number.isInteger(asset.height) &&
    asset.height >= 0
  )
}

export function readSearchReferenceTransfer(
  dataTransfer: DataTransfer | null | undefined,
): LibrarySearchReference | null {
  const payload = readPayload(dataTransfer, SEARCH_REFERENCE_MIME)
  if (payload && typeof payload === 'object' && 'reference' in payload) {
    const reference = (payload as Partial<SearchReferencePayload>).reference
    return isSearchReference(reference) ? reference : null
  }
  return null
}

export function readLibraryFolderMoveTransfer(
  dataTransfer: DataTransfer | null | undefined,
): string | null {
  const payload = readPayload(dataTransfer, LIBRARY_FOLDER_MOVE_MIME)
  if (
    payload &&
    typeof payload === 'object' &&
    (payload as Partial<LibraryFolderMovePayload>).type === 'library-folder-move' &&
    'assetId' in payload
  ) {
    const assetId = (payload as Partial<LibraryFolderMovePayload>).assetId
    return isValidLibraryAssetID(assetId) ? assetId : null
  }
  return null
}

export function readCanvasLibraryImportTransfer(
  dataTransfer: DataTransfer | null | undefined,
): CanvasLibraryImportDescriptor | null {
  const payload = readPayload(dataTransfer, CANVAS_LIBRARY_IMPORT_MIME)
  if (
    payload &&
    typeof payload === 'object' &&
    (payload as Partial<CanvasLibraryImportPayload>).type === 'canvas-library-import' &&
    'asset' in payload
  ) {
    const asset = (payload as Partial<CanvasLibraryImportPayload>).asset
    return isCanvasLibraryImportDescriptor(asset) ? asset : null
  }
  return null
}

export function getLibraryDropEffect(
  dataTransfer: DataTransfer | null | undefined,
): 'copy' | 'move' | 'none' {
  const types = Array.from(dataTransfer?.types ?? [])
  if (types.includes(LIBRARY_FOLDER_MOVE_MIME)) return 'move'
  if (types.includes(SEARCH_REFERENCE_MIME)) return 'copy'
  return 'none'
}
