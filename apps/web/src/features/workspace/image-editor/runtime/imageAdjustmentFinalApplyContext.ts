import type { InjectionKey, Ref } from 'vue'
import type { ImageProcessingProgress } from './types'

export type ImageAdjustmentFinalApplyProgress = ImageProcessingProgress

export type WorkspaceImageAdjustmentFinalApplyContext = {
  progress: Readonly<Ref<ImageAdjustmentFinalApplyProgress | null>>
  cancel: () => void
}

export const IMAGE_ADJUSTMENT_FINAL_APPLY_CONTEXT_KEY: InjectionKey<WorkspaceImageAdjustmentFinalApplyContext> =
  Symbol('workspace:image-adjustment-final-apply')
