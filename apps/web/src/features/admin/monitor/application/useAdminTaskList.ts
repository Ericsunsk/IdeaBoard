import { computed, ref, watch } from 'vue'
import type { ListAdminTasksStatus as ListAdminTasksStatusValue } from '../../../../shared/domain/taskMetricTokens'
import { usePagePagination } from '../../shared/composables/usePagePagination'
import { stripUndefined } from '../../../../shared/api/stripUndefined'
import { keepPrevious } from '../../../../shared/query/queryDefaults'
import { useQueryPolling } from '../../shared/composables/useQueryPolling'
import {
  useAdminTaskListResource,
  type AdminMonitorListItemModel,
  type ListAdminTasksParams,
} from '../infrastructure/resource'
import {
  ADMIN_MONITOR_PAGE_SIZE,
  ADMIN_MONITOR_REFRESH_INTERVAL_MS,
  ADMIN_MONITOR_RESULT_PREVIEW_ENABLED,
} from '../domain/displayPolicy'

type TaskStatusStats = {
  running: number
  pending: number
  completed: number
  failed: number
  cancelled: number
  other: number
}

export type AdminTaskRow = AdminMonitorListItemModel

export type AdminTaskListParamsInput = {
  status: ListAdminTasksStatusValue | null
  keyword: string
  page: number
  pageSize: number
}

export function buildAdminTaskListParams(input: AdminTaskListParamsInput): ListAdminTasksParams {
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
  const filterStatus = ref<ListAdminTasksStatusValue | null>(null)
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

  const tasksQuery = useAdminTaskListResource(computed(resolveParams), {
    query: {
      ...keepPrevious(),
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  })

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
      const data = tasksQuery.data.value
      if (!data) return true
      return (data.pending_count ?? 0) > 0 || (data.running_count ?? 0) > 0
    },
    (nextHasActiveTasks) => {
      hasActiveTasks.value = nextHasActiveTasks
    },
    { immediate: true },
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

  watch(polling.isPageVisible, (isVisible, wasVisible) => {
    if (isVisible && !wasVisible && hasActiveTasks.value) void tasksQuery.refetch()
  })

  function loadTasks() {
    void tasksQuery.refetch()
  }

  return {
    tasks,
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
    appliedSearchKeyword,
    loadTasks,
    handleSearch,
  }
}
