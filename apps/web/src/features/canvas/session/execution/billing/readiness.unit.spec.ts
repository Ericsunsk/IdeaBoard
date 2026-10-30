import { describe, expect, it, vi } from 'vitest'
import { SERVICE, type Service } from '../../../../../core/contracts/imageServices'
import type {
  SubmitTaskReadinessData,
  CanvasTaskRequestData,
} from '../../../core/execution/canvasTaskRequest'
import { createCanvasBillingReadinessStore } from './readiness.ts'

function makeReadiness(
  service: Service,
  overrides: Partial<SubmitTaskReadinessData> = {},
): SubmitTaskReadinessData {
  return {
    ready: true,
    service,
    billing: {
      sku: 'ai-image:generation',
      output_resolution: '1k',
      display_name: 'Pattern generation',
      price_version_id: 1,
      price_version: 1,
      unit_credits: 4,
      quantity: 1,
      total_credits: 4,
      quoted_at: '2026-07-12T00:00:00.000Z',
      available_credits: 20,
      reserved_credits: 0,
      sufficient: true,
    },
    ...overrides,
  }
}

describe('canvas billing readiness store', () => {
  it('loads multiple action quotes through one readiness matrix request', async () => {
    const fetchReadinessMatrix = vi.fn(async (requests: CanvasTaskRequestData[]) =>
      requests.map((request) => makeReadiness(request.service as Service)),
    )
    const store = createCanvasBillingReadinessStore({ fetchReadinessMatrix })
    const entries = [
      {
        service: SERVICE.variations,
        request: { prompt: '', service: SERVICE.variations },
      },
      {
        service: SERVICE.cutout,
        request: { prompt: '', service: SERVICE.cutout },
      },
    ]

    await store.ensureMatrix(entries)
    await store.ensureMatrix(entries)

    const firstEntry = entries[0]
    const secondEntry = entries[1]
    if (!firstEntry || !secondEntry) throw new Error('expected readiness entries')

    expect(fetchReadinessMatrix).toHaveBeenCalledOnce()
    expect(fetchReadinessMatrix).toHaveBeenCalledWith(entries.map((entry) => entry.request))
    expect(store.getServiceState(SERVICE.variations, firstEntry.request).status).toBe('ready')
    expect(store.getServiceState(SERVICE.cutout, secondEntry.request).status).toBe('ready')
  })

  it('exposes the server quote and refreshes when the quote changes', async () => {
    const fetchReadiness = vi.fn(async (service: Service) => makeReadiness(service))
    const store = createCanvasBillingReadinessStore({ fetchReadiness })

    await store.ensureService(SERVICE.generation)
    expect(store.getServiceState(SERVICE.generation)).toMatchObject({
      status: 'ready',
      unitCredits: 4,
      totalCredits: 4,
      availableCredits: 20,
      priceVersionId: 1,
    })

    fetchReadiness.mockResolvedValueOnce(
      makeReadiness(SERVICE.generation, {
        billing: {
          ...makeReadiness(SERVICE.generation).billing!,
          price_version_id: 2,
          price_version: 2,
          unit_credits: 6,
          quantity: 4,
          total_credits: 24,
        },
      }),
    )

    await store.ensureService(SERVICE.generation, { shouldForce: true })
    expect(store.getServiceState(SERVICE.generation)).toMatchObject({
      status: 'ready',
      unitCredits: 6,
      totalCredits: 24,
      priceVersionId: 2,
    })
  })

  it('retains an insufficient quote for actionable preflight instead of disabling the control', async () => {
    const store = createCanvasBillingReadinessStore({
      fetchReadiness: async (service) =>
        makeReadiness(service, {
          ready: false,
          message: 'Idea不足',
          billing: {
            ...makeReadiness(service).billing!,
            total_credits: 8,
            available_credits: 3,
            sufficient: false,
          },
        }),
    })

    await store.ensureService(SERVICE.inpaint)

    expect(store.getServiceState(SERVICE.inpaint)).toMatchObject({
      status: 'ready',
      unitCredits: 4,
      availableCredits: 3,
      isSufficient: false,
    })
  })

  it('classifies a blocked readiness through the shared error catalog', async () => {
    const store = createCanvasBillingReadinessStore({
      fetchReadiness: async () =>
        makeReadiness(SERVICE.cutout, {
          ready: false,
          error_code: 'pricing_unavailable',
        }),
    })

    await store.ensureService(SERVICE.cutout)

    expect(store.getServiceState(SERVICE.cutout)).toMatchObject({
      status: 'blocked',
      reason: 'Pricing is temporarily unavailable. Please try again shortly.',
    })
  })

  it('falls back to billing-unavailable copy for transport failures', async () => {
    const store = createCanvasBillingReadinessStore({
      fetchReadiness: async () => {
        throw new Error('dial tcp refused')
      },
    })

    await store.ensureService(SERVICE.cutout)

    expect(store.getServiceState(SERVICE.cutout)).toMatchObject({
      status: 'error',
      reason: 'Billing information is unavailable right now.',
    })
  })

  it('caches readiness failures to avoid repeated requests for the same dynamic quote', async () => {
    let currentTime = 1_000
    const fetchReadiness = vi.fn(async () => {
      throw new Error('pricing is not configured')
    })
    const store = createCanvasBillingReadinessStore({
      fetchReadiness,
      now: () => currentTime,
      cacheMs: 60_000,
    })
    const request: CanvasTaskRequestData = {
      prompt: 'quote',
      service: SERVICE.cutout,
    }

    await store.ensureService(SERVICE.cutout, { request, shouldForce: true })
    await store.ensureService(SERVICE.cutout, { request, shouldForce: true })
    expect(fetchReadiness).toHaveBeenCalledOnce()

    currentTime += 60_001
    await store.ensureService(SERVICE.cutout, { request, shouldForce: true })
    expect(fetchReadiness).toHaveBeenCalledTimes(2)
  })
})
