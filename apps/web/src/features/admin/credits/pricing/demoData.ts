import type {
  AdminBillableItemCard,
  AdminModelPool,
  AdminPoolCatalogModel,
  PricingVariant,
} from './billableCatalogTypes'

export const DEMO_MODE_QUERY_VALUE = '1'

const DEMO_IMAGE_ITEMS: readonly AdminBillableItemCard[] = [
  {
    sku: 'ai-image:generation',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '图片生成',
    sortOrder: 10,
    status: 'active',
    pricingVariants: ['1k', '2k', '4k'],
    currentPrices: {
      '1k': {
        priceVersionId: 101,
        version: 1,
        unitCredits: 2,
        createdAt: '2026-09-01T10:00:00Z',
      },
      '2k': {
        priceVersionId: 102,
        version: 1,
        unitCredits: 4,
        createdAt: '2026-09-01T10:00:00Z',
      },
      '4k': {
        priceVersionId: 103,
        version: 1,
        unitCredits: 8,
        createdAt: '2026-09-01T10:00:00Z',
      },
    },
    updatedAt: '2026-09-01T10:00:00Z',
  },
  {
    sku: 'ai-image:inpaint',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '局部重绘',
    sortOrder: 20,
    status: 'active',
    pricingVariants: ['1k', '2k', '4k'],
    currentPrices: {
      '1k': {
        priceVersionId: 104,
        version: 1,
        unitCredits: 3,
        createdAt: '2026-09-02T11:00:00Z',
      },
      '2k': {
        priceVersionId: 105,
        version: 1,
        unitCredits: 6,
        createdAt: '2026-09-02T11:00:00Z',
      },
    },
    updatedAt: '2026-09-02T11:00:00Z',
  },
  {
    sku: 'ai-image:cutout',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '一键抠图',
    sortOrder: 30,
    status: 'active',
    pricingVariants: ['1k'],
    currentPrices: {
      '1k': {
        priceVersionId: 106,
        version: 1,
        unitCredits: 1,
        createdAt: '2026-09-03T12:00:00Z',
      },
    },
    updatedAt: '2026-09-03T12:00:00Z',
  },
  {
    sku: 'ai-image:upscale',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '画质提升',
    sortOrder: 40,
    status: 'disabled',
    pricingVariants: ['1k', '2k', '4k'],
    currentPrices: {
      '1k': {
        priceVersionId: 107,
        version: 1,
        unitCredits: 2,
        createdAt: '2026-09-04T08:00:00Z',
      },
      '2k': {
        priceVersionId: 108,
        version: 1,
        unitCredits: 5,
        createdAt: '2026-09-04T08:00:00Z',
      },
      '4k': {
        priceVersionId: 109,
        version: 1,
        unitCredits: 10,
        createdAt: '2026-09-04T08:00:00Z',
      },
    },
    updatedAt: '2026-09-04T08:00:00Z',
  },
  {
    sku: 'ai-image:variations',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '风格延展',
    sortOrder: 50,
    status: 'unpriced',
    pricingVariants: ['1k', '2k', '4k'],
    currentPrices: {},
    updatedAt: '2026-09-05T09:00:00Z',
  },
  {
    sku: 'ai-image:outpaint',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '扩图延展',
    sortOrder: 60,
    status: 'active',
    pricingVariants: ['1k', '2k'],
    currentPrices: {
      '1k': {
        priceVersionId: 110,
        version: 1,
        unitCredits: 2,
        createdAt: '2026-09-06T14:00:00Z',
      },
      '2k': {
        priceVersionId: 111,
        version: 1,
        unitCredits: 4,
        createdAt: '2026-09-06T14:00:00Z',
      },
    },
    updatedAt: '2026-09-06T14:00:00Z',
  },
  {
    sku: 'ai-image:fusion',
    namespace: 'ai-image',
    productKind: 'workflow-feature',
    displayName: '多图融合',
    sortOrder: 70,
    status: 'unpriced',
    pricingVariants: ['1k', '2k', '4k'],
    currentPrices: {},
    updatedAt: '2026-09-07T15:00:00Z',
  },
]

const DEMO_OTHER_ITEMS: readonly AdminBillableItemCard[] = [
  {
    sku: 'ai-search:query',
    namespace: 'ai-search',
    productKind: 'workflow-feature',
    displayName: '智能搜索',
    sortOrder: 10,
    status: 'active',
    pricingVariants: ['1k'],
    currentPrices: {
      '1k': {
        priceVersionId: 201,
        version: 1,
        unitCredits: 1,
        createdAt: '2026-09-01T09:00:00Z',
      },
    },
    updatedAt: '2026-09-01T09:00:00Z',
  },
  {
    sku: 'ai-search:rerank',
    namespace: 'ai-search',
    productKind: 'workflow-feature',
    displayName: '语义重排',
    sortOrder: 20,
    status: 'active',
    pricingVariants: ['1k'],
    currentPrices: {
      '1k': {
        priceVersionId: 202,
        version: 1,
        unitCredits: 2,
        createdAt: '2026-09-01T09:00:00Z',
      },
    },
    updatedAt: '2026-09-01T09:00:00Z',
  },
  {
    sku: 'legacy:bg-remover',
    namespace: 'legacy',
    productKind: 'legacy',
    displayName: '旧版去底',
    sortOrder: 99,
    status: 'archived',
    pricingVariants: ['1k'],
    currentPrices: {
      '1k': {
        priceVersionId: 301,
        version: 1,
        unitCredits: 1,
        createdAt: '2026-08-01T08:00:00Z',
      },
    },
    updatedAt: '2026-08-01T08:00:00Z',
  },
]

export function createDemoBillingItems(): AdminBillableItemCard[] {
  return JSON.parse(
    JSON.stringify([...DEMO_IMAGE_ITEMS, ...DEMO_OTHER_ITEMS]),
  ) as AdminBillableItemCard[]
}

export function createDemoModelPools(): Record<string, AdminModelPool> {
  return {
    'ai-image:generation': {
      service: 'generation',
      sku: 'ai-image:generation',
      version: 1,
      entries: [
        { modelId: 'google/imagen-3', enabled: true, resolutions: ['1k', '2k', '4k'] },
        { modelId: 'flux/flux-1.1-pro', enabled: true, resolutions: ['1k', '2k'] },
        { modelId: 'black-forest-labs/flux-schnell', enabled: true, resolutions: ['1k'] },
      ],
    },
    'ai-image:inpaint': {
      service: 'inpaint',
      sku: 'ai-image:inpaint',
      version: 1,
      entries: [
        {
          modelId: 'stabilityai/stable-diffusion-xl-inpaint',
          enabled: true,
          resolutions: ['1k', '2k'],
        },
        { modelId: 'runway/inpaint-v2', enabled: true, resolutions: ['1k'] },
      ],
    },
    'ai-image:cutout': {
      service: 'cutout',
      sku: 'ai-image:cutout',
      version: 1,
      entries: [
        { modelId: 'bria-ai/rmbg-2.0', enabled: true, resolutions: ['1k'] },
        { modelId: 'bria-ai/image-rmbg', enabled: true, resolutions: ['1k'] },
      ],
    },
    'ai-image:upscale': {
      service: 'upscale',
      sku: 'ai-image:upscale',
      version: 1,
      entries: [
        { modelId: 'topaz/gigapixel-v2', enabled: true, resolutions: ['1k', '2k', '4k'] },
        { modelId: 'real-esrgan/v4', enabled: false, resolutions: ['1k', '2k'] },
      ],
    },
    'ai-image:variations': {
      service: 'variations',
      sku: 'ai-image:variations',
      version: 1,
      entries: [
        { modelId: 'midjourney/v6-style', enabled: true, resolutions: ['1k', '2k', '4k'] },
      ],
    },
    'ai-image:outpaint': {
      service: 'outpaint',
      sku: 'ai-image:outpaint',
      version: 1,
      entries: [
        { modelId: 'stabilityai/sdxl-outpaint', enabled: true, resolutions: ['1k', '2k'] },
      ],
    },
    'ai-image:fusion': {
      service: 'fusion',
      sku: 'ai-image:fusion',
      version: 1,
      entries: [
        { modelId: 'stabilityai/sdxl-blend', enabled: true, resolutions: ['1k', '2k', '4k'] },
      ],
    },
  }
}

export function createDemoCatalogModels(): AdminPoolCatalogModel[] {
  return [
    { id: 'google/imagen-3', name: 'Google Imagen 3' },
    { id: 'flux/flux-1.1-pro', name: 'FLUX 1.1 Pro' },
    { id: 'black-forest-labs/flux-schnell', name: 'FLUX.1 Schnell' },
    { id: 'stabilityai/stable-diffusion-xl-inpaint', name: 'SDXL Inpaint' },
    { id: 'stabilityai/sdxl-outpaint', name: 'SDXL Outpaint' },
    { id: 'stabilityai/sdxl-blend', name: 'SDXL Blend' },
    { id: 'bria-ai/rmbg-2.0', name: 'Bria RMBG 2.0' },
    { id: 'bria-ai/image-rmbg', name: 'Bria Image RMBG' },
    { id: 'topaz/gigapixel-v2', name: 'Topaz Gigapixel v2' },
    { id: 'real-esrgan/v4', name: 'Real-ESRGAN v4' },
    { id: 'midjourney/v6-style', name: 'Midjourney v6' },
    { id: 'openai/dall-e-3', name: 'OpenAI DALL-E 3' },
    { id: 'deepseek/janus-pro-7b', name: 'DeepSeek Janus Pro 7B' },
  ]
}

export function createDemoPricesUpdate(
  current: AdminBillableItemCard['currentPrices'],
  prices: Record<PricingVariant, number | undefined>,
): AdminBillableItemCard['currentPrices'] {
  const nowIso = new Date().toISOString()
  const result: AdminBillableItemCard['currentPrices'] = { ...current }
  for (const [v, credits] of Object.entries(prices) as [PricingVariant, number | undefined][]) {
    if (credits !== undefined) {
      result[v] = {
        priceVersionId: Math.floor(Math.random() * 1000) + 100,
        version: (current[v]?.version ?? 0) + 1,
        unitCredits: credits,
        createdAt: nowIso,
      }
    }
  }
  return result
}
