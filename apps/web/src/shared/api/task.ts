import {
  checkUserTaskSubmitReadiness,
  deleteUserTask,
  getUserTask,
  listUserTasks,
  submitUserTask,
} from './generated-query/tasks/tasks'

export type {
  ApiResponseTaskSubmitReadinessData as SubmitTaskReadinessData,
  ListUserTasksParams,
  SubmitTaskRequest as SubmitTaskData,
  SubmitTaskRequestParams as SubmitTaskParams,
  Task,
} from './generated-query/model'

export const taskApi = {
  submit: submitUserTask,
  checkSubmitReadiness: checkUserTaskSubmitReadiness,
  get: getUserTask,
  list: listUserTasks,
  delete: deleteUserTask,
}

export {
  checkUserTaskSubmitReadiness as checkSubmitReadiness,
  deleteUserTask as deleteTask,
  getUserTask,
  listUserTasks,
  submitUserTask as submitTask,
}
