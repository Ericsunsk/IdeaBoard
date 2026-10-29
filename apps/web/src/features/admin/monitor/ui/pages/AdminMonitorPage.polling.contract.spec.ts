import { QueryClient, VueQueryPlugin, focusManager } from '@tanstack/vue-query'
import { shallowMount } from '@vue/test-utils'
import { nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PaginatedControlTasks } from '@/api/generated-query/control/model'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { ADMIN_TASK_LIST_QUERY_KEY } from '../../resource'
import { ADMIN_MONITOR_REFRESH_INTERVAL_MS } from '../../core/displayPolicy'
import AdminMonitorPage from './AdminMonitorPage.vue'

const visibility = ref<'visible' | 'hidden'>('visible')

vi.mock('@vueuse/core', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@vueuse/core')>()),
  useDocumentVisibility: () => visibility,
}))
vi.mock('../../session/useAdminTaskRuntimeSnapshot', () => ({
  useAdminTaskRuntimeSnapshot: () => ({
    isStale: ref(false),
    isError: ref(false),
    error: ref(null),
    retry: vi.fn(),
  }),
}))
vi.mock('../../session/useAdminTaskCancel', () => ({
  useAdminTaskCancel: () => ({
    isCancellableTask: () => false,
    isCancellingTask: () => false,
    requestCancelTask: vi.fn(),
  }),
}))
vi.mock('../../session/useAdminMonitorFeedback', () => ({
  useAdminTaskCancellationFeedback: () => ({}),
  useAdminTaskLoadFeedback: () => ({
    snapshotLoadFailed: vi.fn(),
    listLoadFailed: vi.fn(),
    detailLoadFailed: vi.fn(),
  }),
}))
vi.mock('@/core/composables/useConfirmDialog', () => ({
  useConfirm: () => ({ confirm: vi.fn() }),
}))

function buildResponse(status: 'running' | 'completed'): PaginatedControlTasks {
  return {
    data: [{ task_id: 'task-1', service: 'render', status }],
    meta: {
      has_more: false,
      counts: {
        total: 1,
        pending_count: 0,
        running_count: status === 'running' ? 1 : 0,
        completed_count: status === 'completed' ? 1 : 0,
        failed_count: 0,
        cancelled_count: 0,
        other_count: 0,
      },
    },
  }
}

describe('AdminMonitorPage polling reconciliation', () => {
  let queryClient: QueryClient
  let unmount: (() => void) | undefined

  beforeEach(() => {
    vi.useFakeTimers()
    visibility.value = 'visible'
    focusManager.setFocused(true)
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity } },
    })
  })

  afterEach(() => {
    unmount?.()
    queryClient.clear()
    focusManager.setFocused(undefined)
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it.each([false, true])(
    'reconciles a late running chart after the table becomes idle (visibility resume: %s)',
    async (shouldResume) => {
      let resolveOldChart!: (response: PaginatedControlTasks) => void
      let chartRequests = 0
      let totalRequests = 0
      useMswHandlers(
        http.get('/api/control/v1/tasks', async ({ request }) => {
          totalRequests++
          if (new URL(request.url).searchParams.get('page_size') !== '100') {
            return HttpResponse.json(buildResponse('completed'))
          }
          chartRequests++
          if (chartRequests === 1) {
            const response = await new Promise<PaginatedControlTasks>((resolve) => {
              resolveOldChart = resolve
            })
            return HttpResponse.json(response)
          }
          return HttpResponse.json(buildResponse('completed'))
        }),
      )
      const view = shallowMount(AdminMonitorPage, {
        global: { plugins: [[VueQueryPlugin, { queryClient }]] },
      })
      unmount = () => view.unmount()
      await vi.waitFor(() => expect(chartRequests).toBe(1))
      resolveOldChart(buildResponse('running'))

      const chartData = () =>
        queryClient
          .getQueriesData<PaginatedControlTasks>({ queryKey: ADMIN_TASK_LIST_QUERY_KEY })
          .find(([key]) => (key.at(-1) as { page_size: number }).page_size === 100)?.[1]
      await vi.waitFor(() => expect(chartData()?.data[0]?.status).toBe('running'))

      if (shouldResume) {
        visibility.value = 'hidden'
        focusManager.setFocused(false)
        await nextTick()
        await vi.advanceTimersByTimeAsync(ADMIN_MONITOR_REFRESH_INTERVAL_MS * 2)
        expect(chartRequests).toBe(1)
        visibility.value = 'visible'
        focusManager.setFocused(true)
        await nextTick()
        await vi.advanceTimersByTimeAsync(1)
      } else {
        await vi.advanceTimersByTimeAsync(ADMIN_MONITOR_REFRESH_INTERVAL_MS)
      }

      await vi.waitFor(() => expect(chartData()?.data[0]?.status).toBe('completed'))
      expect(chartRequests).toBe(2)
      // The other observer may already have scheduled its last interval.
      await vi.advanceTimersByTimeAsync(ADMIN_MONITOR_REFRESH_INTERVAL_MS)
      const requestsAtIdle = totalRequests
      await vi.advanceTimersByTimeAsync(ADMIN_MONITOR_REFRESH_INTERVAL_MS * 3)
      visibility.value = 'hidden'
      await nextTick()
      visibility.value = 'visible'
      await nextTick()
      await vi.advanceTimersByTimeAsync(1)
      expect(totalRequests).toBe(requestsAtIdle)
    },
  )
})
