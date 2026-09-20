import { describe, expect, it } from 'vitest'
import {
  formatCachePricing,
  formatModelPricing,
  isCloudflareHosted,
  matchesModelKeyword,
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_OPTIONS,
  MODEL_HOSTING_THIRD_PARTY,
  modelProviderBadgeClass,
  type AdminModelItem,
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

  it('formats community pricing per 1M tokens and hides unknown', () => {
    expect(formatModelPricing({})).toBe('')
    expect(formatModelPricing({ inputCostPerToken: 0, outputCostPerToken: -1 })).toBe('')
    expect(formatModelPricing({ inputCostPerToken: 0.0000025, outputCostPerToken: 0.00001 })).toBe(
      '$2.5 in · $10 out /1M',
    )
    expect(formatModelPricing({ inputCostPerToken: 0.000000075 })).toBe('$0.075 in /1M')
    expect(formatModelPricing({ outputCostPerToken: 0.000002 })).toBe('$2 out /1M')
  })

  it('formats cache read/write pricing only when present', () => {
    expect(formatCachePricing({})).toBe('')
    expect(
      formatCachePricing({ cacheReadCostPerToken: 0.00000125, cacheCreateCostPerToken: 0.000002 }),
    ).toBe('缓存 读 $1.25 · 写 $2 /1M')
    expect(formatCachePricing({ cacheReadCostPerToken: 0.00000125 })).toBe('缓存 读 $1.25 /1M')
  })
})
