import type { LibraryIslandState } from '../../ports/persistencePort'
import { searchScopedStorageKey } from './browserPersistence'
import { readSessionStorageItem, writeSessionStorageItem } from '../../../../core/utils/storage'

const ISLAND_STATE_KEY = 'ideaboard:search:island_state'

export function getLibraryIslandState(scope = 'anonymous'): LibraryIslandState {
  try {
    const raw = readSessionStorageItem(searchScopedStorageKey(ISLAND_STATE_KEY, scope))
    return raw ? (JSON.parse(raw) as LibraryIslandState) : { open: false, expanded: false }
  } catch {
    return { open: false, expanded: false }
  }
}

export function setLibraryIslandState(state: LibraryIslandState, scope = 'anonymous'): void {
  try {
    writeSessionStorageItem(searchScopedStorageKey(ISLAND_STATE_KEY, scope), JSON.stringify(state))
  } catch {
    // Island layout is an optional local preference.
  }
}
