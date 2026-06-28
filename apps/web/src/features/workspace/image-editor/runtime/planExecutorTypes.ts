import type { ImageAdjustmentPlan } from '../utils/imageAdjustmentEngine'
import type { ImageProcessingBackendPreviewInput } from './types'

export type ImageProcessingBackendTaskProgressReporter =
  ImageProcessingBackendPreviewInput['context']['reportProgress']

export type ImageAdjustmentPlanExecutorInput = {
  imageData: ImageData
  plan: ImageAdjustmentPlan
  signal: AbortSignal
  onProgress?: ImageProcessingBackendTaskProgressReporter
}

export type ImageAdjustmentPlanExecutor = {
  execute(input: ImageAdjustmentPlanExecutorInput): Promise<ImageData> | ImageData
  dispose?(): void
}

export type ImageAdjustmentPlanExecutionEvent =
  | {
      kind: 'hybrid-plan-selected'
      backend: 'cpu-main'
      executionMode: 'sync' | 'cooperative'
      reason: 'work-threshold' | 'empty-plan'
      planLength: number
      width: number
      height: number
      pixelCount: number
      workBytes: number
      syncThresholdWorkBytes: number
    }
  | {
      kind: 'worker-plan-started'
      planLength: number
      width: number
      height: number
    }
  | {
      kind: 'worker-plan-completed'
      planLength: number
      width: number
      height: number
    }
  | {
      kind: 'worker-plan-fallback'
      planLength: number
      width: number
      height: number
      errorName: string
      errorMessage: string
    }
  | {
      kind: 'cpu-plan-completed'
      planLength: number
      width: number
      height: number
    }

export type ImageAdjustmentPlanExecutionEventReporter = (
  event: ImageAdjustmentPlanExecutionEvent,
) => void

export type CreateImageAdjustmentPlanExecutorOptions = {
  onPlanExecutorEvent?: ImageAdjustmentPlanExecutionEventReporter
}

export function reportPlanExecutionEvent(
  reporter: ImageAdjustmentPlanExecutionEventReporter | undefined,
  event: ImageAdjustmentPlanExecutionEvent,
) {
  reporter?.(event)
}
