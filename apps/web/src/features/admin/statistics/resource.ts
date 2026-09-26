import { computed, toValue, type MaybeRef, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api } from '../../../shared/api/client'
import { CONTROL_API_PREFIX } from '../../../shared/api/controlMutator'
import {
  createApiDataSelector,
  type ApiResponseWithData,
} from '../../../shared/api/apiResponse'
import { getGetControlStatisticsUrl } from '../../../shared/api/generated-query/control/control-platform/control-platform'

// Admin statistics boundary (final cut: Control Plane read). The legacy
// `GET /api/admin/statistics` route group is deleted; the same handler now
// serves `GET /api/control/v1/platform/statistics` as a guarded proxy with
// the legacy `{code, msg, data}` envelope untouched, so this boundary keeps
// its local view types and only repoints transport at the generated Control
// path. Charts and presentations keep importing these names, never generated
// code.
export const GetAdminStatisticsRange = {
  '1d': '1d',
  '7d': '7d',
  '30d': '30d',
  '1y': '1y',
} as const

export type GetAdminStatisticsRange =
  (typeof GetAdminStatisticsRange)[keyof typeof GetAdminStatisticsRange]
export type GetAdminStatisticsRangeValue = GetAdminStatisticsRange

export type GetAdminStatisticsParams = {
  range?: GetAdminStatisticsRange
}

export type AdminStatisticsSummary = {
  total_calls: number
  today_calls: number
  today_success: number
  today_failed: number
  today_cancelled: number
  yesterday_calls: number
  yesterday_success: number
  yesterday_failed: number
  yesterday_cancelled: number
  this_month_calls: number
  this_month_cancelled: number
  pending: number
  running: number
  completed: number
  failed: number
  cancelled: number
}

export type AdminStatisticsTrendSegment = {
  label: string
  value: number
}

export type AdminStatisticsTrendItem = {
  date: string
  label: string
  total: number
  captured_credits?: number
  segments: AdminStatisticsTrendSegment[]
}

export type AdminStatisticsModelHealthItem = {
  name: string
  total: number
  completed: number
  failed: number
  failure_rate: number
  avg_seconds: number
}

export type AdminStatisticsQualityTrendItem = {
  date: string
  label: string
  total: number
  success: number
  fail: number
  success_rate: number
}

export type AdminStatisticsSlowModelsItem = {
  name: string
  avg_seconds: number
  p95_seconds: number
  samples: number
}

export type AdminStatisticsBottleneckSourcesItem = {
  source: string
  label: string
  total: number
  failed: number
  avg_latency_ms: number
}

export type AdminStatisticsFailureCategoriesItem = {
  category: string
  label: string
  total: number
  avg_latency_ms: number
}

export type AdminStatisticsBackfillStatus = {
  range_start: string
  range_end: string
  missing_days: number
  backfilled_days: number
}

export type AdminStatistics = {
  summary: AdminStatisticsSummary
  trend: AdminStatisticsTrendItem[]
  model_health: AdminStatisticsModelHealthItem[]
  quality_trend: AdminStatisticsQualityTrendItem[]
  slow_models: AdminStatisticsSlowModelsItem[]
  bottleneck_sources: AdminStatisticsBottleneckSourcesItem[]
  failure_categories: AdminStatisticsFailureCategoriesItem[]
  backfill_status: AdminStatisticsBackfillStatus
}

export type ApiResponseAdminStatistics = ApiResponseWithData<AdminStatistics>

export type AdminStatisticsResourceOptions = {
  enabled?: MaybeRefOrGetter<boolean>
  refetchInterval?: number | false | (() => number | false)
  refetchIntervalInBackground?: boolean
  refetchOnWindowFocus?: boolean
  staleTime?: number
  keepPreviousData?: boolean
}

const ADMIN_STATISTICS_STALE_TIME_MS = 60_000
const selectAdminStatistics = createApiDataSelector<AdminStatistics>('加载统计数据失败')

function statisticsUrl(params?: GetAdminStatisticsParams): string {
  // Generated Control path (`/platform/statistics[?range=]`); the shared
  // `api.request` mounts only `/api`, so the `/control/v1` prefix is added
  // here exactly like `controlMutator` does for generated callers.
  return `${CONTROL_API_PREFIX}${getGetControlStatisticsUrl(params?.range ? { range: params.range } : undefined)}`
}

export function useAdminStatisticsResource(
  params?: MaybeRef<GetAdminStatisticsParams>,
  options: AdminStatisticsResourceOptions = {},
) {
  const query = useQuery<ApiResponseWithData<AdminStatistics>, Error, AdminStatistics>({
    queryKey: ['admin', 'statistics', params] as const,
    queryFn: ({ signal }) =>
      api.request<ApiResponseAdminStatistics>(statisticsUrl(toValue(params)), {
        method: 'GET',
        ...(signal == null ? {} : { signal }),
      }),
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
