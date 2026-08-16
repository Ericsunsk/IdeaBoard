export const MASK_EDITOR_INTERACTION_KEY = 'image-mask-editor' as const

export type MaskEditorInteractionKey = typeof MASK_EDITOR_INTERACTION_KEY
export type ImageMaskDrawMode = 'paint' | 'erase'
export type ImageMaskBrushShape = 'round' | 'square'

export type ImageMaskEditorCommittedData = {
  executionMaskDataUrl: string
  maskDataUrl: string
}
