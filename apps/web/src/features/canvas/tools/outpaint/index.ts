export { default as ImageOutpaintOverlay } from './ui/ImageOutpaintOverlay.vue'
export { OUTPAINT_DRAG_ANCHORS, useImageOutpaintOverlay } from './session/useImageOutpaintOverlay.ts'
export type { OutpaintDragAnchor } from './session/useImageOutpaintOverlay.ts'
export { useFlowOutpaintInteractionHandlers } from './session/outpaintIntent.ts'
export {
  containImageRect,
  createOutpaintRect,
  resizeOutpaintFrameFromDrag,
  resolveDefaultOutpaintFrame,
  resolveOutpaintIntent,
  resolveOutpaintPixelGeometry,
} from './core/geometry.ts'
export type {
  OutpaintPixelGeometry,
  OutpaintRect,
  OutpaintResolvedIntent,
} from './core/geometry.ts'
