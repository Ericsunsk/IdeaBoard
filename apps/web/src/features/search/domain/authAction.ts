import type { LibrarySearchReference } from '../../library'

export type { LibrarySearchReference } from '../../library'

export type SearchAuthAction =
  | { kind: 'text-search'; query: string }
  | { kind: 'vision-search'; query: string }
  | { kind: 'save-reference'; reference: LibrarySearchReference }
  | { kind: 'save-batch'; references: LibrarySearchReference[] }
  | { kind: 'open-library' }
