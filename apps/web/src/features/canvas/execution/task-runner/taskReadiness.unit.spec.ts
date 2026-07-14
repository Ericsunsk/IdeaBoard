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
        feature: 'pattern-gen',
        client_action_id: 'canvas:action-1',
        billing_quantity: 4,
      }),
    ).rejects.toThrow('Idea不足')

    expect(listener).toHaveBeenCalledWith(readiness)
    unsubscribe()
  })

  it('keeps an explicit free quote ready without requiring wallet coverage', async () => {
    const readiness = {
      ready: true,
      feature: 'pattern-gen',
      billing: {
        sku: 'ai-image:pattern-gen.test',
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
        feature: 'pattern-gen',
        client_action_id: 'canvas:free-action',
        billing_quantity: 8,
      }),
    ).resolves.toEqual(readiness)
  })

  it('classifies confirmed browser connection failures separately from server quote failures', async () => {
    mocks.checkSubmitReadiness.mockRejectedValueOnce(new Error('fetch failed'))

    await expect(
      assertTaskSubmitReady({
        prompt: 'network prompt',
        feature: 'pattern-gen',
        billing_quantity: 1,
      }),
    ).rejects.toMatchObject({ kind: 'network', message: '网络错误，请重试' })

    mocks.checkSubmitReadiness.mockRejectedValueOnce(
      Object.assign(new Error('暂时无法获取报价，请重试'), {
        status: 503,
        code: 'pricing_unavailable',
      }),
    )

    await expect(
      assertTaskSubmitReady({
        prompt: 'quote prompt',
        feature: 'pattern-gen',
        billing_quantity: 1,
      }),
    ).rejects.toMatchObject({
      kind: 'pricing-unavailable',
      message: '暂时无法获取报价，请重试',
    })
  })
})
