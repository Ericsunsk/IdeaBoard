import { describe, expect, it, vi } from 'vitest'
import { WORKFLOW_FEATURE, type WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import type { SubmitTaskReadinessData } from '../../../../shared/api/task'
import { createCanvasBillingReadinessStore } from './useCanvasBillingReadiness'

function makeReadiness(
  feature: WorkflowFeature,
  overrides: Partial<SubmitTaskReadinessData> = {},
): SubmitTaskReadinessData {
  return {
    ready: true,
    feature,
    billing: {
      sku: 'ai-image:prompt-generation',
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
  it('exposes the server quote and refreshes when the quote changes', async () => {
    const fetchReadiness = vi.fn(async (feature: WorkflowFeature) => makeReadiness(feature))
    const store = createCanvasBillingReadinessStore({ fetchReadiness })

    await store.ensureFeature(WORKFLOW_FEATURE.promptGeneration)
    expect(store.getFeatureState(WORKFLOW_FEATURE.promptGeneration)).toMatchObject({
      status: 'ready',
      unitCredits: 4,
      totalCredits: 4,
      availableCredits: 20,
      priceVersionId: 1,
    })

    store.applyReadiness(
      makeReadiness(WORKFLOW_FEATURE.promptGeneration, {
        billing: {
          ...makeReadiness(WORKFLOW_FEATURE.promptGeneration).billing!,
          price_version_id: 2,
          price_version: 2,
          unit_credits: 6,
          quantity: 4,
          total_credits: 24,
        },
      }),
    )

    expect(store.getFeatureState(WORKFLOW_FEATURE.promptGeneration)).toMatchObject({
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

    await store.ensureFeature(WORKFLOW_FEATURE.localInpaint)

    expect(store.getFeatureState(WORKFLOW_FEATURE.localInpaint)).toMatchObject({
      status: 'ready',
      unitCredits: 4,
      availableCredits: 3,
      sufficient: false,
    })
  })

  it.each([
    ['SKU 价格未配置', '价格未配置'],
    ['当前 SKU 价格已停用', '价格已停用'],
  ])('maps server billing errors to compact Chinese reasons', async (message, reason) => {
    const store = createCanvasBillingReadinessStore({
      fetchReadiness: async () => {
        throw new Error(message)
      },
    })

    await store.ensureFeature(WORKFLOW_FEATURE.removeBg)

    expect(store.getFeatureState(WORKFLOW_FEATURE.removeBg)).toMatchObject({
      status: 'error',
      reason,
    })
  })
})
