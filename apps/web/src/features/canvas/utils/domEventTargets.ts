export const CANVAS_EDITING_SURFACE_SELECTOR = [
  '[data-flow-editing-surface]',
  '[data-image-editor]',
  '[data-image-mask-editor]',
].join(',')

export function isEditableEventTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  const tagName = target.tagName.toLowerCase()
  if (tagName === 'textarea') return true
  if (tagName === 'input') {
    const input = target as HTMLInputElement
    return input.type !== 'button' && input.type !== 'submit' && input.type !== 'reset'
  }
  return !!target.closest('textarea,input,[contenteditable=""],[contenteditable="true"]')
}

export function isCanvasEditingSurfaceEventTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return !!target.closest(CANVAS_EDITING_SURFACE_SELECTOR)
}
