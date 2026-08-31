import {
  cancelUserTask,
  checkUserTaskSubmitReadiness,
  checkUserTaskSubmitReadinessMatrix,
  getUserTask,
  submitUserTask,
} from './generated-query/tasks/tasks'
import { resolveApiData, resolveApiOk } from './apiResponse'
import { i18n } from '../i18n'
import type { ApiResponseTaskSubmitReadinessData } from './generated-query/model'

export type {
  ApiResponseTaskSubmitReadinessData as SubmitTaskReadinessData,
  ListUserTasksParams,
  SubmitTaskRequest as SubmitTaskData,
  SubmitTaskReadinessRequest as SubmitTaskReadinessRequestData,
  SubmitTaskReadinessMatrixRequest,
  SubmitTaskRequestParams as SubmitTaskParams,
  Task,
} from './generated-query/model'

async function submit(...args: Parameters<typeof submitUserTask>) {
  const fallback = () => i18n.global.t('common.errors.taskSubmitFailed')
  const data = await resolveApiData(submitUserTask(...args), fallback())
  if (!data.task_id?.trim()) throw new Error(fallback())
  return { ...data, task_id: data.task_id.trim() }
}

async function checkSubmitReadiness(...args: Parameters<typeof checkUserTaskSubmitReadiness>) {
  const fallback = () => i18n.global.t('common.errors.billingInfoUnavailable')
  const data = await resolveApiData(checkUserTaskSubmitReadiness(...args), fallback())
  if (typeof data.ready !== 'boolean') throw new Error(fallback())
  return data
}

export type SubmitTaskReadinessMatrixItem = ApiResponseTaskSubmitReadinessData

async function checkSubmitReadinessMatrix(
  ...args: Parameters<typeof checkUserTaskSubmitReadinessMatrix>
): Promise<SubmitTaskReadinessMatrixItem[]> {
  const fallback = () => i18n.global.t('common.errors.billingInfoUnavailable')
  const data = await resolveApiData(checkUserTaskSubmitReadinessMatrix(...args), fallback())
  if (!Array.isArray(data.items)) throw new Error(fallback())
  return data.items as SubmitTaskReadinessMatrixItem[]
}

async function get(...args: Parameters<typeof getUserTask>) {
  const fallback = () => i18n.global.t('common.errors.taskLoadFailed')
  const data = await resolveApiData(getUserTask(...args), fallback())
  if (!data.task) throw new Error(fallback())
  return data
}

async function cancel(...args: Parameters<typeof cancelUserTask>) {
  await resolveApiOk(cancelUserTask(...args), i18n.global.t('common.errors.taskCancelFailed'))
}

export const taskApi = {
  submit,
  checkSubmitReadiness,
  checkSubmitReadinessMatrix,
  get,
  cancel,
}
