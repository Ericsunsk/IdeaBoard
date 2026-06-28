import {
  disposeDefaultWorkerImageProcessingClient,
  executeWorkerImageAdjustmentPlan,
} from '../backends/worker/executeWorkerImageAdjustmentPlan'
import { createCpuImageAdjustmentPlanExecutor } from './cpuPlanExecutor'
import {
  reportPlanExecutionEvent,
  type CreateImageAdjustmentPlanExecutorOptions,
  type ImageAdjustmentPlanExecutor,
} from './planExecutorTypes'

export function createWorkerImageAdjustmentPlanExecutor(
  options: Pick<CreateImageAdjustmentPlanExecutorOptions, 'onPlanExecutorEvent'> = {},
): ImageAdjustmentPlanExecutor {
  return {
    async execute({ imageData, plan, signal, onProgress }) {
      try {
        reportPlanExecutionEvent(options.onPlanExecutorEvent, {
          kind: 'worker-plan-started',
          planLength: plan.length,
          width: imageData.width,
          height: imageData.height,
        })
        const output = await executeWorkerImageAdjustmentPlan(imageData, plan, {
          signal,
          onProgress,
        })
        reportPlanExecutionEvent(options.onPlanExecutorEvent, {
          kind: 'worker-plan-completed',
          planLength: plan.length,
          width: imageData.width,
          height: imageData.height,
        })
        return output
      } catch (error) {
        if (signal.aborted) throw error
        reportPlanExecutionEvent(options.onPlanExecutorEvent, {
          kind: 'worker-plan-fallback',
          planLength: plan.length,
          width: imageData.width,
          height: imageData.height,
          ...normalizeExecutorError(error),
        })

        return createCpuImageAdjustmentPlanExecutor(options).execute({
          imageData,
          plan,
          signal,
          onProgress,
        })
      }
    },

    dispose() {
      disposeDefaultWorkerImageProcessingClient()
    },
  }
}

function normalizeExecutorError(error: unknown) {
  if (error instanceof Error) {
    return {
      errorName: error.name,
      errorMessage: error.message,
    }
  }
  return {
    errorName: typeof error,
    errorMessage: String(error),
  }
}
