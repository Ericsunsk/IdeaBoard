import type {
  SubmitTaskReadinessData,
  SubmitTaskReadinessRequestData,
} from '../../../domain/execution-contracts/canvasTaskRequest'
import { sanitizeApiErrorMessage } from '../../../../../shared/utils/errorMessages'
import { i18n } from '../../../../../shared/i18n'
import { publishCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import {
  insufficientCreditsPreflightError,
  mapCanvasBillingRequestError,
} from './canvasBillingPreflight'
import { fetchCanvasWorkspaceTaskReadiness } from '../workspace/canvasWorkspaceTaskAdapter'

export async function assertTaskSubmitReady(
  request: SubmitTaskReadinessRequestData,
  fallbackMessage: string = i18n.global.t('common.errors.taskSubmitFailed'),
): Promise<SubmitTaskReadinessData> {
  let readiness: SubmitTaskReadinessData
  try {
    readiness = await fetchCanvasWorkspaceTaskReadiness(request)
  } catch (error) {
    throw mapCanvasBillingRequestError(error)
  }
  publishCanvasTaskReadiness(readiness)
  if (readiness.billing && !readiness.billing.sufficient) {
    throw insufficientCreditsPreflightError()
  }
  if (!readiness.ready) {
    throw new Error(sanitizeApiErrorMessage(readiness.message, fallbackMessage))
  }
  return readiness
}

/** Workspace-backed readiness lives in `assertTaskSubmitReady` (single seam). */
