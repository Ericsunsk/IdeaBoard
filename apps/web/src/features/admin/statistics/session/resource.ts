import { computed, toValue, type MaybeRef, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { requireContractData } from '../../../../api/contractKernel'
import { keepPrevious, STALE_TIME_MS } from '../../../../core/query/queryDefaults'
import { getControlStatistics } from '../../../../api/generated-query/control/control-platform/control-platform'
import type { GetControlStatisticsParams } from '../../../../api/generated-query/control/model'

// Admin statistics boundary (final cut: Control Plane read). The legacy
// `GET /api/admin/statistics` route group is deleted; the same handler now
// serves `GET /api/control/v1/platform/statistics` with the unified
// contract envelope (`{ data }` success / `{ error: { code, message,
// request_id } }` errors), so this boundary keeps its local view types and
// rides the generated `getControlStatistics` hook (wire-identical to the
// retired hand-rolled GET). Charts and presentations keep importing these
// names, never generated code.
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

type AdminStatisticsSummary = {
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

type AdminStatisticsTrendSegment = {
  label: string
  value: number
}

type AdminStatisticsTrendItem = {
  date: string
  label: string
  total: number
  captured_credits?: number
  segments: AdminStatisticsTrendSegment[]
}

type AdminStatisticsModelHealthItem = {
  name: string
  total: number
  completed: number
  failed: number
  failure_rate: number
  avg_seconds: number
}

type AdminStatisticsQualityTrendItem = {
  date: string
  label: string
  total: number
  success: number
  fail: number
  success_rate: number
}

type AdminStatisticsSlowModelsItem = {
  name: string
  avg_seconds: number
  p95_seconds: number
  samples: number
}

type AdminStatisticsBottleneckSourcesItem = {
  source: string
  label: string
  total: number
  failed: number
  avg_latency_ms: number
}

type AdminStatisticsFailureCategoriesItem = {
  category: string
  label: string
  total: number
  avg_latency_ms: number
}

type AdminStatisticsBackfillStatus = {
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

export type ApiResponseAdminStatistics = { data: AdminStatistics }

export type AdminStatisticsResourceOptions = {
  enabled?: MaybeRefOrGetter<boolean>
  refetchInterval?: number | false | (() => number | false)
  refetchIntervalInBackground?: boolean
  refetchOnWindowFocus?: boolean
  staleTime?: number
  keepPreviousData?: boolean
}

const ADMIN_STATISTICS_STALE_TIME_MS = STALE_TIME_MS.statistics

// Composed-query keys: the Control statistics op has no orval query-key
// helper, so this factory owns the key shape (never inline literals).
const adminStatisticsQueryKeys = {
  all: ['admin', 'statistics'] as const,
  byParams: (params?: MaybeRef<GetAdminStatisticsParams>) =>
    ['admin', 'statistics', params] as const,
}
function selectAdminStatistics(response: ApiResponseAdminStatistics): AdminStatistics {
  return requireContractData<AdminStatistics>(response, '加载统计数据失败')
}

function statisticsParams(
  params?: GetAdminStatisticsParams,
): GetControlStatisticsParams | undefined {
  const range = params?.range
  return range ? { range } : undefined
}

export function useAdminStatisticsResource(
  params?: MaybeRef<GetAdminStatisticsParams>,
  options: AdminStatisticsResourceOptions = {},
) {
  const query = useQuery<ApiResponseAdminStatistics, Error, AdminStatistics>({
    queryKey: adminStatisticsQueryKeys.byParams(params),
    queryFn: ({ signal }) =>
      getControlStatistics(statisticsParams(toValue(params)), {
        ...(signal == null ? {} : { signal }),
      }) as Promise<ApiResponseAdminStatistics>,
    select: selectAdminStatistics,
    staleTime: options.staleTime ?? ADMIN_STATISTICS_STALE_TIME_MS,
    refetchIntervalInBackground: options.refetchIntervalInBackground ?? false,
    refetchOnWindowFocus: options.refetchOnWindowFocus ?? false,
    ...(options.enabled !== undefined ? { enabled: options.enabled } : {}),
    ...(options.refetchInterval !== undefined ? { refetchInterval: options.refetchInterval } : {}),
    ...(options.keepPreviousData ? { ...keepPrevious() } : {}),
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
