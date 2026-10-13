import type { SearchResult } from '../../core/models'

export const SEARCH_LIBRARY_AUTO_OPEN_DELAY_MS = 280

const SEARCH_DRAG_GHOST_OFFSET_X = 29
const SEARCH_DRAG_GHOST_OFFSET_Y = 22

const SEARCH_DRAG_GHOST_STYLE =
  'position:fixed;top:-9999px;left:-9999px;width:58px;height:44px;border-radius:var(--radius-lg);overflow:hidden;box-shadow:var(--shadow-floating);transform:rotate(-3deg);z-index:9999'
const SEARCH_DRAG_GHOST_IMAGE_STYLE = 'width:100%;height:100%;object-fit:cover'

export function resolveDragAnchor(event: Event): HTMLElement | null {
  return event.currentTarget as HTMLElement | null
}

export function createSearchDragGhost(result: SearchResult): HTMLElement {
  const ghost = document.createElement('div')
  ghost.style.cssText = SEARCH_DRAG_GHOST_STYLE
  const image = document.createElement('img')
  image.src = result.thumbnail_url || result.preview_url
  image.style.cssText = SEARCH_DRAG_GHOST_IMAGE_STYLE
  ghost.appendChild(image)
  document.body.appendChild(ghost)
  return ghost
}

export function attachSearchDragImage(
  dataTransfer: DataTransfer,
  result: SearchResult,
): () => void {
  const ghost = createSearchDragGhost(result)
  dataTransfer.setDragImage(ghost, SEARCH_DRAG_GHOST_OFFSET_X, SEARCH_DRAG_GHOST_OFFSET_Y)
  let disposed = false
  const dispose = () => {
    if (disposed) return
    disposed = true
    ghost.remove()
  }
  setTimeout(dispose, 0)
  return dispose
}
