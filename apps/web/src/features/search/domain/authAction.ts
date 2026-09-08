import type { SearchResult } from './models'

export type LibrarySearchReference = Pick<
  SearchResult,
  | 'provider'
  | 'id'
  | 'source_url'
  | 'preview_url'
  | 'original_url'
  | 'title'
  | 'author'
  | 'width'
  | 'height'
>

export type SearchAuthAction =
  | { kind: 'text-search'; query: string }
  | { kind: 'vision-search'; query: string }
  | { kind: 'save-reference'; reference: LibrarySearchReference }
  | { kind: 'save-batch'; references: LibrarySearchReference[] }
  | { kind: 'open-library' }
