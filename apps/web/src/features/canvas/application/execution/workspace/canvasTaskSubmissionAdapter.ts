import type {
  SubmitTaskData,
  SubmitTaskReadinessRequestData,
} from '../../../domain/execution-contracts/canvasTaskRequest'
import { submitTaskRequest } from '../task-runner/imageDirectActionExecutor'
import { assertTaskSubmitReady } from '../task-runner/taskReadiness'

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
  assertTaskRequestReady: (request, fallbackMessage) =>
    assertTaskSubmitReady(request, fallbackMessage).then(() => undefined),
  submitTaskRequest: (request, fallbackMessage) => submitTaskRequest(request, fallbackMessage),
}
