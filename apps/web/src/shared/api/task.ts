import {
  cancelUserTask,
  checkUserTaskSubmitReadiness,
  getUserTask,
  submitUserTask,
} from './generated-query/tasks/tasks'
import { resolveApiData, resolveApiOk } from './apiResponse'

export type {
  ApiResponseTaskSubmitReadinessData as SubmitTaskReadinessData,
  ListUserTasksParams,
  SubmitTaskRequest as SubmitTaskData,
  SubmitTaskReadinessRequest as SubmitTaskReadinessRequestData,
  SubmitTaskRequestParams as SubmitTaskParams,
  Task,
} from './generated-query/model'

async function submit(...args: Parameters<typeof submitUserTask>) {
  const data = await resolveApiData(submitUserTask(...args), '提交任务失败')
  if (!data.task_id?.trim()) throw new Error('提交任务失败')
  return { ...data, task_id: data.task_id.trim() }
}

async function checkSubmitReadiness(...args: Parameters<typeof checkUserTaskSubmitReadiness>) {
  const data = await resolveApiData(checkUserTaskSubmitReadiness(...args), 'Idea计费信息暂不可用')
  if (typeof data.ready !== 'boolean') throw new Error('Idea计费信息暂不可用')
  return data
}

async function get(...args: Parameters<typeof getUserTask>) {
  const data = await resolveApiData(getUserTask(...args), '加载任务失败')
  if (!data.task) throw new Error('加载任务失败')
  return data
}

async function cancel(...args: Parameters<typeof cancelUserTask>) {
  await resolveApiOk(cancelUserTask(...args), '取消任务失败')
}

export const taskApi = {
  submit,
  checkSubmitReadiness,
  get,
  cancel,
}
