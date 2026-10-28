import { computed, ref, watch } from 'vue'
import { type TaskStatus, usePagePagination, useQueryPolling } from '../../shared'
import { stripUndefined } from '@/api/stripUndefined'
import {
  useAdminTaskListResource,
  type AdminMonitorListItemModel,
  type ListAdminTasksParams,
} from '../resource'
import {
  ADMIN_MONITOR_PAGE_SIZE,
  ADMIN_MONITOR_CHART_PAGE_SIZE,
  ADMIN_MONITOR_REFRESH_INTERVAL_MS,
  ADMIN_MONITOR_RESULT_PREVIEW_ENABLED,
} from '../core/displayPolicy'

type TaskStatusStats = {
  running: number
  pending: number
  completed: number
  failed: number
  cancelled: number
  other: number
}

export type AdminTaskRow = AdminMonitorListItemModel

type AdminTaskListParamsInput = {
  status: TaskStatus | null
  keyword: string
  page: number
  pageSize: number
}

function buildAdminTaskListParams(input: AdminTaskListParamsInput): ListAdminTasksParams {
  const keyword = input.keyword.trim()
  const params: ListAdminTasksParams = stripUndefined({
    status: input.status || undefined,
    page: input.page,
    page_size: input.pageSize,
  })
  if (keyword) {
    if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(keyword))
      params.user_public_id = keyword
    else params.keyword = keyword
  }
  return params
}

export function useAdminTaskList() {
  const pageState = ref(1)
  const filterStatus = ref<TaskStatus | null>(null)
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const hasActiveTasks = ref(true)
  const pageSize = computed(() => ADMIN_MONITOR_PAGE_SIZE)
  const resultPreviewEnabled = computed(() => ADMIN_MONITOR_RESULT_PREVIEW_ENABLED)

  function resolveParams(): ListAdminTasksParams {
    return buildAdminTaskListParams({
      status: filterStatus.value,
      keyword: appliedSearchKeyword.value,
      page: pageState.value,
      pageSize: pageSize.value,
    })
  }

  const polling = useQueryPolling({
    visibleInterval: ADMIN_MONITOR_REFRESH_INTERVAL_MS,
    hiddenInterval: false,
    isEnabled: () => hasActiveTasks.value,
  })

  const queryOptions = {
    query: {
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  }
  const tasksQuery = useAdminTaskListResource(computed(resolveParams), queryOptions)
  const chartQuery = useAdminTaskListResource(
    computed(() => ({
      ...resolveParams(),
      page: 1,
      page_size: ADMIN_MONITOR_CHART_PAGE_SIZE,
    })),
    queryOptions,
  )
  const chartRows = computed(() => chartQuery.data.value?.list ?? [])

  const tasks = computed(() => tasksQuery.data.value?.list || [])
  const paginationTotalRecords = computed(() => tasksQuery.data.value?.total)
  const total = computed(() => paginationTotalRecords.value ?? 0)
  const loading = computed(() => tasksQuery.isLoading.value)
  const paginationPending = computed(
    () => tasksQuery.isLoading.value || tasksQuery.isFetching.value,
  )
  const pagination = usePagePagination({
    currentPage: pageState,
    totalRecords: paginationTotalRecords,
    pageSize,
    pending: paginationPending,
  })
  const isError = computed(() => tasksQuery.isError.value)
  const error = computed(() => (isError.value ? (tasksQuery.error.value ?? null) : null))
  const taskStats = computed<TaskStatusStats>(() => {
    const data = tasksQuery.data.value
    return {
      pending: data?.pending_count ?? 0,
      running: data?.running_count ?? 0,
      completed: data?.completed_count ?? 0,
      failed: data?.failed_count ?? 0,
      cancelled: data?.cancelled_count ?? 0,
      other: data?.other_count ?? 0,
    }
  })

  watch(
    () => {
      // Independent requests can finish out of order; stop only after both
      // snapshots confirm idle, including a chart response that arrives late.
      return [tasksQuery.data.value, chartQuery.data.value].some(
        (data) => !data || (data.pending_count ?? 0) > 0 || (data.running_count ?? 0) > 0,
      )
    },
    (nextHasActiveTasks) => {
      hasActiveTasks.value = nextHasActiveTasks
    },
    { immediate: true, flush: 'sync' },
  )

  function handleSearch() {
    pagination.reset()
    appliedSearchKeyword.value = searchKeyword.value.trim()
  }

  watch(
    [filterStatus],
    () => {
      pagination.reset()
    },
    { flush: 'sync' },
  )

  watch(
    pageSize,
    () => {
      pagination.reset()
    },
    { flush: 'sync' },
  )

  // One visible-resume drive refreshes both independent snapshots.
  watch(polling.isPageVisible, (isVisible, wasVisible) => {
    if (!isVisible || wasVisible || !hasActiveTasks.value) return
    loadTasks()
  })

  function loadTasks() {
    void tasksQuery.refetch()
    void chartQuery.refetch()
  }

  return {
    tasks,
    chartRows,
    total,
    loading,
    isError,
    error,
    pagination,
    pageSize,
    resultPreviewEnabled,
    taskStats,
    filterStatus,
    searchKeyword,
    loadTasks,
    handleSearch,
  }
}
