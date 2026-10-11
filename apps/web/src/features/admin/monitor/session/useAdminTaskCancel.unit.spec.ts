import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminTaskRow } from './useAdminTaskList'
import { useAdminTaskCancel } from './useAdminTaskCancel'

const mocks = vi.hoisted(() => ({
  cancelAdminTaskByID: vi.fn().mockResolvedValue(undefined),
  invalidateQueries: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@tanstack/vue-query', () => ({
  useQueryClient: () => ({ invalidateQueries: mocks.invalidateQueries }),
  useMutation: (options: {
    mutationFn: (taskID: string) => Promise<void>
    onSuccess: (result: void, taskID: string) => Promise<void>
  }) => ({
    mutateAsync: async (taskID: string) => {
      const result = await options.mutationFn(taskID)
      await options.onSuccess(result, taskID)
      return result
    },
  }),
}))

vi.mock('../resource', () => ({
  cancelAdminTaskByID: mocks.cancelAdminTaskByID,
  getAdminMonitorQueryKey: () => ['admin', 'monitor'],
  getAdminTaskDetailQueryKey: (taskID: string) => ['admin', 'monitor', 'detail', taskID],
  getAdminTaskListQueryKey: () => ['admin', 'tasks', 'list'],
}))

describe('useAdminTaskCancel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('confirms one active task, invalidates task monitor data, and keeps terminal rows non-actionable', async () => {
    const confirm = { confirm: vi.fn().mockResolvedValue(true) }
    const feedback = { taskCancelled: vi.fn(), cancelTaskFailed: vi.fn() }
    const commands = useAdminTaskCancel({
      confirm,
      feedback,
    })
    const runningTask = makeTask('running')

    expect(commands.isCancellableTask(runningTask)).toBe(true)
    expect(commands.isCancellableTask(makeTask('completed'))).toBe(false)

    await commands.requestCancelTask(runningTask)
    expect(confirm.confirm).toHaveBeenCalledTimes(1)
    expect(confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认取消任务',
        confirmLabel: '取消任务',
        intent: 'warn',
      }),
    )

    expect(mocks.cancelAdminTaskByID).toHaveBeenCalledWith(runningTask.task_id)
    expect(feedback.taskCancelled).toHaveBeenCalledOnce()
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['admin', 'tasks', 'list'] })
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['admin', 'monitor'] })
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['admin', 'monitor', 'detail', runningTask.task_id],
    })
  })

  it('skips cancellation when the operator declines confirmation', async () => {
    const confirm = { confirm: vi.fn().mockResolvedValue(false) }
    const feedback = { taskCancelled: vi.fn(), cancelTaskFailed: vi.fn() }
    const commands = useAdminTaskCancel({
      confirm,
      feedback,
    })

    await commands.requestCancelTask(makeTask('running'))

    expect(confirm.confirm).toHaveBeenCalledTimes(1)
    expect(mocks.cancelAdminTaskByID).not.toHaveBeenCalled()
    expect(feedback.taskCancelled).not.toHaveBeenCalled()
  })
})

function makeTask(status: AdminTaskRow['status']): AdminTaskRow {
  return {
    task_id: 'task-a',
    user_public_id: '00000000-0000-0000-0000-000000000042',
    user_display_name: 'User 42',
    prompt: 'prompt',
    service: 'render',
    credits: 0,
    priority: 2,
    status,
    error_msg: '',
    result: '',
    created_at: '2026-07-29T00:00:00Z',
    completed_at: null,
  }
}
