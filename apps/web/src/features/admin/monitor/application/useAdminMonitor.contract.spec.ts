import { computed, nextTick, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TaskStatus } from '../../shared/lib/taskMetricTokens'
import type {
  ApiResponseAdminTaskListData,
  AdminMonitorListItem,
  ListAdminTasksParams,
  MonitorData,
  MonitorResponse,
  MonitorStats,
} from '../infrastructure/resource'
import { useAdminTaskList } from './useAdminTaskList'
import { useAdminTaskRuntimeSnapshot } from './useAdminTaskRuntimeSnapshot'
import { ADMIN_MONITOR_PAGE_SIZE } from '../domain/displayPolicy'

const mockState = vi.hoisted(() => ({
  getAdminMonitorReturn: undefined as unknown,
  listAdminTasksReturn: undefined as unknown,
  pageVisible: undefined as unknown,
  pollingOptions: [] as unknown[],
  streamReturn: undefined as unknown,
  taskListParams: undefined as MaybeRefOrGetter<ListAdminTasksParams> | undefined,
}))

vi.mock('../infrastructure/resource', async (importOriginal) => {
  const original = await importOriginal<typeof import('../infrastructure/resource')>()
  return {
    ...original,
    useAdminTaskListResource: vi.fn((params: MaybeRefOrGetter<ListAdminTasksParams>) => {
      mockState.taskListParams = params
      return mockState.listAdminTasksReturn
    }),
    useAdminMonitorResource: vi.fn(() => mockState.getAdminMonitorReturn),
  }
})

vi.mock('../../shared/composables/useQueryPolling', () => ({
  useQueryPolling: vi.fn(
    (options: {
      isEnabled?: () => boolean
      visibleInterval: number | false | (() => number | false)
    }) => {
      mockState.pollingOptions.push(options)
      return {
        isPageVisible: mockState.pageVisible,
        refetchInterval: () => {
          if (options.isEnabled && !options.isEnabled()) return false
          return typeof options.visibleInterval === 'function'
            ? options.visibleInterval()
            : options.visibleInterval
        },
      }
    },
  ),
}))

vi.mock('./useAdminMonitorStream', () => ({
  useAdminMonitorStream: vi.fn(() => mockState.streamReturn),
}))

beforeEach(() => {
  mockState.getAdminMonitorReturn = undefined
  mockState.listAdminTasksReturn = undefined
  mockState.pageVisible = ref(true)
  mockState.pollingOptions = []
  mockState.streamReturn = undefined
  mockState.taskListParams = undefined
})

describe('useMonitor polling', () => {
  it('uses aggregate pending/running counts instead of current page rows', async () => {
    const data = ref<ApiResponseAdminTaskListData>()
    mockState.listAdminTasksReturn = {
      data,
      error: ref(null),
      isError: ref(false),
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    useAdminTaskList()
    const polling = mockState.pollingOptions[0] as { isEnabled: () => boolean }

    data.value = makeTaskList([makeTask(TaskStatus.completed, 1)], {
      pending: 1,
      running: 0,
    })
    await nextTick()

    expect(polling.isEnabled()).toBe(true)

    data.value = makeTaskList([makeTask(TaskStatus.running, 2)], {
      pending: 0,
      running: 0,
    })
    await nextTick()

    expect(polling.isEnabled()).toBe(false)
  })

  it('exposes task list API errors so the page does not render them as an empty list', () => {
    const error = new Error('task list failed')
    mockState.listAdminTasksReturn = {
      data: ref<ApiResponseAdminTaskListData>(),
      error: ref(error),
      isError: ref(true),
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    const monitor = useAdminTaskList()

    expect(monitor.isError.value).toBe(true)
    expect(monitor.error.value).toBe(error)
  })

  it('uses the fixed task monitor display policy', () => {
    const data = ref<ApiResponseAdminTaskListData>()
    mockState.listAdminTasksReturn = {
      data,
      error: ref(null),
      isError: ref(false),
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    const monitor = useAdminTaskList()
    const polling = mockState.pollingOptions[0] as { visibleInterval: number | false }

    expect(monitor.pageSize.value).toBe(ADMIN_MONITOR_PAGE_SIZE)
    expect(monitor.resultPreviewEnabled.value).toBe(true)
    expect(polling.visibleInterval).toBe(4_000)
  })

  it('uses the shared page boundary for pending navigation, clamp, and filter reset', () => {
    const data = ref<ApiResponseAdminTaskListData>(makeTaskList([], { pending: 0, running: 0 }, 35))
    const isFetching = ref(false)
    mockState.listAdminTasksReturn = {
      data,
      error: ref(null),
      isError: ref(false),
      isLoading: ref(false),
      isFetching,
      refetch: vi.fn(),
    }

    const monitor = useAdminTaskList()
    const pagination = monitor.pagination

    expect(pagination.totalPages.value).toBe(3)
    expect(pagination.next()).toBe(true)
    expect(pagination.currentPage.value).toBe(2)
    expect(toValue(mockState.taskListParams)?.page).toBe(2)

    isFetching.value = true
    expect(pagination.canPrevious.value).toBe(false)
    expect(pagination.canNext.value).toBe(false)
    expect(pagination.previous()).toBe(false)
    expect(pagination.next()).toBe(false)
    expect(toValue(mockState.taskListParams)?.page).toBe(2)

    isFetching.value = false
    expect(pagination.next()).toBe(true)
    data.value = makeTaskList([], { pending: 0, running: 0 }, 20)
    expect(pagination.currentPage.value).toBe(2)

    monitor.filterStatus.value = TaskStatus.failed
    expect(pagination.currentPage.value).toBe(1)
  })

  it('keeps the current page when a replacement request fails without data', () => {
    const data = ref<ApiResponseAdminTaskListData | undefined>(
      makeTaskList([], { pending: 0, running: 0 }, 25),
    )
    mockState.listAdminTasksReturn = {
      data,
      error: ref<Error | null>(null),
      isError: ref(false),
      isLoading: ref(false),
      isFetching: ref(false),
      refetch: vi.fn(),
    }

    const monitor = useAdminTaskList()
    monitor.pagination.next()
    data.value = undefined

    expect(monitor.pagination.currentPage.value).toBe(2)
    expect(monitor.pagination.totalPages.value).toBe(2)
    expect(toValue(mockState.taskListParams)?.page).toBe(2)
  })
})

describe('useTaskRuntimeMonitor SSE fallback', () => {
  it('keeps stale monitor data and exposes retry when both stream and REST are degraded', async () => {
    const sseResponse = makeMonitorResponse(50, 20, 30)
    const monitorData = ref<MonitorResponse>()
    const refetch = vi.fn()
    const reconnect = vi.fn()
    const streamStatus = ref('connected')
    const streamSnapshot = ref<MonitorResponse | null>(sseResponse)
    mockState.getAdminMonitorReturn = {
      data: monitorData,
      error: ref(new Error('monitor failed')),
      isError: ref(true),
      refetch,
    }
    mockState.streamReturn = {
      isConnected: computed(() => streamStatus.value === 'connected'),
      snapshot: streamSnapshot,
      status: streamStatus,
      reconnect,
    }

    const monitor = useAdminTaskRuntimeSnapshot()

    expect(monitor.monitorData.value?.global_queue_len).toBe(50)
    expect(monitor.monitorStats.value?.pending).toBe(20)
    expect(monitor.isStale.value).toBe(false)

    streamStatus.value = 'degraded'
    await nextTick()

    expect(refetch).toHaveBeenCalledTimes(1)
    expect(streamSnapshot.value).toStrictEqual(sseResponse)
    expect(monitor.monitorData.value?.global_queue_len).toBe(50)
    expect(monitor.monitorStats.value?.pending).toBe(20)
    expect(monitor.isError.value).toBe(false)
    expect(monitor.isStale.value).toBe(true)

    await monitor.retry()

    expect(refetch).toHaveBeenCalledTimes(2)
    expect(reconnect).toHaveBeenCalledTimes(1)
  })

  it('exposes REST monitor errors when no stale stream data is available', () => {
    const error = new Error('monitor failed')
    const streamStatus = ref('degraded')
    mockState.getAdminMonitorReturn = {
      data: ref<MonitorResponse>(),
      error: ref(error),
      isError: ref(true),
      refetch: vi.fn(),
    }
    mockState.streamReturn = {
      isConnected: computed(() => streamStatus.value === 'connected'),
      snapshot: ref<MonitorResponse | null>(null),
      status: streamStatus,
      reconnect: vi.fn(),
    }

    const monitor = useAdminTaskRuntimeSnapshot()

    expect(monitor.isError.value).toBe(true)
    expect(monitor.error.value).toBe(error)
  })
})

function makeTask(status: string, id: number): AdminMonitorListItem {
  return {
    task_id: `task-${id}`,
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: `prompt-${id}`,
    service: 'render',
    credits: 0,
    priority: 2,
    status,
    error_msg: '',
    result: '',
    created_at: '2026-06-10T00:00:00.000Z',
    completed_at: null,
  }
}

function makeTaskList(
  list: AdminMonitorListItem[],
  counts: { pending: number; running: number },
  total = list.length,
): ApiResponseAdminTaskListData {
  return {
    list,
    total,
    pending_count: counts.pending,
    running_count: counts.running,
    completed_count: 0,
    failed_count: 0,
    cancelled_count: 0,
    other_count: 0,
  }
}

function makeMonitorResponse(
  globalQueueLen: number,
  pending: number,
  running: number,
): MonitorResponse {
  return {
    monitor: makeMonitorData(globalQueueLen),
    stats: makeMonitorStats(pending, running),
  }
}

function makeMonitorData(globalQueueLen: number): MonitorData {
  return {
    global_queue_len: globalQueueLen,
    wait_queue_len: 0,
    running_attempts: 0,
    execution_pressure: [],
    golden_signals: {
      window_seconds: 3600,
      sample_count: 0,
      provider_attempts: 0,
      provider_error_count: 0,
      provider_error_rate: 0,
      queue_delay_ms: { samples: 0, avg_ms: 0, p95_ms: 0, max_ms: 0 },
      provider_latency_ms: { samples: 0, avg_ms: 0, p95_ms: 0, max_ms: 0 },
      finalize_latency_ms: { samples: 0, avg_ms: 0, p95_ms: 0, max_ms: 0 },
    },
  }
}

function makeMonitorStats(pending: number, running: number): MonitorStats {
  return {
    pending,
    running,
    completed: 0,
    failed: 0,
    cancelled: 0,
  }
}
