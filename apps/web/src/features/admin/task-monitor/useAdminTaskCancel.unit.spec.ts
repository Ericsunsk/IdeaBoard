import { describe, expect, it, vi } from 'vitest'
import type { AdminConfirmRequest } from '../shared/composables/useAdminConfirm'
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

vi.mock('./taskMonitorResource', () => ({
  cancelAdminTaskByID: mocks.cancelAdminTaskByID,
  getAdminMonitorQueryKey: () => ['admin', 'monitor'],
  getAdminTaskDetailQueryKey: (taskID: string) => ['admin', 'tasks', taskID],
  getAdminTaskListQueryKey: () => ['admin', 'tasks'],
}))

describe('useAdminTaskCancel', () => {
  it('confirms one active task, invalidates task monitor data, and keeps terminal rows non-actionable', async () => {
    const requests: AdminConfirmRequest[] = []
    const feedback = { taskCancelled: vi.fn(), cancelTaskFailed: vi.fn() }
    const commands = useAdminTaskCancel({
      confirm: { request: (request) => requests.push(request) },
      feedback,
    })
    const runningTask = makeTask('running')

    expect(commands.isCancellableTask(runningTask)).toBe(true)
    expect(commands.isCancellableTask(makeTask('completed'))).toBe(false)

    commands.requestCancelTask(runningTask)
    expect(requests).toHaveLength(1)
    expect(requests[0]).toMatchObject({
      title: '确认取消任务',
      confirmLabel: '取消任务',
      intent: 'warn',
    })

    const confirmRequest = requests[0]
    if (!confirmRequest) throw new Error('expected cancel confirm request')

    await confirmRequest.onConfirm()

    expect(mocks.cancelAdminTaskByID).toHaveBeenCalledWith(runningTask.task_id)
    expect(feedback.taskCancelled).toHaveBeenCalledOnce()
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['admin', 'tasks'] })
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['admin', 'monitor'] })
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['admin', 'tasks', runningTask.task_id],
    })
  })
})

function makeTask(status: AdminTaskRow['status']): AdminTaskRow {
  return {
    task_id: 'task-a',
    user_public_id: '00000000-0000-0000-0000-000000000042',
    user_display_name: 'User 42',
    prompt: 'prompt',
    priority: 2,
    status,
    error_msg: '',
    result: '',
    created_at: '2026-07-29T00:00:00Z',
    completed_at: null,
  }
}
