import {
  deleteUserTask,
  getUserTask,
  listUserTasks,
  submitUserTask,
} from './generated-query/tasks/tasks'
import type {
  ApiResponseMessageData,
  ApiResponseTaskData,
  ApiResponseTaskIdData,
  ApiResponseTaskListData,
  ListUserTasksParams,
  SubmitTaskRequest,
  Task as GeneratedTask,
} from './generated-query/model'
import type { ApiResponse } from './types'

export type Task = GeneratedTask
export type SubmitTaskData = SubmitTaskRequest

export const taskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<ApiResponseTaskIdData>> =>
    submitUserTask(data),
  get: (task_id: string): Promise<ApiResponse<ApiResponseTaskData>> => getUserTask(task_id),
  list: (params?: ListUserTasksParams): Promise<ApiResponse<ApiResponseTaskListData>> =>
    listUserTasks(params),
  delete: (task_id: string): Promise<ApiResponse<ApiResponseMessageData>> =>
    deleteUserTask(task_id),
}
