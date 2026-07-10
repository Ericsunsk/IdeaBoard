import type { SubmitTaskData } from '../../../../shared/api/task'
import {
  assertTaskRequestReady,
  submitTaskRequest,
} from '../task-runner/flowImageDirectActionExecutor'
import { withTaskClientActionId } from '../task-runner/taskClientActionIds'

export type CanvasTaskSubmission = {
  taskId: string
}

export type CanvasTaskSubmissionCallbacks = {
  assertReady?: () => Promise<void>
  submitTask: (clientActionId?: string) => Promise<CanvasTaskSubmission>
}

export type CanvasTaskSubmissionAdapter = {
  assertTaskRequestReady: (request: SubmitTaskData, fallbackMessage?: string) => Promise<void>
  submitTaskRequest: (
    request: SubmitTaskData,
    fallbackMessage?: string,
  ) => Promise<CanvasTaskSubmission>
}

export const defaultCanvasTaskSubmissionAdapter: CanvasTaskSubmissionAdapter = {
  assertTaskRequestReady,
  submitTaskRequest,
}

export function createCanvasTaskSubmissionCallbacks(options: {
  readinessRequest?: () => SubmitTaskData
  buildRequest: () => SubmitTaskData | Promise<SubmitTaskData>
  fallbackMessage?: string
  taskAdapter?: CanvasTaskSubmissionAdapter
}): CanvasTaskSubmissionCallbacks {
  const taskAdapter = options.taskAdapter ?? defaultCanvasTaskSubmissionAdapter
  const { readinessRequest } = options
  const assertReady = readinessRequest
    ? () => taskAdapter.assertTaskRequestReady(readinessRequest(), options.fallbackMessage)
    : undefined
  return {
    assertReady,
    submitTask: async (clientActionId) =>
      taskAdapter.submitTaskRequest(
        withTaskClientActionId(await options.buildRequest(), clientActionId),
        options.fallbackMessage,
      ),
  }
}
