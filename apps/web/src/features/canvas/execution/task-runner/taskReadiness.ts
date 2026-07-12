import {
  taskApi,
  type SubmitTaskData,
  type SubmitTaskReadinessData,
} from '../../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import { publishCanvasTaskReadiness } from '../billing/taskReadinessEvents'

export async function assertTaskSubmitReady(
  request: SubmitTaskData,
  fallbackMessage = '提交任务失败',
): Promise<SubmitTaskReadinessData> {
  const readiness = await taskApi.checkSubmitReadiness(request)
  if (readiness.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(readiness.msg, fallbackMessage))
  }
  if (!readiness.data) {
    throw new Error(fallbackMessage)
  }
  publishCanvasTaskReadiness(readiness.data)
  if (!readiness.data.ready) {
    throw new Error(sanitizeApiErrorMessage(readiness.data.message, fallbackMessage))
  }
  return readiness.data
}
