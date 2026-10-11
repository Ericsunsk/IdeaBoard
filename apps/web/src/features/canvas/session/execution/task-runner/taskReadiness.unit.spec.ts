import { beforeEach, describe, expect, it, vi } from 'vitest'
import { subscribeCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import { assertTaskSubmitReady } from './taskReadiness'

const mocks = vi.hoisted(() => ({
  fetchReadiness: vi.fn(),
}))

vi.mock('../workspace/canvasWorkspaceTaskAdapter', () => ({
  fetchCanvasWorkspaceTaskReadiness: mocks.fetchReadiness,
}))

describe('task readiness billing updates', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('publishes the fresh server quote before blocking an insufficient submission', async () => {
    const readiness = {
      ready: false,
      message: 'Idea不足',
      service: 'generation',
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
    mocks.fetchReadiness.mockResolvedValue(readiness)
    const listener = vi.fn()
    const unsubscribe = subscribeCanvasTaskReadiness(listener)

    await expect(
      assertTaskSubmitReady({
        prompt: 'final prompt',
        service: 'generation',
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
      service: 'generation',
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
    mocks.fetchReadiness.mockResolvedValue(readiness)

    await expect(
      assertTaskSubmitReady({
        prompt: 'free prompt',
        service: 'generation',
        client_action_id: 'canvas:free-action',
        params: { output_count: 4 },
      }),
    ).resolves.toEqual(readiness)
  })

  it('classifies confirmed browser connection failures separately from server quote failures', async () => {
    mocks.fetchReadiness.mockRejectedValueOnce(new Error('fetch failed'))

    await expect(
      assertTaskSubmitReady({
        prompt: 'network prompt',
        service: 'generation',
        params: { output_count: 1 },
      }),
    ).rejects.toMatchObject({ kind: 'network', message: 'The request could not be completed.' })

    mocks.fetchReadiness.mockRejectedValueOnce(
      Object.assign(new Error('pricing unavailable'), {
        status: 503,
        code: 'billing_quote_unavailable',
      }),
    )

    await expect(
      assertTaskSubmitReady({
        prompt: 'quote prompt',
        service: 'generation',
        params: { output_count: 1 },
      }),
    ).rejects.toMatchObject({
      kind: 'pricing-unavailable',
      message: 'Pricing is temporarily unavailable. Please try again shortly.',
    })
  })

  it('blocks an unavailable resolution with the clear pool copy', async () => {
    mocks.fetchReadiness.mockResolvedValue({
      ready: false,
      service: 'generation',
      error_code: 'service_resolution_unavailable',
      message: 'This resolution is unavailable right now.',
    })

    await expect(
      assertTaskSubmitReady({
        prompt: 'a cat',
        service: 'generation',
        params: { output_count: 1, output_resolution: '2k' },
      }),
    ).rejects.toThrow('This resolution is unavailable right now.')
  })
})
