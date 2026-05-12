import {
  getAdminTask,
  listAdminTasks,
  submitAdminTask,
} from './generated-query/admin-tasks/admin-tasks'
import type {
  ApiResponseTaskData,
  ApiResponseTaskIdData,
  ApiResponseTaskListData,
  ListAdminTasksParams,
} from './generated-query/model'
import type { ApiResponse } from './types'
import type { SubmitTaskData } from './task'

export const adminTaskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<ApiResponseTaskIdData>> =>
    submitAdminTask(data),
  get: (task_id: string): Promise<ApiResponse<ApiResponseTaskData>> => getAdminTask(task_id),
  list: (params?: ListAdminTasksParams): Promise<ApiResponse<ApiResponseTaskListData>> =>
    listAdminTasks(params),
}
