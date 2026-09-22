import { describe, expect, it } from 'vitest'
import {
  formatObservedFailure,
  formatObservedMargin,
  formatObservedRate,
  formatReferenceContext,
  formatReferenceFreshness,
  formatReferenceModalities,
  formatReferencePricing,
  formatReferenceReleaseKnowledge,
  hasReferenceCapabilityData,
  isCloudflareHosted,
  isFalReferenceModel,
  matchesModelKeyword,
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_OPTIONS,
  MODEL_HOSTING_THIRD_PARTY,
  modelProviderBadgeClass,
  observedOnSaleMargins,
  referenceCapabilities,
  referenceStatusKind,
  type AdminModelItem,
  type ModelObservedSnapshot,
} from './catalogData'

describe('catalogData vocabulary', () => {
  it('centralizes hosting copy and inference', () => {
    expect([...MODEL_HOSTING_OPTIONS]).toEqual([
      MODEL_HOSTING_CLOUDFLARE,
      MODEL_HOSTING_THIRD_PARTY,
    ])
    expect(isCloudflareHosted('Cloudflare Workers AI')).toBe(true)
    expect(isCloudflareHosted('Fal AI')).toBe(false)
  })

  it('badges known providers distinctly', () => {
    expect(modelProviderBadgeClass('OpenAI')).not.toBe(modelProviderBadgeClass('Fal AI'))
    expect(modelProviderBadgeClass('unknown-vendor')).toBe(
      modelProviderBadgeClass('another-unknown'),
    )
  })
  it('matches keywords across name, id, provider, mode and description', () => {
    const model: AdminModelItem = {
      id: 'openai/gpt-image-1',
      name: 'GPT Image',
      provider: 'OpenAI',
      mode: 'image_generation',
      description: 'seed model',
      contextOrResolution: '4096×4096',
      capabilities: [],
      hosting: MODEL_HOSTING_THIRD_PARTY,
      termsUrl: '',
      docsUrl: '',
    }
    expect(matchesModelKeyword(model, '')).toBe(true)
    expect(matchesModelKeyword(model, 'gpt')).toBe(true)
    expect(matchesModelKeyword(model, 'image_generation')).toBe(true)
    expect(matchesModelKeyword(model, 'seed model')).toBe(true)
    expect(matchesModelKeyword(model, 'non-existent-xyz')).toBe(false)
  })

  it('formats models.dev reference pricing per 1M tokens and hides unknown', () => {
    expect(formatReferencePricing({})).toBe('')
    expect(formatReferencePricing({ inputUsdPerMtok: 0, outputUsdPerMtok: -1 })).toBe('')
    expect(formatReferencePricing({ inputUsdPerMtok: 2.5, outputUsdPerMtok: 10 })).toBe(
      '$2.5 in · $10 out /1M',
    )
    expect(formatReferencePricing({ inputUsdPerMtok: 0.075 })).toBe('$0.075 in /1M')
    expect(formatReferencePricing({ outputUsdPerMtok: 2 })).toBe('$2 out /1M')
  })

  it('formats reference context limits and hides unknown', () => {
    expect(formatReferenceContext(undefined)).toBe('')
    expect(formatReferenceContext(0)).toBe('')
    expect(formatReferenceContext(128000)).toBe('128K')
    expect(formatReferenceContext(200000)).toBe('200K')
    expect(formatReferenceContext(9500)).toBe('9500')
  })

  it('detects fal models for graceful reference fallback without false positives', () => {
    expect(isFalReferenceModel({ provider: 'Fal AI', modelId: 'fal_ai/flux-dev' })).toBe(true)
    expect(isFalReferenceModel({ provider: 'fal', modelId: '' })).toBe(true)
    expect(isFalReferenceModel({ provider: '', modelId: 'fal_ai/flux-dev' })).toBe(true)
    expect(isFalReferenceModel({ provider: '', modelId: 'fal/flux-dev' })).toBe(true)
    expect(isFalReferenceModel({ provider: 'OpenAI', modelId: 'openai/gpt-4o' })).toBe(false)
    expect(isFalReferenceModel({ provider: 'Falcon AI', modelId: 'falcon/7b' })).toBe(false)
    expect(isFalReferenceModel({})).toBe(false)
  })

  it('formats reference freshness relatively and hides unknown or future dates', () => {
    const now = Date.parse('2026-09-22T00:00:00Z')
    expect(formatReferenceFreshness(undefined, now)).toBe('')
    expect(formatReferenceFreshness('not-a-date', now)).toBe('')
    expect(formatReferenceFreshness('2026-09-23T00:00:00Z', now)).toBe('')
    expect(formatReferenceFreshness('2026-09-22T00:00:00Z', now)).toBe('刚刚')
    expect(formatReferenceFreshness('2026-09-21T23:20:00Z', now)).toBe('40 分钟前')
    expect(formatReferenceFreshness('2026-09-21T21:00:00Z', now)).toBe('3 小时前')
    expect(formatReferenceFreshness('2026-09-21T00:00:00Z', now)).toBe('1 天前')
    expect(formatReferenceFreshness('2026-07-24T00:00:00Z', now)).toBe('2 个月前')
    expect(formatReferenceFreshness('2025-09-22T00:00:00Z', now)).toBe('1 年前')
  })

  it('lists reference capabilities in fixed order and detects unknown-only snapshots', () => {
    const capabilities = referenceCapabilities({
      reasoning: true,
      toolCall: false,
      temperature: undefined,
      attachment: true,
      structuredOutput: false,
      openWeights: undefined,
    })
    expect(capabilities.map((capability) => capability.key)).toEqual([
      'reasoning',
      'toolCall',
      'temperature',
      'attachment',
      'structuredOutput',
      'openWeights',
    ])
    expect(capabilities[0]).toMatchObject({ label: '推理', supported: true })
    expect(capabilities[1]).toMatchObject({ label: '工具调用', supported: false })
    expect(capabilities[2]).toMatchObject({ label: '温度参数', supported: undefined })
    expect(hasReferenceCapabilityData({})).toBe(false)
    expect(hasReferenceCapabilityData({ temperature: false })).toBe(true)
  })

  it('formats reference modalities and hides unknown sides', () => {
    expect(formatReferenceModalities(undefined)).toBe('')
    expect(formatReferenceModalities({})).toBe('')
    expect(formatReferenceModalities({ input: ['text', 'image'], output: ['text'] })).toBe(
      'text / image → text',
    )
    expect(formatReferenceModalities({ output: ['text'] })).toBe('text')
    expect(formatReferenceModalities({ input: ['text'] })).toBe('text')
  })

  it('formats reference release date and knowledge cutoff and hides unknown', () => {
    expect(formatReferenceReleaseKnowledge(undefined, undefined)).toBe('')
    expect(formatReferenceReleaseKnowledge('2025-08-07', '2024-05-30')).toBe(
      '2025-08-07 / 截止 2024-05-30',
    )
    expect(formatReferenceReleaseKnowledge('2025-08-07', undefined)).toBe('2025-08-07')
    expect(formatReferenceReleaseKnowledge(undefined, '2024-05-30')).toBe('截止 2024-05-30')
  })

  it('classifies reference status without false deprecated warnings', () => {
    expect(referenceStatusKind(undefined)).toBe('unknown')
    expect(referenceStatusKind('')).toBe('unknown')
    expect(referenceStatusKind('deprecated')).toBe('deprecated')
    expect(referenceStatusKind('Deprecated')).toBe('deprecated')
    expect(referenceStatusKind('beta')).toBe('beta')
    expect(referenceStatusKind('stable')).toBe('other')
  })

  it('formats observed rates as trimmed percents and hides unknown', () => {
    expect(formatObservedRate(undefined)).toBe('')
    expect(formatObservedRate(null)).toBe('')
    expect(formatObservedRate(NaN)).toBe('')
    expect(formatObservedRate(-0.1)).toBe('')
    expect(formatObservedRate(1.2)).toBe('')
    expect(formatObservedRate(0)).toBe('0%')
    expect(formatObservedRate(1)).toBe('100%')
    expect(formatObservedRate(0.92)).toBe('92%')
    expect(formatObservedRate(0.9235)).toBe('92.35%')
  })

  it('formats the top failure stage/category with counts and hides empty failures', () => {
    expect(formatObservedFailure({})).toBe('')
    expect(formatObservedFailure({ topFailureCount: 0 })).toBe('')
    expect(
      formatObservedFailure({
        topFailureStage: 'render',
        topFailureCategory: 'timeout',
        topFailureCount: 7,
      }),
    ).toBe('render / timeout · 7 次')
    // 任一侧缺失只展已知侧；无 count 不拼次数。
    expect(formatObservedFailure({ topFailureStage: 'render', topFailureCount: 3 })).toBe(
      'render · 3 次',
    )
    expect(formatObservedFailure({ topFailureCategory: 'timeout' })).toBe('timeout')
  })

  it('filters on-sale SKU margins without dropping off-sale rows at the source', () => {
    const snapshot = {
      skuMargins: [
        { sku: 'ai-image:a', onSale: true },
        { sku: 'ai-image:b', onSale: false },
      ],
    } as unknown as ModelObservedSnapshot
    const onSale = observedOnSaleMargins(snapshot)
    expect(onSale.map((margin) => margin.sku)).toEqual(['ai-image:a'])
    expect(snapshot.skuMargins).toHaveLength(2)
  })

  it('never fabricates observed margins: null stays unknown', () => {
    expect(formatObservedMargin(undefined)).toBe('')
    expect(formatObservedMargin(null)).toBe('')
    expect(formatObservedMargin(12)).toBe('+12')
    expect(formatObservedMargin(-3)).toBe('-3')
    expect(formatObservedMargin(0)).toBe('0')
  })
})
