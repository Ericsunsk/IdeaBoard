import { describe, expect, it } from 'vitest'
import {
  modelIdentityKey,
  displayModelId,
  formatObservedFailure,
  formatObservedMargin,
  formatObservedRate,
  isCloudflareHosted,
  matchesModelKeyword,
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_OPTIONS,
  MODEL_HOSTING_THIRD_PARTY,
  observedOnSaleMargins,
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

  it('renders native nested names with their provider and keys same-name models separately', () => {
    expect(displayModelId({ id: 'gpt-image-1', provider: 'openai' })).toBe('openai/gpt-image-1')
    expect(displayModelId({ id: 'google/gemini-flash', provider: 'openrouter' })).toBe(
      'openrouter/google/gemini-flash',
    )
    expect(modelIdentityKey({ id: ' GPT-IMAGE-1 ', provider: 'OpenAI' })).toBe('openai/gpt-image-1')
    expect(modelIdentityKey({ id: 'gpt-image-1', provider: 'azureopenai' })).not.toBe(
      modelIdentityKey({ id: 'gpt-image-1', provider: 'openai' }),
    )
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
