import { beforeEach, describe, expect, it, vi } from 'vitest'
import { subscribeCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import { assertTaskSubmitReady } from './taskReadiness'

const mocks = vi.hoisted(() => ({
  checkSubmitReadiness: vi.fn(),
}))

vi.mock('../../../../shared/api/task', () => ({
  taskApi: {
    checkSubmitReadiness: mocks.checkSubmitReadiness,
  },
}))

describe('task readiness billing updates', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('publishes the fresh server quote before blocking an insufficient submission', async () => {
    const readiness = {
      ready: false,
      message: 'Idea不足',
      feature: 'pattern-gen',
      billing: {
        sku: 'ai.image.pattern-gen',
        price_version_id: 2,
        price_version: 2,
        unit_credits: 6,
        quantity: 1,
        total_credits: 6,
        quoted_at: '2026-07-12T00:00:00.000Z',
        available_credits: 3,
        reserved_credits: 0,
        sufficient: false,
      },
    } as const
    mocks.checkSubmitReadiness.mockResolvedValue({ code: 0, data: readiness })
    const listener = vi.fn()
    const unsubscribe = subscribeCanvasTaskReadiness(listener)

    await expect(
      assertTaskSubmitReady({
        prompt: 'final prompt',
        feature: 'pattern-gen',
        client_action_id: 'canvas:action-1',
      }),
    ).rejects.toThrow('Idea不足')

    expect(listener).toHaveBeenCalledWith(readiness)
    unsubscribe()
  })
})
