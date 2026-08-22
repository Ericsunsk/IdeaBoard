import type { LibraryAssetView, LibrarySearchReference } from './libraryResource'

export const SEARCH_REFERENCE_MIME = 'application/x-ideaboard-search-reference'
export const LIBRARY_ASSET_MIME = 'application/x-ideaboard-library-asset'

type SearchReferencePayload = {
  type: 'search-reference'
  reference: LibrarySearchReference
}

type LibraryAssetPayload = {
  type: 'library-asset'
  asset: LibraryAssetView
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

export function writeLibraryAssetTransfer(
  dataTransfer: DataTransfer,
  asset: LibraryAssetView,
): void {
  setPayload(dataTransfer, LIBRARY_ASSET_MIME, {
    type: 'library-asset',
    asset,
  } satisfies LibraryAssetPayload)
  dataTransfer.effectAllowed = 'move'
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
    (reference.provider === 'unsplash' || reference.provider === 'pexels') &&
    typeof reference.id === 'string' &&
    typeof reference.source_url === 'string' &&
    typeof reference.preview_url === 'string' &&
    typeof reference.original_url === 'string'
  )
}

function isLibraryAsset(value: unknown): value is LibraryAssetView {
  if (!value || typeof value !== 'object') return false
  const asset = value as Partial<LibraryAssetView>
  return (
    typeof asset.id === 'number' &&
    (asset.provider === 'unsplash' || asset.provider === 'pexels') &&
    typeof asset.source_id === 'string' &&
    typeof asset.preview_url === 'string'
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

export function readLibraryAssetTransfer(
  dataTransfer: DataTransfer | null | undefined,
): LibraryAssetView | null {
  const payload = readPayload(dataTransfer, LIBRARY_ASSET_MIME)
  if (payload && typeof payload === 'object' && 'asset' in payload) {
    const asset = (payload as Partial<LibraryAssetPayload>).asset
    return isLibraryAsset(asset) ? asset : null
  }
  return null
}

export function getLibraryDropEffect(
  dataTransfer: DataTransfer | null | undefined,
): 'copy' | 'move' | 'none' {
  const types = Array.from(dataTransfer?.types ?? [])
  if (types.includes(LIBRARY_ASSET_MIME)) return 'move'
  if (types.includes(SEARCH_REFERENCE_MIME)) return 'copy'
  return 'none'
}
