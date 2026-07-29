import type { MaybeRef } from 'vue'
import { getGetAdminMonitorQueryKey } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import {
  cancelAdminTask,
  getAdminTask,
  getGetAdminTaskQueryKey,
  getListAdminTasksQueryKey,
  listAdminTasks,
  useGetAdminTask,
  useListAdminTasks,
} from '../../../shared/api/generated-query/admin-tasks/admin-tasks'
import type {
  AdminTaskExecutionAttempt,
  AdminTaskMonitorDetail,
  AdminTaskMonitorListItem,
  ApiResponseAdminTaskData,
  ApiResponseAdminTaskListData,
  ListAdminTasksParams,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'

export type AdminTaskListQueryParams = ListAdminTasksParams
export type AdminTaskMonitorDetailModel = AdminTaskMonitorDetail
export type AdminTaskMonitorListItemModel = AdminTaskMonitorListItem
export type AdminTaskMonitorExecutionAttempt = AdminTaskExecutionAttempt

type UseAdminTaskListResourceOptions = Parameters<
  typeof useListAdminTasks<ApiResponseAdminTaskListData>
>[1]

const selectAdminTaskList = createApiDataSelector<ApiResponseAdminTaskListData>('加载任务列表失败')
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

export function getAdminTaskDetailQueryKey(taskID: string) {
  return getGetAdminTaskQueryKey(taskID)
}

export async function getAdminTaskList(
  params: AdminTaskListQueryParams,
): Promise<ApiResponseAdminTaskListData> {
  return resolveApiData(listAdminTasks(params), '加载任务列表失败')
}

export function getAdminMonitorQueryKey() {
  return getGetAdminMonitorQueryKey()
}

export async function cancelAdminTaskByID(taskID: string): Promise<void> {
  await resolveApiData(cancelAdminTask(taskID), '取消任务失败')
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

export async function getAdminTaskDetail(taskID: string): Promise<AdminTaskMonitorDetailModel> {
  const data = await resolveApiData(getAdminTask(taskID), '加载任务详情失败')
  return data.task
}
