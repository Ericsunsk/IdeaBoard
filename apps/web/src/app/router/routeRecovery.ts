import type { RouteLocationNormalized } from 'vue-router'

export const CHUNK_RELOAD_GUARD_KEY = 'ideaboard:chunk-reload'

type SavedScrollPosition = { left: number; top: number }
type SavedScrollInput = SavedScrollPosition | null

type ScrollTarget = Pick<RouteLocationNormalized, 'path'>

/**
 * Back/forward restores the saved position; a path change starts at the top.
 * Same-path navigations (e.g. search `?q` keystrokes) keep the scroll position
 * so feed scroll-restore never fights the router.
 */
export function resolveScrollPosition(
  to: ScrollTarget,
  from: ScrollTarget,
  savedPosition: SavedScrollInput,
): SavedScrollPosition | { top: number } | false {
  if (savedPosition) return savedPosition
  if (to.path !== from.path) return { top: 0 }
  return false
}

/** Matches stale-manifest chunk failures from Vite and webpack builds. */
export function isChunkLoadFailure(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error ?? '')
  return /Failed to fetch dynamically imported module|Importing a module script failed|Loading chunk [\w-]+ failed/i.test(
    message,
  )
}
