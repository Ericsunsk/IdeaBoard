import { computed, nextTick, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ListAdminTasksStatus,
  type ApiResponseMonitor,
  type ApiResponseTaskList,
  type MonitorData,
  type MonitorStats,
  type Task,
} from '../../../../shared/api/generated-query/model'
import { useTaskMonitor, TASK_MONITOR_PAGE_SIZE } from './useTaskMonitor'
import { useTaskRuntimeMonitor } from './useTaskRuntimeMonitor'

const mockState = vi.hoisted(() => ({
  adminMockMode: undefined as unknown,
  getAdminMonitorReturn: undefined as unknown,
  listAdminTasksReturn: undefined as unknown,
  mockTasks: [] as unknown[],
  pageVisible: undefined as unknown,
  pollingOptions: [] as unknown[],
  streamReturn: undefined as unknown,
}))

vi.mock('../../../../shared/api/generated-query/admin-tasks/admin-tasks', () => ({
  useListAdminTasks: vi.fn(() => mockState.listAdminTasksReturn),
}))

vi.mock('../../../../shared/api/generated-query/admin-monitor/admin-monitor', () => ({
  useGetAdminMonitor: vi.fn(() => mockState.getAdminMonitorReturn),
}))

vi.mock('../../shared/composables/useAdminQueryPolling', () => ({
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

vi.mock('../../shared/lib/adminMockData', () => ({
  createMockAdminTasks: vi.fn(() => mockState.mockTasks),
  mockMonitorData: {
    global_queue_len: 900,
    wait_queue_len: 0,
    running_attempts: 0,
    account_loads: {},
    execution_pressure: [],
  },
  mockMonitorStats: {
    pending: 9,
    running: 0,
    completed: 0,
    failed: 0,
    cancelled: 0,
  },
  useAdminMockMode: vi.fn(() => mockState.adminMockMode),
}))

vi.mock('./useTaskMonitorStream', () => ({
  useTaskMonitorStream: vi.fn(() => mockState.streamReturn),
}))

beforeEach(() => {
  mockState.adminMockMode = ref(false)
  mockState.getAdminMonitorReturn = undefined
  mockState.listAdminTasksReturn = undefined
  mockState.mockTasks = []
  mockState.pageVisible = ref(true)
  mockState.pollingOptions = []
  mockState.streamReturn = undefined
})

describe('useTaskMonitor polling', () => {
  it('uses aggregate pending/running counts instead of current page rows', async () => {
    const data = ref<ApiResponseTaskList>()
    mockState.listAdminTasksReturn = {
      data,
      isLoading: ref(false),
      refetch: vi.fn(),
    }

    useTaskMonitor()
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

  it('keeps mock-mode polling based on the visible mock page', async () => {
    mockState.adminMockMode = ref(true)
    mockState.listAdminTasksReturn = {
      data: ref<ApiResponseTaskList>(),
      isLoading: ref(false),
      refetch: vi.fn(),
    }
    mockState.mockTasks = [
      ...Array.from({ length: TASK_MONITOR_PAGE_SIZE }, (_, index) =>
        makeTask(ListAdminTasksStatus.completed, index + 1),
      ),
      makeTask(ListAdminTasksStatus.pending, TASK_MONITOR_PAGE_SIZE + 1),
    ]

    const monitor = useTaskMonitor()
    const polling = mockState.pollingOptions[0] as { isEnabled: () => boolean }

    expect(polling.isEnabled()).toBe(false)

    monitor.page.value = 2
    await nextTick()

    expect(polling.isEnabled()).toBe(true)
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
      refetch,
    }
    mockState.streamReturn = {
      isConnected: computed(() => streamStatus.value === 'connected'),
      snapshot: streamSnapshot,
      status: streamStatus,
    }

    const monitor = useTaskRuntimeMonitor()

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
