import { taskApi, type SubmitTaskData } from '../../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'

export async function assertTaskSubmitReady(
  request: SubmitTaskData,
  fallbackMessage = '提交任务失败',
): Promise<void> {
  const readiness = await taskApi.checkSubmitReadiness(request)
  if (readiness.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(readiness.msg, fallbackMessage))
  }
  if (!readiness.data?.ready) {
    throw new Error(sanitizeApiErrorMessage(readiness.data?.message, fallbackMessage))
  }
}
