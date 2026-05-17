import {
  getAdminTask,
  listAdminTasks,
  submitAdminTask,
} from './generated-query/admin-tasks/admin-tasks'
export type { ListAdminTasksParams, SubmitTaskRequest as AdminSubmitTaskData } from './generated-query/model'

export const adminTaskApi = {
  submit: submitAdminTask,
  get: getAdminTask,
  list: listAdminTasks,
}

export { getAdminTask, listAdminTasks, submitAdminTask }
