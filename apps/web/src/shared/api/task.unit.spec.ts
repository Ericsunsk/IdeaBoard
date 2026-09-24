import { beforeEach, describe, expect, it, vi } from 'vitest'
import { taskApi } from './task'

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('./client', () => ({
  api: { request: mocks.request },
}))

describe('task API boundary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns domain data for task operations', async () => {
    const task = { task_id: 'task-1', status: 'running' }
    const readiness = { ready: true }
    mocks.request
      .mockResolvedValueOnce({ code: 0, msg: 'ok', data: { task_id: ' task-1 ' } })
      .mockResolvedValueOnce({ code: 0, msg: 'ok', data: readiness })
      .mockResolvedValueOnce({ code: 0, msg: 'ok', data: { task } })
      .mockResolvedValueOnce({ code: 0, msg: 'ok' })

    await expect(taskApi.submit({ prompt: 'test' })).resolves.toEqual({ task_id: 'task-1' })
    await expect(taskApi.checkSubmitReadiness({ prompt: 'test' })).resolves.toBe(readiness)
    await expect(taskApi.get('task-1')).resolves.toEqual({ task })
    await expect(taskApi.cancel('task-1')).resolves.toBeUndefined()
    expect(mocks.request).toHaveBeenLastCalledWith(expect.stringContaining('/user/tasks/task-1/cancel'), {
      method: 'POST',
      headers: { 'Idempotency-Key': 'task-cancel:task-1' },
    })
  })

  it('rejects business failures and malformed successful responses', async () => {
    mocks.request
      .mockResolvedValueOnce({ code: 503, msg: 'api_key=exposed' })
      .mockResolvedValueOnce({ code: 0, msg: 'ok', data: {} })

    await expect(taskApi.submit({ prompt: 'test' })).rejects.toThrow(
      'Could not submit the task. Please try again.',
    )
    await expect(taskApi.get('task-1')).rejects.toThrow('Could not load the task.')
  })
})
