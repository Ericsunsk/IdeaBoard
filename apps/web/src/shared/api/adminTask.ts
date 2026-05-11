import { getAdminTasks } from './generated/admin-tasks/admin-tasks'
import type {
  ApiResponseTaskData,
  ApiResponseTaskIdData,
  ApiResponseTaskListData,
  ListAdminTasksParams,
} from './generated/model'
import type { ApiResponse } from './types'
import type { SubmitTaskData } from './task'

const adminTasksClient = getAdminTasks()

export const adminTaskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<ApiResponseTaskIdData>> =>
    adminTasksClient.submitAdminTask(data),
  get: (task_id: string): Promise<ApiResponse<ApiResponseTaskData>> =>
    adminTasksClient.getAdminTask(task_id),
  list: (params?: ListAdminTasksParams): Promise<ApiResponse<ApiResponseTaskListData>> =>
    adminTasksClient.listAdminTasks(params),
}
