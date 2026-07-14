import { beforeEach, describe, expect, it, vi } from 'vitest'
import { taskApi } from './task'

const mocks = vi.hoisted(() => ({
  cancelUserTask: vi.fn(),
  checkUserTaskSubmitReadiness: vi.fn(),
  getUserTask: vi.fn(),
  submitUserTask: vi.fn(),
}))

vi.mock('./generated-query/tasks/tasks', () => mocks)

describe('task API boundary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns domain data for task operations', async () => {
    const task = { task_id: 'task-1', status: 'running' }
    const readiness = { ready: true }
    mocks.submitUserTask.mockResolvedValue({ code: 0, msg: 'ok', data: { task_id: ' task-1 ' } })
    mocks.checkUserTaskSubmitReadiness.mockResolvedValue({ code: 0, msg: 'ok', data: readiness })
    mocks.getUserTask.mockResolvedValue({ code: 0, msg: 'ok', data: { task } })
    mocks.cancelUserTask.mockResolvedValue({ code: 0, msg: 'ok' })

    await expect(taskApi.submit({ prompt: 'test' })).resolves.toEqual({ task_id: 'task-1' })
    await expect(
      taskApi.checkSubmitReadiness({ prompt: 'test', billing_quantity: 1 }),
    ).resolves.toBe(readiness)
    await expect(taskApi.get('task-1')).resolves.toEqual({ task })
    await expect(taskApi.cancel('task-1')).resolves.toBeUndefined()
  })

  it('rejects business failures and malformed successful responses', async () => {
    mocks.submitUserTask.mockResolvedValue({ code: 503, msg: 'api key leaked' })
    mocks.getUserTask.mockResolvedValue({ code: 0, msg: 'ok', data: {} })

    await expect(taskApi.submit({ prompt: 'test' })).rejects.toThrow('提交任务失败')
    await expect(taskApi.get('task-1')).rejects.toThrow('加载任务失败')
  })
})
