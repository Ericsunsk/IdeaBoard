import type {
  CanvasTaskRequestData,
  SubmitTaskReadinessData,
} from '../../../core/execution/canvasTaskRequest'
import { i18n } from '../../../../../core/i18n'
import { sanitizeApiErrorMessage } from '../../../../../core/utils/errorMessages'
import { publishCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import {
  insufficientCreditsPreflightError,
  mapCanvasBillingRequestError,
} from '../task-runner/canvasBillingPreflight'
import {
  fetchCanvasWorkspaceTaskReadiness,
  submitCanvasWorkspaceTask,
} from './canvasWorkspaceTaskAdapter'

export async function assertTaskRequestReady(request: CanvasTaskRequestData): Promise<void> {
  let readiness: SubmitTaskReadinessData
  try {
    readiness = await fetchCanvasWorkspaceTaskReadiness(request)
  } catch (error) {
    throw mapCanvasBillingRequestError(error)
  }
  // Publish the authoritative Quote even when it blocks this submission.
  publishCanvasTaskReadiness(readiness)
  if (readiness.billing && !readiness.billing.sufficient) {
    throw insufficientCreditsPreflightError()
  }
  if (!readiness.ready) {
    throw new Error(
      sanitizeApiErrorMessage(readiness.message, i18n.global.t('common.errors.taskSubmitFailed')),
    )
  }
}

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
