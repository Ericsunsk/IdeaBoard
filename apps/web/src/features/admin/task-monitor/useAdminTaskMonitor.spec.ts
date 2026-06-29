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

vi.mock('./taskMonitorSettingsResource', () => ({
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
  it('falls back to fresh REST data and clears stale SSE snapshot when stream degrades', async () => {
    const restResponse = makeMonitorResponse(3, 1, 2)
    const sseResponse = makeMonitorResponse(50, 20, 30)
    const monitorData = ref<ApiResponseMonitor>(restResponse)
    const refetch = vi.fn()
    const streamStatus = ref('connected')
    const streamSnapshot = ref<ApiResponseMonitor | null>(sseResponse)
    mockState.getAdminMonitorReturn = {
      data: monitorData,
      error: ref(null),
      isError: ref(false),
      refetch,
    }
    mockState.streamReturn = {
      isConnected: computed(() => streamStatus.value === 'connected'),
      snapshot: streamSnapshot,
      status: streamStatus,
    }

    const monitor = useAdminTaskRuntimeSnapshot()

    expect(monitor.monitorData.value?.global_queue_len).toBe(50)
    expect(monitor.monitorStats.value?.pending).toBe(20)

    monitorData.value = makeMonitorResponse(4, 2, 2)
    streamStatus.value = 'degraded'
    await nextTick()

    expect(streamSnapshot.value).toBeNull()
    expect(refetch).toHaveBeenCalledTimes(1)
    expect(monitor.monitorData.value?.global_queue_len).toBe(4)
    expect(monitor.monitorStats.value?.pending).toBe(2)
  })

  it('exposes REST monitor errors when the stream cannot provide live data', () => {
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
    user_id: 1,
    prompt: `prompt-${id}`,
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
