import {
  taskApi,
  type SubmitTaskReadinessData,
  type SubmitTaskReadinessRequestData,
} from '../../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import { i18n } from '../../../../shared/i18n'
import { publishCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import {
  insufficientCreditsPreflightError,
  mapCanvasBillingRequestError,
} from './canvasBillingPreflight'

export async function assertTaskSubmitReady(
  request: SubmitTaskReadinessRequestData,
  fallbackMessage: string = i18n.global.t('common.errors.taskSubmitFailed'),
): Promise<SubmitTaskReadinessData> {
  let readiness: SubmitTaskReadinessData
  try {
    readiness = await taskApi.checkSubmitReadiness(request)
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
