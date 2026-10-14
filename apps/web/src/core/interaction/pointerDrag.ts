import type { PointerDropTargets } from './pointerDropTargets'
import { createImageDragGhost } from './imageDragGhost'

type PointerDragOptions<T> = {
  payload: T
  targets: PointerDropTargets<T>
  previewUrl: string
  onEnd?: () => void
}

function suppressDragClick() {
  const suppress = (event: MouseEvent) => {
    event.preventDefault()
    event.stopImmediatePropagation()
  }
  window.addEventListener('click', suppress, { capture: true, once: true })
  setTimeout(() => window.removeEventListener('click', suppress, true), 0)
}

export function startPointerDrag<T>(
  initial: PointerEvent,
  options: PointerDragOptions<T>,
): () => void {
  if (initial.button !== 0 || initial.isPrimary === false || initial.pointerType === 'touch') {
    return () => {}
  }
  let ghost: HTMLElement | null = null
  let frame: number | null = null
  let lastEvent = initial
  let isDisposed = false

  function trackTargets() {
    options.targets.update(lastEvent.clientX, lastEvent.clientY, options.payload)
    frame = requestAnimationFrame(trackTargets)
  }

  function move(event: PointerEvent) {
    if (event.pointerId !== initial.pointerId) return
    lastEvent = event
    if (!ghost) {
      if (Math.hypot(event.clientX - initial.clientX, event.clientY - initial.clientY) < 6) return
      ghost = createImageDragGhost(options.previewUrl)
      document.documentElement.classList.add('image-pointer-dragging')
      frame = requestAnimationFrame(trackTargets)
    }
    event.preventDefault()
    ghost.style.left = `${event.clientX - 29}px`
    ghost.style.top = `${event.clientY - 22}px`
    options.targets.update(event.clientX, event.clientY, options.payload)
  }

  function dispose() {
    if (isDisposed) return
    isDisposed = true
    window.removeEventListener('pointermove', move, true)
    window.removeEventListener('pointerup', end, true)
    window.removeEventListener('pointercancel', cancel, true)
    window.removeEventListener('pointerout', leaveWindow, true)
    window.removeEventListener('blur', dispose)
    window.removeEventListener('keydown', keydown, true)
    if (frame !== null) cancelAnimationFrame(frame)
    options.targets.clear()
    ghost?.remove()
    if (ghost) document.documentElement.classList.remove('image-pointer-dragging')
    release()
    options.onEnd?.()
  }

  function end(event: PointerEvent) {
    if (event.pointerId !== initial.pointerId) return
    try {
      if (!ghost) return
      options.targets.drop(options.payload, event.clientX, event.clientY)
      suppressDragClick()
    } finally {
      dispose()
    }
  }

  function cancel(event: PointerEvent) {
    if (event.pointerId === initial.pointerId) dispose()
  }

  function leaveWindow(event: PointerEvent) {
    if (event.relatedTarget === null) cancel(event)
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape') dispose()
  }

  const release = options.targets.begin(dispose)
  window.addEventListener('pointermove', move, { capture: true, passive: false })
  window.addEventListener('pointerup', end, true)
  window.addEventListener('pointercancel', cancel, true)
  window.addEventListener('pointerout', leaveWindow, true)
  window.addEventListener('blur', dispose)
  window.addEventListener('keydown', keydown, true)
  return dispose
}
