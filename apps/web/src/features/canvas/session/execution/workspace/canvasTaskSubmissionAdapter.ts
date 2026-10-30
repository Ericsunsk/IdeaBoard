import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import { mapCanvasBillingRequestError } from '../task-runner/canvasBillingPreflight'
import { submitCanvasWorkspaceTask } from './canvasWorkspaceTaskAdapter'

export async function submitTaskRequest(
  request: CanvasTaskRequestData,
): Promise<{ taskId: string }> {
  try {
    const submission = await submitCanvasWorkspaceTask(request)
    return { taskId: submission.taskId }
  } catch (error) {
    throw mapCanvasBillingRequestError(error)
  }
}
