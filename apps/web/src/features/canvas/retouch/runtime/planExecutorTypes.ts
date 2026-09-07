import type { ImageRetouchPlan } from '../utils/imageRetouchEngine'
import type { ImageProcessingProgress } from './types'

export type ImageProcessingBackendTaskProgressReporter = (progress: ImageProcessingProgress) => void

export type ImageRetouchPlanExecutorInput = {
  imageData: ImageData
  plan: ImageRetouchPlan
  signal: AbortSignal
  onProgress?: ImageProcessingBackendTaskProgressReporter | undefined
}

export type ImageRetouchPlanExecutor = {
  execute(input: ImageRetouchPlanExecutorInput): Promise<ImageData> | ImageData
  dispose?(): void
}

export type ImageRetouchPlanExecutionEvent =
  | {
      kind: 'hybrid-plan-selected'
      backend: 'cpu-main'
      executionMode: 'sync' | 'cooperative'
      reason: 'work-threshold' | 'empty-plan'
      planLength: number
      width: number
      height: number
      pixelCount: number
      workUnits: number
      syncThresholdWorkUnits: number
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

export type ImageRetouchPlanExecutionEventReporter = (event: ImageRetouchPlanExecutionEvent) => void

export type CreateImageRetouchPlanExecutorOptions = {
  onPlanExecutorEvent?: ImageRetouchPlanExecutionEventReporter | undefined
}

export function reportPlanExecutionEvent(
  reporter: ImageRetouchPlanExecutionEventReporter | undefined,
  event: ImageRetouchPlanExecutionEvent,
) {
  reporter?.(event)
}
