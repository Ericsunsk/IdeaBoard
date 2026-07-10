import type { MaybeRef } from 'vue'
import { getGetAdminMonitorQueryKey } from '../../../shared/api/generated-query/admin-monitor/admin-monitor'
import {
  deleteAdminTasks,
  getListAdminTasksQueryKey,
  useListAdminTasks,
} from '../../../shared/api/generated-query/admin-tasks/admin-tasks'
import type {
  AdminTaskDeleteResult,
  ApiResponseTaskList,
  ListAdminTasksParams,
} from '../../../shared/api/generated-query/model'
import { requireApiData } from '../shared/queries/adminApiResponse'

export type AdminTaskListQueryParams = ListAdminTasksParams

type UseAdminTaskListResourceOptions = Parameters<
  typeof useListAdminTasks<ApiResponseTaskList>
>[1]

export function useAdminTaskListResource(
  params: MaybeRef<AdminTaskListQueryParams>,
  options?: UseAdminTaskListResourceOptions,
) {
  return useListAdminTasks<ApiResponseTaskList>(params, options)
}

export function getAdminTaskListQueryKey() {
  return getListAdminTasksQueryKey()
}

export function getAdminMonitorQueryKey() {
  return getGetAdminMonitorQueryKey()
}

export async function deleteAdminTaskSelection(
  taskIDs: string[],
): Promise<AdminTaskDeleteResult> {
  const res = await deleteAdminTasks({ task_ids: taskIDs })
  return requireApiData(res, '删除任务失败')
}
