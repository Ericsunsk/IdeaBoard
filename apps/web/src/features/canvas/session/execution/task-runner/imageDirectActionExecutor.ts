import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import type { TaskImageResult } from '../../../../../core/query/taskResult'
import { mapCanvasBillingRequestError } from './canvasBillingPreflight'
import { i18n } from '../../../../../core/i18n'
import {
  submitCanvasWorkspaceTask,
  waitCanvasWorkspaceTaskImageResults,
} from '../workspace/canvasWorkspaceTaskAdapter'

export async function submitTaskRequest(
  request: CanvasTaskRequestData,
  fallbackMessage: string = i18n.global.t('common.errors.taskSubmitFailed'),
): Promise<{ taskId: string }> {
  let submission: Awaited<ReturnType<typeof submitCanvasWorkspaceTask>>
  try {
    submission = await submitCanvasWorkspaceTask(request)
  } catch (error) {
    throw mapCanvasBillingRequestError(error)
  }
  if (!submission.taskId) throw new Error(fallbackMessage)
  return { taskId: submission.taskId }
}

export async function waitExistingOrLiveTaskImageResults(
  taskId: string,
): Promise<TaskImageResult[]> {
  return waitCanvasWorkspaceTaskImageResults(taskId)
}
