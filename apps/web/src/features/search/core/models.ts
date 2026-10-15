import type { MediaDisplay } from '../../../core/media/mediaDisplay'
import { type ExternalImageProvider } from '../../../core/contracts/externalImageProviders'
export type SearchCatalogProvider = ExternalImageProvider

/** Search owns this model; generated transport types stay inside its adapter. */
export type SearchResult = {
  id: string
  provider: SearchCatalogProvider
  title: string
  description?: string
  author: string
  author_url: string
  source_url: string
  thumbnail_url: string
  preview_url: string
  original_url: string
  width: number
  height: number
  media_kind: 'image'
  display?: MediaDisplay | undefined
}

export type SearchPage = {
  query: string
  results: SearchResult[]
  next_cursor?: string
  has_more: boolean
}

export type VisionExternalReference = {
  provider: string
  id: string
  preview_url: string
}

export type VisionReference = {
  file?: File | undefined
  libraryAssetID?: string | undefined
  externalReference?: VisionExternalReference | undefined
  previewUrl: string
  key: string
  previewExpiresAt?: string | undefined
}
