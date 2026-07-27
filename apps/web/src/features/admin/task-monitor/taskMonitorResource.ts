import type { MaybeRef } from 'vue'
import { getGetAdminMonitorQueryKey } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import {
  deleteAdminTasks,
  getAdminTask,
  getListAdminTasksQueryKey,
  useGetAdminTask,
  useListAdminTasks,
} from '../../../shared/api/generated-query/admin-tasks/admin-tasks'
import type {
  AdminTask as AdminTaskModel,
  AdminTaskDeleteResult,
  ApiResponseAdminTaskData,
  ApiResponseAdminTaskListData,
  ListAdminTasksParams,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'

export type AdminTaskListQueryParams = ListAdminTasksParams
export type AdminTask = AdminTaskModel
export type AdminTaskExecutionAttempt = NonNullable<AdminTask['execution_attempts']>[number]

type UseAdminTaskListResourceOptions = Parameters<
  typeof useListAdminTasks<ApiResponseAdminTaskListData>
>[1]

const selectAdminTaskList =
  createApiDataSelector<ApiResponseAdminTaskListData>('加载任务列表失败')
const selectAdminTaskDetail = createApiDataSelector<ApiResponseAdminTaskData>('加载任务详情失败')

export function useAdminTaskListResource(
  params: MaybeRef<AdminTaskListQueryParams>,
  options?: UseAdminTaskListResourceOptions,
) {
  return useListAdminTasks<ApiResponseAdminTaskListData>(params, {
    ...options,
    query: {
      ...options?.query,
      select: selectAdminTaskList,
    },
  })
}

export function getAdminTaskListQueryKey() {
  return getListAdminTasksQueryKey()
}

export function getAdminMonitorQueryKey() {
  return getGetAdminMonitorQueryKey()
}

type UseAdminTaskDetailResourceOptions = Parameters<
  typeof useGetAdminTask<ApiResponseAdminTaskData>
>[1]

export function useAdminTaskDetailResource(
  taskID: MaybeRef<string>,
  options?: UseAdminTaskDetailResourceOptions,
) {
  return useGetAdminTask<ApiResponseAdminTaskData>(taskID, {
    ...options,
    query: {
      ...options?.query,
      select: selectAdminTaskDetail,
    },
  })
}

export async function getAdminTaskDetail(taskID: string): Promise<AdminTask> {
  const data = await resolveApiData(getAdminTask(taskID), '加载任务详情失败')
  return data.task
}

export async function deleteAdminTaskSelection(taskIDs: string[]): Promise<AdminTaskDeleteResult> {
  return resolveApiData(deleteAdminTasks({ task_ids: taskIDs }), '删除任务失败')
}
