import type { LibraryIslandState } from '../ports/searchPersistence'
import { searchScopedStorageKey, searchSessionStorage } from './searchBrowserPersistence'

const ISLAND_STATE_KEY = 'ideaboard:search:island_state'

export function getLibraryIslandState(scope = 'anonymous'): LibraryIslandState {
  const session = searchSessionStorage()
  if (!session) return { open: false, expanded: false }
  try {
    const raw = session.getItem(searchScopedStorageKey(ISLAND_STATE_KEY, scope))
    return raw ? (JSON.parse(raw) as LibraryIslandState) : { open: false, expanded: false }
  } catch {
    return { open: false, expanded: false }
  }
}

export function setLibraryIslandState(state: LibraryIslandState, scope = 'anonymous'): void {
  try {
    searchSessionStorage()?.setItem(
      searchScopedStorageKey(ISLAND_STATE_KEY, scope),
      JSON.stringify(state),
    )
  } catch {
    // Island layout is an optional local preference.
  }
}
