import { describe, expect, it, vi } from 'vitest'
import {
  WORKFLOW_FEATURE,
  type WorkflowFeature,
} from '../../../../../shared/domain/workflowFeatures'
import type {
  SubmitTaskReadinessData,
  SubmitTaskReadinessRequestData,
} from '../../../../../shared/api/task'
import { createCanvasBillingReadinessStore } from './readiness.ts'

function makeReadiness(
  feature: WorkflowFeature,
  overrides: Partial<SubmitTaskReadinessData> = {},
): SubmitTaskReadinessData {
  return {
    ready: true,
    feature,
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
    const fetchReadinessMatrix = vi.fn(async (requests: SubmitTaskReadinessRequestData[]) =>
      requests.map((request) => makeReadiness(request.feature as WorkflowFeature)),
    )
    const store = createCanvasBillingReadinessStore({ fetchReadinessMatrix })
    const entries = [
      {
        feature: WORKFLOW_FEATURE.variations,
        request: { prompt: '', feature: WORKFLOW_FEATURE.variations },
      },
      {
        feature: WORKFLOW_FEATURE.cutout,
        request: { prompt: '', feature: WORKFLOW_FEATURE.cutout },
      },
    ]

    await store.ensureMatrix(entries)
    await store.ensureMatrix(entries)

    const firstEntry = entries[0]
    const secondEntry = entries[1]
    if (!firstEntry || !secondEntry) throw new Error('expected readiness entries')

    expect(fetchReadinessMatrix).toHaveBeenCalledOnce()
    expect(fetchReadinessMatrix).toHaveBeenCalledWith(entries.map((entry) => entry.request))
    expect(store.getFeatureState(WORKFLOW_FEATURE.variations, firstEntry.request).status).toBe(
      'ready',
    )
    expect(store.getFeatureState(WORKFLOW_FEATURE.cutout, secondEntry.request).status).toBe('ready')
  })

  it('exposes the server quote and refreshes when the quote changes', async () => {
    const fetchReadiness = vi.fn(async (feature: WorkflowFeature) => makeReadiness(feature))
    const store = createCanvasBillingReadinessStore({ fetchReadiness })

    await store.ensureFeature(WORKFLOW_FEATURE.generation)
    expect(store.getFeatureState(WORKFLOW_FEATURE.generation)).toMatchObject({
      status: 'ready',
      unitCredits: 4,
      totalCredits: 4,
      availableCredits: 20,
      priceVersionId: 1,
    })

    store.applyReadiness(
      makeReadiness(WORKFLOW_FEATURE.generation, {
        billing: {
          ...makeReadiness(WORKFLOW_FEATURE.generation).billing!,
          price_version_id: 2,
          price_version: 2,
          unit_credits: 6,
          quantity: 4,
          total_credits: 24,
        },
      }),
    )

    expect(store.getFeatureState(WORKFLOW_FEATURE.generation)).toMatchObject({
      status: 'ready',
      unitCredits: 6,
      totalCredits: 24,
      priceVersionId: 2,
    })
  })

  it('retains an insufficient quote for actionable preflight instead of disabling the control', async () => {
    const store = createCanvasBillingReadinessStore({
      fetchReadiness: async (feature) =>
        makeReadiness(feature, {
          ready: false,
          message: 'Idea不足',
          billing: {
            ...makeReadiness(feature).billing!,
            total_credits: 8,
            available_credits: 3,
            sufficient: false,
          },
        }),
    })

    await store.ensureFeature(WORKFLOW_FEATURE.inpaint)

    expect(store.getFeatureState(WORKFLOW_FEATURE.inpaint)).toMatchObject({
      status: 'ready',
      unitCredits: 4,
      availableCredits: 3,
      sufficient: false,
    })
  })

  it('classifies a blocked readiness through the shared error catalog', async () => {
    const store = createCanvasBillingReadinessStore({
      fetchReadiness: async () =>
        makeReadiness(WORKFLOW_FEATURE.cutout, {
          ready: false,
          error_code: 'pricing_unavailable',
        }),
    })

    await store.ensureFeature(WORKFLOW_FEATURE.cutout)

    expect(store.getFeatureState(WORKFLOW_FEATURE.cutout)).toMatchObject({
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

    await store.ensureFeature(WORKFLOW_FEATURE.cutout)

    expect(store.getFeatureState(WORKFLOW_FEATURE.cutout)).toMatchObject({
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
    const request: SubmitTaskReadinessRequestData = {
      prompt: 'quote',
      feature: WORKFLOW_FEATURE.cutout,
    }

    await store.ensureFeature(WORKFLOW_FEATURE.cutout, { request, force: true })
    await store.ensureFeature(WORKFLOW_FEATURE.cutout, { request, force: true })
    expect(fetchReadiness).toHaveBeenCalledOnce()

    currentTime += 60_001
    await store.ensureFeature(WORKFLOW_FEATURE.cutout, { request, force: true })
    expect(fetchReadiness).toHaveBeenCalledTimes(2)
  })
})
