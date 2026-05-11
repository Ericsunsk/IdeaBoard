import { getTasks } from './generated/tasks/tasks'
import type {
  ApiResponseMessageData,
  ApiResponseTaskData,
  ApiResponseTaskIdData,
  ApiResponseTaskListData,
  ListUserTasksParams,
  SubmitTaskRequest,
  Task as GeneratedTask,
} from './generated/model'
import type { ApiResponse } from './types'

const tasksClient = getTasks()

export type Task = GeneratedTask
export type SubmitTaskData = SubmitTaskRequest

export const taskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<ApiResponseTaskIdData>> =>
    tasksClient.submitUserTask(data),
  get: (task_id: string): Promise<ApiResponse<ApiResponseTaskData>> =>
    tasksClient.getUserTask(task_id),
  list: (params?: ListUserTasksParams): Promise<ApiResponse<ApiResponseTaskListData>> =>
    tasksClient.listUserTasks(params),
  delete: (task_id: string): Promise<ApiResponse<ApiResponseMessageData>> =>
    tasksClient.deleteUserTask(task_id),
}
