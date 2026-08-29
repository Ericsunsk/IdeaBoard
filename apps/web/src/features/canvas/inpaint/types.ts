export const INPAINT_INTERACTION_KEY = 'inpaint' as const

export type InpaintInteractionKey = typeof INPAINT_INTERACTION_KEY
export type ImageMaskDrawMode = 'paint' | 'erase'

export type ImageInpaintCommittedData = {
  executionMaskDataUrl: string
  maskDataUrl: string
}
