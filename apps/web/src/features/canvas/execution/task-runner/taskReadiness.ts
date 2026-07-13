import {
  taskApi,
  type SubmitTaskReadinessData,
  type SubmitTaskReadinessRequestData,
} from '../../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import { publishCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import {
  insufficientCreditsPreflightError,
  mapCanvasBillingRequestError,
} from './canvasBillingPreflight'

export async function assertTaskSubmitReady(
  request: SubmitTaskReadinessRequestData,
  fallbackMessage = '提交任务失败',
): Promise<SubmitTaskReadinessData> {
  let readiness: Awaited<ReturnType<typeof taskApi.checkSubmitReadiness>>
  try {
    readiness = await taskApi.checkSubmitReadiness(request)
  } catch (error) {
    throw mapCanvasBillingRequestError(error)
  }
  if (readiness.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(readiness.msg, fallbackMessage))
  }
  if (!readiness.data) {
    throw new Error(fallbackMessage)
  }
  publishCanvasTaskReadiness(readiness.data)
  if (readiness.data.billing && !readiness.data.billing.sufficient) {
    throw insufficientCreditsPreflightError()
  }
  if (!readiness.data.ready) {
    throw new Error(sanitizeApiErrorMessage(readiness.data.message, fallbackMessage))
  }
  return readiness.data
}
