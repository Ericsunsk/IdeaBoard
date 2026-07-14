import { computed, type MaybeRef, type MaybeRefOrGetter } from 'vue'
import { useGetAdminStatistics } from '../../../shared/api/generated-query/admin-statistics/admin-statistics'
import {
  type ApiResponseAdminStatistics,
  GetAdminStatisticsRange,
  type AdminStatistics,
  type GetAdminStatisticsParams,
  type GetAdminStatisticsRange as GetAdminStatisticsRangeValue,
} from '../../../shared/api/generated-query/model'
import { createApiDataSelector } from '../../../shared/api/apiResponse'

type AdminStatisticsRefetchInterval = number | false | (() => number | false)

export type AdminStatisticsResourceOptions = {
  enabled?: MaybeRefOrGetter<boolean>
  refetchInterval?: AdminStatisticsRefetchInterval
  refetchIntervalInBackground?: boolean
  refetchOnWindowFocus?: boolean
  staleTime?: number
  keepPreviousData?: boolean
}

const ADMIN_STATISTICS_STALE_TIME_MS = 60_000
const selectAdminStatistics = createApiDataSelector<AdminStatistics>('加载统计数据失败')

export { GetAdminStatisticsRange }
export type { AdminStatistics, GetAdminStatisticsRangeValue }

export function useAdminStatisticsResource(
  params?: MaybeRef<GetAdminStatisticsParams>,
  options: AdminStatisticsResourceOptions = {},
) {
  const query = useGetAdminStatistics<AdminStatistics>(params, {
    query: createAdminStatisticsQueryOptions(options),
  })
  const statistics = computed<AdminStatistics | null>(() => query.data.value ?? null)
  const isSettled = computed(() => query.isFetched.value || query.isError.value)
  const error = computed(() => (query.isError.value ? (query.error.value ?? null) : null))

  return {
    query,
    statistics,
    isError: query.isError,
    isSettled,
    error,
  }
}

function createAdminStatisticsQueryOptions(options: AdminStatisticsResourceOptions) {
  return {
    select: selectAdminStatistics,
    staleTime: options.staleTime ?? ADMIN_STATISTICS_STALE_TIME_MS,
    refetchIntervalInBackground: options.refetchIntervalInBackground ?? false,
    refetchOnWindowFocus: options.refetchOnWindowFocus ?? false,
    ...(options.enabled !== undefined ? { enabled: options.enabled } : {}),
    ...(options.refetchInterval !== undefined ? { refetchInterval: options.refetchInterval } : {}),
    ...(options.keepPreviousData
      ? {
          placeholderData: (previous: ApiResponseAdminStatistics | undefined) => previous,
        }
      : {}),
  }
}
