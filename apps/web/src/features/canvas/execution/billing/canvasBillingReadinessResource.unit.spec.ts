import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildFusionBillingReadinessRequest,
  fetchCanvasBillingReadiness,
} from './canvasBillingReadinessResource'

const mocks = vi.hoisted(() => ({
  checkSubmitReadiness: vi.fn(),
}))

vi.mock('../../../../shared/api/task', () => ({
  taskApi: { checkSubmitReadiness: mocks.checkSubmitReadiness },
}))

describe('canvas billing readiness request contracts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses the request output count for presentation quote refreshes', async () => {
    const readiness = { ready: true, feature: 'prompt-generation' } as const
    mocks.checkSubmitReadiness.mockResolvedValue(readiness)

    await expect(
      fetchCanvasBillingReadiness('prompt-generation', {
        prompt: 'presentation quote',
        feature: 'prompt-generation',
        params: { output_count: 1 },
      }),
    ).resolves.toEqual(readiness)

    expect(mocks.checkSubmitReadiness).toHaveBeenCalledWith(
      expect.objectContaining({ params: { output_count: 1 } }),
    )
  })

  it('uses current Fusion media with normalized weights for a valid readiness request', () => {
    expect(
      buildFusionBillingReadinessRequest({
        imageUrls: [' /uploads/a.png ', '/uploads/b.png'],
        aspectRatio: '1:1',
      }),
    ).toMatchObject({
      prompt: '[style-fusion] submit readiness',
      feature: 'style-fusion',
      image_urls: ['/uploads/a.png', '/uploads/b.png'],
      params: {
        ratio: '1:1',
        fusion: {
          mode: 'weighted_reference',
          input_weights: [0.5, 0.5],
        },
      },
    })
  })

  it('quotes Fusion at the selected output count', async () => {
    const readiness = { ready: true, feature: 'style-fusion' } as const
    mocks.checkSubmitReadiness.mockResolvedValue(readiness)
    const request = buildFusionBillingReadinessRequest({
      imageUrls: ['/uploads/a.png', '/uploads/b.png'],
      aspectRatio: '1:1',
      outputCount: 3,
    })

    await fetchCanvasBillingReadiness('style-fusion', request ?? undefined)

    expect(mocks.checkSubmitReadiness).toHaveBeenCalledWith(
      expect.objectContaining({
        params: expect.objectContaining({ output_count: 3 }),
      }),
    )
  })

  it('does not issue a Fusion readiness request before two images are available', () => {
    expect(
      buildFusionBillingReadinessRequest({ imageUrls: ['/uploads/a.png'], aspectRatio: 'auto' }),
    ).toBeNull()
  })
})
