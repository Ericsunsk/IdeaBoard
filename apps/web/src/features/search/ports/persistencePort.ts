import type { SearchResult, VisionReference } from '../core/models'

export type SearchFeedSnapshot = {
  query: string
  results: SearchResult[]
  nextCursor: string
  hasMore: boolean
  scrollY: number
}

export type SearchFeedPersistence = {
  read: (query: string, scope: string, allowReloadBypass: boolean) => SearchFeedSnapshot | null
  write: (snapshot: Omit<SearchFeedSnapshot, 'scrollY'>, scope: string) => void
  writeScroll: (scrollY: number, query: string, scope: string) => void
  scheduleWrite: (write: () => void) => () => void
}

export type CachedVisionReference = Pick<
  VisionReference,
  'key' | 'previewUrl' | 'libraryAssetID' | 'externalReference' | 'previewExpiresAt'
>

export type LibraryIslandState = { open: boolean; expanded: boolean }
