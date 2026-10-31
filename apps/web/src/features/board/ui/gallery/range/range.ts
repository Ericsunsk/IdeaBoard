import { ref, type Ref } from 'vue'
import { readLocalStorageItem, writeLocalStorageItem } from '../../../../../core/utils/storage'
import { boardScopeKey, getBoardScope, isCompleteBoardScope } from '../../../session/catalog/scope'

// Ranges filter the authorized catalog without changing permissions or fetching.
// `scope` identifies User + Workspace; `range` selects personal or team cards.

export type BoardGalleryRange = 'personal' | 'team'

function isGalleryRange(value: unknown): value is BoardGalleryRange {
  return value === 'personal' || value === 'team'
}

/**
 * Minimal structural range facts: the gallery, the controller, and specs
 * share these predicates without importing the store or adapter types.
 */
export type RangeBoardFacts = {
  accessState?: string | undefined
  isCreator?: boolean | undefined
  revision?: number | undefined
}

/**
 * Personal boards by ownership: boards the caller created (`isCreator`)
 * plus never-synced local drafts (`revision === undefined`).
 * Creator-shared boards stay personal after sharing, so they appear in
 * both personal and team; ranges overlap and are counted independently.
 */
export function isPersonalBoard(board: RangeBoardFacts): boolean {
  return board.isCreator === true || board.revision === undefined
}

/** Every workspace-shared board, including boards the caller shared. */
export function isTeamBoard(board: RangeBoardFacts): boolean {
  return board.accessState === 'workspace'
}

export function filterBoardsByRange<TBoard extends RangeBoardFacts>(
  boards: readonly TBoard[],
  range: BoardGalleryRange,
): TBoard[] {
  if (range === 'team') return boards.filter(isTeamBoard)
  return boards.filter(isPersonalBoard)
}

const GALLERY_RANGE_STORAGE_PREFIX = 'ideaboard.board.galleryRange.v1'

function persistedRangeKey(): string | null {
  // Fail closed to the default personal range when the scope cannot be
  // resolved (signed-out surfaces, incomplete scope, unavailable store).
  let scope: ReturnType<typeof getBoardScope>
  try {
    scope = getBoardScope()
  } catch {
    return null
  }
  if (!isCompleteBoardScope(scope)) return null
  return `${GALLERY_RANGE_STORAGE_PREFIX}.${boardScopeKey(scope)}`
}

/** Persisted range for the current User + Workspace; defaults to personal. */
export function readPersistedGalleryRange(): BoardGalleryRange {
  const key = persistedRangeKey()
  if (!key) return 'personal'
  const raw = readLocalStorageItem(key)
  return isGalleryRange(raw) ? raw : 'personal'
}

/** Persists the range isolated per User + Workspace; no-op without scope. */
export function persistGalleryRange(range: BoardGalleryRange): void {
  if (!isGalleryRange(range)) return
  const key = persistedRangeKey()
  if (!key) return
  writeLocalStorageItem(key, range)
}

/**
 * Gallery range state: defaults to the persisted User + Workspace
 * selection (personal when nothing is saved), persists every switch, and
 * reloads the current scope's saved range on demand. The controller owns
 * one instance; switching never touches the catalog or permissions.
 */
export function useGalleryRange(): {
  range: Ref<BoardGalleryRange>
  setRange(next: BoardGalleryRange): void
  reloadPersistedRange(): void
} {
  const range = ref<BoardGalleryRange>(readPersistedGalleryRange())
  function setRange(next: BoardGalleryRange): void {
    if (!isGalleryRange(next)) return
    range.value = next
    persistGalleryRange(next)
  }
  function reloadPersistedRange(): void {
    range.value = readPersistedGalleryRange()
  }
  return { range, setRange, reloadPersistedRange }
}
