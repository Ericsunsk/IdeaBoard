import type {
  SubmitTaskData,
  SubmitTaskReadinessRequestData,
} from '../../../../shared/api/task'
import {
  assertTaskRequestReady,
  submitTaskRequest,
} from '../task-runner/flowImageDirectActionExecutor'
import {
  createCanvasTaskClientActionId,
  withTaskClientActionId,
} from '../task-runner/taskClientActionIds'

export type CanvasTaskSubmission = {
  taskId: string
}

export type CanvasTaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: (clientActionId?: string) => Promise<CanvasTaskSubmission>
}

export type CanvasTaskSubmissionAdapter = {
  assertTaskRequestReady: (
    request: SubmitTaskReadinessRequestData,
    fallbackMessage?: string,
  ) => Promise<void>
  submitTaskRequest: (
    request: SubmitTaskData,
    fallbackMessage?: string,
  ) => Promise<CanvasTaskSubmission>
}

export const defaultCanvasTaskSubmissionAdapter: CanvasTaskSubmissionAdapter = {
  assertTaskRequestReady,
  submitTaskRequest,
}

type CanvasTaskSubmissionOptions = {
  buildRequest: () => SubmitTaskData | Promise<SubmitTaskData>
  fallbackMessage?: string
  taskAdapter?: CanvasTaskSubmissionAdapter
} & (
  | {
      readinessRequest: () => SubmitTaskData
      outputCount: () => number
    }
  | {
      readinessRequest?: undefined
      outputCount?: undefined
    }
)

function requireBillingQuantity(outputCount: number): number {
  if (!Number.isInteger(outputCount) || outputCount < 1 || outputCount > 16) {
    throw new Error('生成数量无效')
  }
  return outputCount
}

export function createCanvasTaskSubmissionCallbacks(
  options: CanvasTaskSubmissionOptions,
): CanvasTaskSubmissionCallbacks {
  const taskAdapter = options.taskAdapter ?? defaultCanvasTaskSubmissionAdapter
  const { readinessRequest } = options
  const assertReady = readinessRequest
    ? async () => {
        await taskAdapter.assertTaskRequestReady(
          {
            ...withTaskClientActionId(readinessRequest(), createCanvasTaskClientActionId()),
            billing_quantity: requireBillingQuantity(options.outputCount()),
          },
          options.fallbackMessage,
        )
      }
    : undefined
  return {
    assertReady,
    submitTask: async (clientActionId) => {
      const request = withTaskClientActionId(await options.buildRequest(), clientActionId)
      return taskAdapter.submitTaskRequest(request, options.fallbackMessage)
    },
  }
}
