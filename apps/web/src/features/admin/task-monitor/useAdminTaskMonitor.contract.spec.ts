import { computed, nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ListAdminTasksStatus,
  type ApiResponseMonitor,
  type ApiResponseTaskList,
  type MonitorData,
  type MonitorStats,
  type TaskMonitorSetting,
  type Task,
} from '../../../shared/api/generated-query/model'
import { useAdminTaskList } from './useAdminTaskList'
import { useAdminTaskRuntimeSnapshot } from './useAdminTaskRuntimeSnapshot'

const mockState = vi.hoisted(() => ({
  getAdminMonitorReturn: undefined as unknown,
  listAdminTasksReturn: undefined as unknown,
  pageVisible: undefined as unknown,
  pollingOptions: [] as unknown[],
  streamReturn: undefined as unknown,
  taskMonitorSettings: undefined as unknown as { value: TaskMonitorSetting },
}))

vi.mock('../../../shared/api/generated-query/admin-tasks/admin-tasks', () => ({
  useListAdminTasks: vi.fn(() => mockState.listAdminTasksReturn),
}))

vi.mock('../../../shared/api/generated-query/admin-monitor/admin-monitor', () => ({
  useGetAdminMonitor: vi.fn(() => mockState.getAdminMonitorReturn),
}))

vi.mock('../shared/composables/useAdminQueryPolling', () => ({
  useAdminQueryPolling: vi.fn(
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

vi.mock('../system-settings/taskMonitorSettingsResource', () => ({
  useAdminTaskMonitorSettingQuery: vi.fn(() => ({
    settings: mockState.taskMonitorSettings,
  })),
}))

vi.mock('./useAdminTaskMonitorStream', () => ({
  useAdminTaskMonitorStream: vi.fn(() => mockState.streamReturn),
}))

beforeEach(() => {
  mockState.getAdminMonitorReturn = undefined
  mockState.listAdminTasksReturn = undefined
  mockState.pageVisible = ref(true)
  mockState.pollingOptions = []
  mockState.streamReturn = undefined
  mockState.taskMonitorSettings = ref({
    list_page_size: 12,
    auto_refresh_enabled: true,
    refresh_interval_seconds: 4,
    result_preview_enabled: true,
  })
})

describe('useTaskMonitor polling', () => {
  it('uses aggregate pending/running counts instead of current page rows', async () => {
    const data = ref<ApiResponseTaskList>()
    mockState.listAdminTasksReturn = {
      data,
      error: ref(null),
      isError: ref(false),
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    useAdminTaskList()
    const polling = mockState.pollingOptions[0] as { isEnabled: () => boolean }

    data.value = makeTaskList([makeTask(ListAdminTasksStatus.completed, 1)], {
      pending: 1,
      running: 0,
    })
    await nextTick()

    expect(polling.isEnabled()).toBe(true)

    data.value = makeTaskList([makeTask(ListAdminTasksStatus.running, 2)], {
      pending: 0,
      running: 0,
    })
    await nextTick()

    expect(polling.isEnabled()).toBe(false)
  })

  it('exposes task list API errors so the page does not render them as an empty list', () => {
    const error = new Error('task list failed')
    mockState.listAdminTasksReturn = {
      data: ref<ApiResponseTaskList>(),
      error: ref(error),
      isError: ref(true),
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    const monitor = useAdminTaskList()

    expect(monitor.isError.value).toBe(true)
    expect(monitor.error.value).toBe(error)
  })

  it('uses task monitor settings for page size and refresh interval', () => {
    const data = ref<ApiResponseTaskList>()
    mockState.taskMonitorSettings.value = {
      list_page_size: 24,
      auto_refresh_enabled: true,
      refresh_interval_seconds: 10,
      result_preview_enabled: false,
    }
    mockState.listAdminTasksReturn = {
      data,
      error: ref(null),
      isError: ref(false),
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    const monitor = useAdminTaskList()
    const polling = mockState.pollingOptions[0] as { visibleInterval: () => number | false }

    expect(monitor.pageSize.value).toBe(24)
    expect(monitor.resultPreviewEnabled.value).toBe(false)
    expect(polling.visibleInterval()).toBe(10_000)
  })
})

describe('useTaskRuntimeMonitor SSE fallback', () => {
  it('keeps stale monitor data and exposes retry when both stream and REST are degraded', async () => {
    const sseResponse = makeMonitorResponse(50, 20, 30)
    const monitorData = ref<ApiResponseMonitor>()
    const refetch = vi.fn()
    const reconnect = vi.fn()
    const streamStatus = ref('connected')
    const streamSnapshot = ref<ApiResponseMonitor | null>(sseResponse)
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
      data: ref<ApiResponseMonitor>(),
      error: ref(error),
      isError: ref(true),
      refetch: vi.fn(),
    }
    mockState.streamReturn = {
      isConnected: computed(() => streamStatus.value === 'connected'),
      snapshot: ref<ApiResponseMonitor | null>(null),
      status: streamStatus,
      reconnect: vi.fn(),
    }

    const monitor = useAdminTaskRuntimeSnapshot()

    expect(monitor.isError.value).toBe(true)
    expect(monitor.error.value).toBe(error)
  })
})

function makeTask(status: string, id: number): Task {
  return {
    id,
    task_id: `task-${id}`,
    correlation_id: `task-${id}`,
    user_id: 1,
    prompt: `prompt-${id}`,
    priority: 2,
    status,
    created_at: '2026-06-10T00:00:00.000Z',
    updated_at: '2026-06-10T00:00:00.000Z',
  }
}

function makeTaskList(
  list: Task[],
  counts: { pending: number; running: number },
): ApiResponseTaskList {
  return {
    code: 0,
    msg: 'ok',
    data: {
      list,
      total: list.length,
      pending_count: counts.pending,
      running_count: counts.running,
      completed_count: 0,
      failed_count: 0,
      cancelled_count: 0,
      other_count: 0,
    },
  }
}

function makeMonitorResponse(
  globalQueueLen: number,
  pending: number,
  running: number,
): ApiResponseMonitor {
  return {
    code: 0,
    msg: 'ok',
    data: {
      monitor: makeMonitorData(globalQueueLen),
      stats: makeMonitorStats(pending, running),
    },
  }
}

function makeMonitorData(globalQueueLen: number): MonitorData {
  return {
    global_queue_len: globalQueueLen,
    wait_queue_len: 0,
    running_attempts: 0,
    account_loads: {},
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
