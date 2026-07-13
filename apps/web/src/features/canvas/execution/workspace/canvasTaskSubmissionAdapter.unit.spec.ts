import { describe, expect, it, vi } from 'vitest'
import { createCanvasTaskSubmissionCallbacks } from './canvasTaskSubmissionAdapter'

describe('canvas task submission billing contract', () => {
  it('runs one typed readiness preflight and submits every output without redundant readiness', async () => {
    const taskAdapter = {
      assertTaskRequestReady: vi.fn(async () => undefined),
      submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
    }
    const callbacks = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => ({ prompt: 'submit readiness', feature: 'pattern-gen' }),
      buildRequest: () => ({ prompt: 'final prompt', feature: 'pattern-gen' }),
      outputCount: () => 2,
      taskAdapter,
    })

    await callbacks.assertReady?.()
    await expect(callbacks.submitTask('canvas:action-1')).resolves.toEqual({ taskId: 'task-1' })
    await expect(callbacks.submitTask('canvas:action-2')).resolves.toEqual({ taskId: 'task-1' })

    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledOnce()
    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledWith(
      expect.objectContaining({
        prompt: 'submit readiness',
        feature: 'pattern-gen',
        billing_quantity: 2,
        client_action_id: expect.stringMatching(/^canvas:/),
      }),
      undefined,
    )
    expect(taskAdapter.submitTaskRequest).toHaveBeenNthCalledWith(
      1,
      {
        prompt: 'final prompt',
        feature: 'pattern-gen',
        client_action_id: 'canvas:action-1',
      },
      undefined,
    )
    expect(taskAdapter.submitTaskRequest).toHaveBeenNthCalledWith(
      2,
      {
        prompt: 'final prompt',
        feature: 'pattern-gen',
        client_action_id: 'canvas:action-2',
      },
      undefined,
    )
  })

  it('stops before submit when the authoritative batch preflight reports insufficient credits', async () => {
    const taskAdapter = {
      assertTaskRequestReady: vi.fn(async () => {
        throw Object.assign(new Error('Idea不足'), { kind: 'insufficient-credits' })
      }),
      submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
    }
    const callbacks = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => ({ prompt: 'submit readiness', feature: 'pattern-gen' }),
      buildRequest: () => ({ prompt: 'final prompt', feature: 'pattern-gen' }),
      outputCount: () => 4,
      taskAdapter,
    })

    await expect(callbacks.assertReady?.()).rejects.toMatchObject({
      kind: 'insufficient-credits',
    })
    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledOnce()
    expect(taskAdapter.submitTaskRequest).not.toHaveBeenCalled()
  })

  it('allows an explicit free server quote without consulting cached balance', async () => {
    const taskAdapter = {
      assertTaskRequestReady: vi.fn(async () => undefined),
      submitTaskRequest: vi.fn(async () => ({ taskId: 'task-free' })),
    }
    const callbacks = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => ({ prompt: 'submit readiness', feature: 'pattern-gen' }),
      buildRequest: () => ({ prompt: 'free prompt', feature: 'pattern-gen' }),
      outputCount: () => 8,
      taskAdapter,
    })

    await expect(callbacks.assertReady?.()).resolves.toBeUndefined()
    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledOnce()
  })

  it('does not consult stale cached balance after a successful server preflight', async () => {
    const taskAdapter = {
      assertTaskRequestReady: vi.fn(async () => undefined),
      submitTaskRequest: vi.fn(async () => ({ taskId: 'task-1' })),
    }
    const callbacks = createCanvasTaskSubmissionCallbacks({
      readinessRequest: () => ({ prompt: 'submit readiness', feature: 'pattern-gen' }),
      buildRequest: () => ({ prompt: 'final prompt', feature: 'pattern-gen' }),
      outputCount: () => 4,
      taskAdapter,
    })

    await expect(callbacks.assertReady?.()).resolves.toBeUndefined()
    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledOnce()
    expect(taskAdapter.assertTaskRequestReady).toHaveBeenCalledWith(
      expect.objectContaining({ billing_quantity: 4 }),
      undefined,
    )
  })
})
