import { describe, expect, it, vi } from 'vitest'
import { createCanvasTaskSubmissionCallbacks } from './canvasTaskSubmissionAdapter'

describe('canvas task submission billing contract', () => {
  it('checks a lightweight readiness request and rechecks the complete request before submit', async () => {
    const taskAdapter = {
      assertTaskRequestReady: vi.fn(async () => undefined),
      submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
    }
    const callbacks = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => ({ prompt: 'submit readiness', feature: 'pattern-gen' }),
      buildRequest: () => ({ prompt: 'final prompt', feature: 'pattern-gen' }),
      taskAdapter,
    })

    await callbacks.assertReady?.()
    await expect(callbacks.submitTask('canvas:action-1')).resolves.toEqual({ taskId: 'task-1' })

    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledTimes(2)
    expect(taskAdapter.assertTaskRequestReady).toHaveBeenNthCalledWith(
      2,
      {
        prompt: 'final prompt',
        feature: 'pattern-gen',
        client_action_id: 'canvas:action-1',
      },
      undefined,
    )
    expect(taskAdapter.submitTaskRequest).toHaveBeenCalledWith(
      {
        prompt: 'final prompt',
        feature: 'pattern-gen',
        client_action_id: 'canvas:action-1',
      },
      undefined,
    )
  })
})
