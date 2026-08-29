import { executeCpuImageAdjustmentPlanInPlace } from '../backends/cpu/executeCpuImageAdjustmentPlan'
import {
  reportPlanExecutionEvent,
  type CreateImageAdjustmentPlanExecutorOptions,
  type ImageAdjustmentPlanExecutor,
} from './planExecutorTypes'

export function createCpuImageAdjustmentPlanExecutor(
  options: Pick<CreateImageAdjustmentPlanExecutorOptions, 'onPlanExecutorEvent'> = {},
): ImageAdjustmentPlanExecutor {
  return {
    execute({ imageData, plan }) {
      const output = executeCpuImageAdjustmentPlanInPlace(imageData, plan)
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
