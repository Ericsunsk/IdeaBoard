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
      feature: 'generation',
      billing: {
        sku: 'ai-image:generation',
        price_version_id: 2,
        price_version: 2,
        unit_credits: 6,
        quantity: 4,
        total_credits: 24,
        quoted_at: '2026-07-12T00:00:00.000Z',
        available_credits: 3,
        reserved_credits: 0,
        sufficient: false,
      },
    } as const
    mocks.checkSubmitReadiness.mockResolvedValue(readiness)
    const listener = vi.fn()
    const unsubscribe = subscribeCanvasTaskReadiness(listener)

    await expect(
      assertTaskSubmitReady({
        prompt: 'final prompt',
        feature: 'generation',
        client_action_id: 'canvas:action-1',
        params: { output_count: 4 },
      }),
    ).rejects.toThrow('You need more Ideas to run this action.')

    expect(listener).toHaveBeenCalledWith(readiness)
    unsubscribe()
  })

  it('keeps an explicit free quote ready without requiring wallet coverage', async () => {
    const readiness = {
      ready: true,
      feature: 'generation',
      billing: {
        sku: 'ai-image:generation.test',
        price_version_id: 2,
        price_version: 2,
        unit_credits: 0,
        quantity: 1,
        total_credits: 0,
        quoted_at: '2026-07-12T00:00:00.000Z',
        available_credits: 0,
        reserved_credits: 0,
        sufficient: true,
      },
    } as const
    mocks.checkSubmitReadiness.mockResolvedValue(readiness)

    await expect(
      assertTaskSubmitReady({
        prompt: 'free prompt',
        feature: 'generation',
        client_action_id: 'canvas:free-action',
        params: { output_count: 4 },
      }),
    ).resolves.toEqual(readiness)
  })

  it('classifies confirmed browser connection failures separately from server quote failures', async () => {
    mocks.checkSubmitReadiness.mockRejectedValueOnce(new Error('fetch failed'))

    await expect(
      assertTaskSubmitReady({
        prompt: 'network prompt',
        feature: 'generation',
        params: { output_count: 1 },
      }),
    ).rejects.toMatchObject({ kind: 'network', message: 'The request could not be completed.' })

    mocks.checkSubmitReadiness.mockRejectedValueOnce(
      Object.assign(new Error('pricing unavailable'), {
        status: 503,
        code: 'pricing_unavailable',
      }),
    )

    await expect(
      assertTaskSubmitReady({
        prompt: 'quote prompt',
        feature: 'generation',
        params: { output_count: 1 },
      }),
    ).rejects.toMatchObject({
      kind: 'pricing-unavailable',
      message: 'Pricing is temporarily unavailable. Please try again shortly.',
    })
  })
})
