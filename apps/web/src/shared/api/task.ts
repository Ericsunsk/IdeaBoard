import {
  cancelUserTask,
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
  cancel: cancelUserTask,
  delete: deleteUserTask,
}

export {
  cancelUserTask as cancelTask,
  checkUserTaskSubmitReadiness as checkSubmitReadiness,
  deleteUserTask as deleteTask,
  getUserTask,
  listUserTasks,
  submitUserTask as submitTask,
}
