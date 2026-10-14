const IMAGE_DRAG_GHOST_STYLE =
  'position:fixed;top:-9999px;left:-9999px;width:58px;height:44px;border-radius:var(--radius-panel);overflow:hidden;box-shadow:var(--shadow-floating);transform:rotate(-3deg);z-index:var(--z-index-lightbox);pointer-events:none'
const IMAGE_DRAG_GHOST_IMAGE_STYLE = 'width:100%;height:100%;object-fit:cover'

export function createImageDragGhost(previewUrl: string): HTMLElement {
  const ghost = document.createElement('div')
  ghost.style.cssText = IMAGE_DRAG_GHOST_STYLE
  const image = document.createElement('img')
  image.src = previewUrl
  image.style.cssText = IMAGE_DRAG_GHOST_IMAGE_STYLE
  ghost.appendChild(image)
  document.body.appendChild(ghost)
  return ghost
}
