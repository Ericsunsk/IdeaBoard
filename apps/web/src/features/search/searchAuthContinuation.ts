import type { LibrarySearchReference } from '../library/libraryResource'
import type { VisionReference } from './searchResource'

const ANONYMOUS_QUERY_GATE_KEY = 'ideaboard:search:anonymous-query-gate'
const PENDING_ACTION_KEY = 'ideaboard:search:pending-auth-action'
export const MAX_ANONYMOUS_SEARCH_QUERIES = 5

export type SearchAuthAction =
  | { kind: 'text-search'; query: string }
  | { kind: 'vision-search'; references: VisionReference[]; query: string }
  | { kind: 'save-reference'; reference: LibrarySearchReference }
  | { kind: 'save-batch'; references: LibrarySearchReference[] }
  | { kind: 'open-library' }

let pendingAction: SearchAuthAction | null = null

function storage(): Storage | null {
  if (typeof window === 'undefined') return null
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

function isPersistableAction(action: SearchAuthAction): boolean {
  return action.kind !== 'vision-search'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object'
}

function isLibrarySearchReference(value: unknown): value is LibrarySearchReference {
  if (!isRecord(value)) return false
  return (
    (value.provider === 'unsplash' || value.provider === 'pexels') &&
    typeof value.id === 'string' &&
    typeof value.source_url === 'string' &&
    typeof value.preview_url === 'string' &&
    typeof value.original_url === 'string' &&
    typeof value.title === 'string' &&
    typeof value.author === 'string' &&
    typeof value.width === 'number' &&
    typeof value.height === 'number'
  )
}

function isPersistedSearchAuthAction(value: unknown): value is SearchAuthAction {
  if (!isRecord(value)) return false
  switch (value.kind) {
    case 'text-search':
      return typeof value.query === 'string'
    case 'save-reference':
      return isLibrarySearchReference(value.reference)
    case 'save-batch':
      return Array.isArray(value.references) && value.references.every(isLibrarySearchReference)
    case 'open-library':
      return true
    default:
      // Vision references can contain File/blob values and are intentionally memory-only.
      return false
  }
}

function readPersistedAction(): SearchAuthAction | null {
  const currentStorage = storage()
  if (!currentStorage) return null
  try {
    const raw = currentStorage.getItem(PENDING_ACTION_KEY)
    if (!raw) return null
    const value: unknown = JSON.parse(raw)
    if (!isPersistedSearchAuthAction(value)) {
      currentStorage.removeItem(PENDING_ACTION_KEY)
      return null
    }
    return value
  } catch {
    currentStorage.removeItem(PENDING_ACTION_KEY)
    return null
  }
}

function persistAction(action: SearchAuthAction | null): void {
  const currentStorage = storage()
  if (!currentStorage) return
  try {
    if (!action || !isPersistableAction(action)) {
      currentStorage.removeItem(PENDING_ACTION_KEY)
      return
    }
    currentStorage.setItem(PENDING_ACTION_KEY, JSON.stringify(action))
  } catch {
    // Private mode or a full session store should not block authentication.
  }
}

function readTrackedQueries(): string[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.sessionStorage.getItem(ANONYMOUS_QUERY_GATE_KEY)
    if (!raw) return []
    const value: unknown = JSON.parse(raw)
    if (!Array.isArray(value)) return []
    return [...new Set(value.filter((query): query is string => typeof query === 'string'))].slice(
      0,
      MAX_ANONYMOUS_SEARCH_QUERIES,
    )
  } catch {
    return []
  }
}

/** This is a conversion gate, not an enforcement or abuse-prevention quota. */
export function allowAnonymousSearchQuery(query: string): boolean {
  const normalized = query.trim().replace(/\s+/g, ' ')
  if (!normalized) return true

  const tracked = readTrackedQueries()
  if (tracked.includes(normalized)) return true
  if (tracked.length >= MAX_ANONYMOUS_SEARCH_QUERIES) return false

  if (typeof window !== 'undefined') {
    try {
      window.sessionStorage.setItem(
        ANONYMOUS_QUERY_GATE_KEY,
        JSON.stringify([...tracked, normalized]),
      )
    } catch {
      // Private mode or a full session store should not block the user.
    }
  }
  return true
}

export function setPendingSearchAuthAction(action: SearchAuthAction): void {
  pendingAction = action
  persistAction(action)
}

export function peekPendingSearchAuthAction(): SearchAuthAction | null {
  pendingAction ??= readPersistedAction()
  return pendingAction
}

export function consumePendingSearchAuthAction(): SearchAuthAction | null {
  pendingAction ??= readPersistedAction()
  const action = pendingAction
  pendingAction = null
  persistAction(null)
  return action
}

export function clearPendingSearchAuthAction(): void {
  pendingAction = null
  persistAction(null)
}
