import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildFusionBillingReadinessRequest,
  fetchCanvasBillingReadiness,
  fetchCanvasBillingReadinessMatrix,
} from './canvasBillingReadinessResource'

const mocks = vi.hoisted(() => ({
  fetchReadiness: vi.fn(),
  fetchMatrix: vi.fn(),
}))

vi.mock('../workspace/canvasWorkspaceTaskAdapter', () => ({
  fetchCanvasWorkspaceTaskReadiness: mocks.fetchReadiness,
  fetchCanvasWorkspaceTaskReadinessMatrix: mocks.fetchMatrix,
}))

describe('canvas billing readiness request contracts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('uses the request output count for presentation quote refreshes', async () => {
    const readiness = { ready: true, service: 'generation' } as const satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >
    mocks.fetchReadiness.mockResolvedValue(readiness)

    await expect(
      fetchCanvasBillingReadiness('generation', {
        prompt: 'presentation quote',
        service: 'generation',
        params: { output_count: 1 },
      }),
    ).resolves.toEqual(readiness)

    expect(mocks.fetchReadiness).toHaveBeenCalledWith(
      expect.objectContaining({ params: { output_count: 1 } }),
    )
  })

  it('uses current Fusion image count for quote weights without carrying media inputs', () => {
    expect(
      buildFusionBillingReadinessRequest({
        imageUrls: [' /uploads/a.png ', '/uploads/b.png'],
        aspectRatio: '1:1',
      }),
    ).toMatchObject({
      prompt: '[fusion] submit readiness',
      service: 'fusion',
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
    const readiness = { ready: true, service: 'fusion' } as const satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >
    mocks.fetchReadiness.mockResolvedValue(readiness)
    const request = buildFusionBillingReadinessRequest({
      imageUrls: ['/uploads/a.png', '/uploads/b.png'],
      aspectRatio: '1:1',
      outputCount: 3,
    })

    await fetchCanvasBillingReadiness('fusion', request ?? undefined)

    expect(mocks.fetchReadiness).toHaveBeenCalledWith(
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

  it('short-circuits an empty matrix without transport calls', async () => {
    await expect(fetchCanvasBillingReadinessMatrix([])).resolves.toEqual([])
    expect(mocks.fetchMatrix).not.toHaveBeenCalled()
  })

  it('delegates matrix readiness to one quote per request', async () => {
    const items = [
      { ready: true, service: 'generation' } satisfies Partial<
        import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
      >,
      { ready: true, service: 'cutout' } satisfies Partial<
        import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
      >,
    ] as const
    mocks.fetchMatrix.mockResolvedValue(items)
    const requests = [
      { prompt: '', service: 'generation', params: { output_count: 1 } },
      { prompt: '', service: 'cutout', params: { output_count: 1 } },
    ] as const

    await expect(fetchCanvasBillingReadinessMatrix([...requests])).resolves.toEqual(items)
    expect(mocks.fetchMatrix).toHaveBeenCalledWith([...requests])
  })
})
