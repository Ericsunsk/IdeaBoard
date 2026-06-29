import type { InjectionKey, Ref } from 'vue'
import type { ImageProcessingProgress } from './types'

export type ImageAdjustmentFinalApplyProgress = ImageProcessingProgress

export type CanvasImageAdjustmentFinalApplyContext = {
  progress: Readonly<Ref<ImageAdjustmentFinalApplyProgress | null>>
  cancel: () => void
}

export const IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY: InjectionKey<CanvasImageAdjustmentFinalApplyContext> =
  Symbol('canvas:image-adjustment-final-apply')
