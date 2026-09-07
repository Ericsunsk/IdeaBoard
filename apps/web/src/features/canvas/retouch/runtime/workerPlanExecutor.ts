import {
  disposeDefaultWorkerImageProcessingClient,
  executeWorkerImageRetouchPlan,
} from '../backends/worker/executeWorkerImageRetouchPlan'
import { createCpuImageRetouchPlanExecutor } from './cpuPlanExecutor'
import {
  reportPlanExecutionEvent,
  type CreateImageRetouchPlanExecutorOptions,
  type ImageRetouchPlanExecutor,
} from './planExecutorTypes'

export function createWorkerImageRetouchPlanExecutor(
  options: Pick<CreateImageRetouchPlanExecutorOptions, 'onPlanExecutorEvent'> = {},
): ImageRetouchPlanExecutor {
  return {
    async execute({ imageData, plan, signal, onProgress }) {
      try {
        reportPlanExecutionEvent(options.onPlanExecutorEvent, {
          kind: 'worker-plan-started',
          planLength: plan.length,
          width: imageData.width,
          height: imageData.height,
        })
        const output = await executeWorkerImageRetouchPlan(imageData, plan, {
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

        return createCpuImageRetouchPlanExecutor(options).execute({
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
