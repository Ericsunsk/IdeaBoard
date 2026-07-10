import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminTaskDeletionFeedback } from '../feedback/useAdminFeedback'
import type { AdminConfirmRequest } from '../shared/composables/useAdminConfirm'
import { useAdminTaskSelectionDelete } from './useAdminTaskSelectionDelete'
import type { AdminTaskRow } from './useAdminTaskList'

const queryClientMock = vi.hoisted(() => ({
  invalidateQueries: vi.fn().mockResolvedValue(undefined),
}))

const taskMonitorResourceMocks = vi.hoisted(() => ({
  deleteAdminTaskSelection: vi.fn().mockResolvedValue({ deleted_count: 2 }),
  getAdminMonitorQueryKey: vi.fn(() => ['admin', 'monitor']),
  getAdminTaskListQueryKey: vi.fn(() => ['admin', 'tasks']),
}))

vi.mock('@tanstack/vue-query', () => ({
  useQueryClient: () => queryClientMock,
  useMutation: (options: {
    mutationFn: (input: string[]) => Promise<unknown>
    onSuccess?: (data: unknown, input: string[]) => Promise<unknown> | unknown
  }) => ({
    mutateAsync: async (input: string[]) => {
      const data = await options.mutationFn(input)
      await options.onSuccess?.(data, input)
      return data
    },
  }),
}))

vi.mock('./taskMonitorResource', () => taskMonitorResourceMocks)

describe('useAdminTaskSelectionDelete', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryClientMock.invalidateQueries.mockResolvedValue(undefined)
    taskMonitorResourceMocks.deleteAdminTaskSelection.mockResolvedValue({ deleted_count: 2 })
  })

  it('confirms, stops active selected tasks through the backend, and clears selection after deletion', async () => {
    const selectedTasks = ref<AdminTaskRow[]>([
      makeTask({ task_id: ' task-running ', status: 'running' }),
      makeTask({ task_id: 'task-completed', status: 'completed' }),
      makeTask({ task_id: 'task-running', status: 'running' }),
    ])
    const confirmRequests: AdminConfirmRequest[] = []
    const feedback = createFeedback()
    const commands = useAdminTaskSelectionDelete({
      selectedTasks,
      confirm: { request: (request) => confirmRequests.push(request) },
      feedback,
    })

    commands.requestDeleteSelectedTasks()

    expect(confirmRequests).toHaveLength(1)
    expect(confirmRequests[0]?.message).toContain('其中 2 个仍在等待或执行中')
    await confirmRequests[0]?.onConfirm()

    expect(taskMonitorResourceMocks.deleteAdminTaskSelection).toHaveBeenCalledWith([
      'task-running',
      'task-completed',
    ])
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['admin', 'tasks'] })
    expect(queryClientMock.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['admin', 'monitor'],
    })
    expect(selectedTasks.value).toEqual([])
    expect(feedback.tasksDeleted).toHaveBeenCalledWith(2)
  })

  it('warns instead of opening confirmation when no task is selected', () => {
    const selectedTasks = ref<AdminTaskRow[]>([])
    const confirmRequests: AdminConfirmRequest[] = []
    const feedback = createFeedback()
    const commands = useAdminTaskSelectionDelete({
      selectedTasks,
      confirm: { request: (request) => confirmRequests.push(request) },
      feedback,
    })

    commands.requestDeleteSelectedTasks()

    expect(confirmRequests).toHaveLength(0)
    expect(feedback.noTasksSelected).toHaveBeenCalledTimes(1)
  })
})

function createFeedback(): AdminTaskDeletionFeedback {
  return {
    noTasksSelected: vi.fn(),
    tasksDeleted: vi.fn(),
    deleteTasksFailed: vi.fn(),
  }
}

function makeTask(overrides: Partial<AdminTaskRow> = {}): AdminTaskRow {
  return {
    id: 1,
    task_id: 'task-1',
    correlation_id: 'task-1',
    user_id: 1,
    prompt: 'prompt',
    status: 'completed',
    created_at: '2026-07-09T00:00:00.000Z',
    updated_at: '2026-07-09T00:00:00.000Z',
    ...overrides,
  }
}
