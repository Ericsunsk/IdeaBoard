import type { MaybeRef } from 'vue'
import { getGetAdminMonitorQueryKey } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import {
  deleteAdminTasks,
  getListAdminTasksQueryKey,
  useListAdminTasks,
} from '../../../shared/api/generated-query/admin-tasks/admin-tasks'
import type {
  AdminTaskDeleteResult,
  ApiResponseAdminTaskListData,
  ListAdminTasksParams,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector, resolveApiData } from '../../../shared/api/apiResponse'

export type AdminTaskListQueryParams = ListAdminTasksParams
export type AdminTask = ApiResponseAdminTaskListData['list'][number]

type UseAdminTaskListResourceOptions = Parameters<
  typeof useListAdminTasks<ApiResponseAdminTaskListData>
>[1]

const selectAdminTaskList =
  createApiDataSelector<ApiResponseAdminTaskListData>('加载任务列表失败')

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

export async function deleteAdminTaskSelection(taskIDs: string[]): Promise<AdminTaskDeleteResult> {
  return resolveApiData(deleteAdminTasks({ task_ids: taskIDs }), '删除任务失败')
}
