import { describe, expect, it } from 'vitest'
import { buildFusionBillingReadinessRequest } from './canvasBillingReadinessResource'

describe('canvas billing readiness request contracts', () => {
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

  it('does not issue a Fusion readiness request before two images are available', () => {
    expect(
      buildFusionBillingReadinessRequest({ imageUrls: ['/uploads/a.png'], aspectRatio: 'auto' }),
    ).toBeNull()
  })
})
