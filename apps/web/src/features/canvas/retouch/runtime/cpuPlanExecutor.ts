import { executeCpuImageRetouchPlanInPlace } from '../backends/cpu/executeCpuImageRetouchPlan'
import {
  reportPlanExecutionEvent,
  type CreateImageRetouchPlanExecutorOptions,
  type ImageRetouchPlanExecutor,
} from './planExecutorTypes'

export function createCpuImageRetouchPlanExecutor(
  options: Pick<CreateImageRetouchPlanExecutorOptions, 'onPlanExecutorEvent'> = {},
): ImageRetouchPlanExecutor {
  return {
    execute({ imageData, plan }) {
      const output = executeCpuImageRetouchPlanInPlace(imageData, plan)
      reportPlanExecutionEvent(options.onPlanExecutorEvent, {
        kind: 'cpu-plan-completed',
        planLength: plan.length,
        width: imageData.width,
        height: imageData.height,
      })
      return output
    },
  }
}
