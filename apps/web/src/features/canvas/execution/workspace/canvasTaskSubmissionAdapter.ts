import type { SubmitTaskData, SubmitTaskReadinessRequestData } from '../../../../shared/api/task'
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
    }
  | {
      readinessRequest?: undefined
    }
)

export function createCanvasTaskSubmissionCallbacks(
  options: CanvasTaskSubmissionOptions,
): CanvasTaskSubmissionCallbacks {
  const taskAdapter = options.taskAdapter ?? defaultCanvasTaskSubmissionAdapter
  const { readinessRequest } = options
  const assertReady = readinessRequest
    ? async () => {
        await taskAdapter.assertTaskRequestReady(
          withTaskClientActionId(readinessRequest(), createCanvasTaskClientActionId()),
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
