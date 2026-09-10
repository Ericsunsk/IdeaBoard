import type { SubmitTaskData, SubmitTaskReadinessRequestData } from '../../../../../shared/api/task'
import {
  assertTaskRequestReady,
  submitTaskRequest,
} from '../task-runner/imageDirectActionExecutor'

export type CanvasTaskSubmission = {
  taskId: string
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
