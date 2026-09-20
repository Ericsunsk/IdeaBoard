import { describe, expect, it } from 'vitest'
import {
  ADMIN_BRAND_CATEGORIES,
  ALL_ADMIN_BRAND_LOGOS,
  detectAdminBrandLogoKey,
  getAdminBrandLogo,
  getAdminBrandLogoOverride,
  replaceAdminBrandLogoOverrides,
  resolveModelBrandLogo,
} from './brandLogos'

describe('adminBrandLogos', () => {
  it('defines all major brand categories', () => {
    const keys = ADMIN_BRAND_CATEGORIES.map((c) => c.key)
    expect(keys).toContain('all')
    expect(keys).toContain('image-gen')
    expect(keys).toContain('china-ai')
    expect(keys).toContain('global-ai')
    expect(keys).toContain('storage')
    expect(keys).toContain('search')
    expect(keys).toContain('auth')
  })

  it('contains logos across all categories', () => {
    expect(ALL_ADMIN_BRAND_LOGOS.length).toBeGreaterThanOrEqual(35)
    expect(ALL_ADMIN_BRAND_LOGOS.some((l) => l.category === 'image-gen')).toBe(true)
    expect(ALL_ADMIN_BRAND_LOGOS.some((l) => l.category === 'china-ai')).toBe(true)
    expect(ALL_ADMIN_BRAND_LOGOS.some((l) => l.category === 'global-ai')).toBe(true)
    expect(ALL_ADMIN_BRAND_LOGOS.some((l) => l.category === 'storage')).toBe(true)
  })

  it('detects brand logos by keyword, domain, and endpoint patterns', () => {
    // 存储识别
    expect(
      detectAdminBrandLogoKey({
        provider: 's3-compatible',
        endpoint: 'oss-cn-hangzhou.aliyuncs.com',
      }),
    ).toBe('alibabacloud')
    expect(
      detectAdminBrandLogoKey({
        provider: 's3-compatible',
        endpoint: 'cos.ap-guangzhou.myqcloud.com',
      }),
    ).toBe('tencent-cos')
    expect(detectAdminBrandLogoKey({ provider: 'minio', endpoint: 'http://127.0.0.1:9000' })).toBe(
      'minio',
    )
    expect(
      detectAdminBrandLogoKey({
        provider: 'cloudflare-r2',
        endpoint: 'https://xxx.r2.cloudflarestorage.com',
      }),
    ).toBe('cloudflare-r2')

    // AI 模型识别
    expect(detectAdminBrandLogoKey({ name: 'Flux Pro', baseUrl: 'https://api.bfl.ai' })).toBe(
      'flux',
    )
    expect(detectAdminBrandLogoKey({ name: 'DeepSeek', baseUrl: 'https://api.deepseek.com' })).toBe(
      'deepseek',
    )
    expect(detectAdminBrandLogoKey({ name: 'GLM-4', baseUrl: 'https://open.bigmodel.cn' })).toBe(
      'zhipu',
    )
    expect(detectAdminBrandLogoKey({ name: 'Kimi Chat', baseUrl: 'https://api.moonshot.cn' })).toBe(
      'moonshot',
    )
  })

  it('renders server-backed scoped overrides from the shared cache', () => {
    replaceAdminBrandLogoOverrides([
      { scope: 'storage-profile', targetKey: 'storage-101', logoKey: 'alibabacloud' },
    ])
    expect(getAdminBrandLogoOverride('storage-profile', 'storage-101')).toBe('alibabacloud')
    expect(
      getAdminBrandLogo({ name: 'Custom Minio' }, 'storage-profile', 'storage-101'),
    ).toBeDefined()

    replaceAdminBrandLogoOverrides([])
    expect(getAdminBrandLogoOverride('storage-profile', 'storage-101')).toBeUndefined()
  })

  it('resolves model brand logo dynamically based on model name, id, and provider', () => {
    const getLogo = (key: string) => ALL_ADMIN_BRAND_LOGOS.find((l) => l.key === key)?.logoSrc

    // 1. 用户真实截图场景：模型名为 Google/Gemini，但提供商为 OpenRouter
    const geminiLogo = resolveModelBrandLogo({
      id: 'google/gemini-3.1-flash-lite-image',
      name: 'Google: Nano Banana 2 Lite (Gemini 3.1 Flash Lite Image)',
      provider: 'OpenRouter',
    })
    expect(geminiLogo).toBe(getLogo('gemini'))

    // 2. OpenAI 模型
    const openaiLogo = resolveModelBrandLogo({
      id: 'openai/gpt-4o',
      name: 'GPT-4o Omnimodal',
      provider: 'OpenRouter',
    })
    expect(openaiLogo).toBe(getLogo('openai'))

    // 3. Anthropic Claude 模型
    const claudeLogo = resolveModelBrandLogo({
      id: 'anthropic/claude-3.5-sonnet',
      name: 'Claude 3.5 Sonnet',
      provider: 'Cloudflare',
    })
    expect(claudeLogo).toBe(getLogo('anthropic'))

    // 4. 模型名称为通用名，但提供商为 Fal AI -> 兜底为 Fal 图标
    const falLogo = resolveModelBrandLogo({
      id: 'fal-ai/birefnet',
      name: 'BiRefNet 高清智能抠图',
      provider: 'Fal AI',
    })
    expect(falLogo).toBe(getLogo('fal'))

    // 5. 完全未知的自建模型 -> 返回 undefined (回退到 Cube 图标)
    const unknownLogo = resolveModelBrandLogo({
      id: 'custom-vendor/custom-model-xyz',
      name: 'My Custom Model',
      provider: 'PrivateHost',
    })
    expect(unknownLogo).toBeUndefined()
  })
})
