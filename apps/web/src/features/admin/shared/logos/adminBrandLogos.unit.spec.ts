import { describe, expect, it } from 'vitest'
import {
  ADMIN_BRAND_CATEGORIES,
  ALL_ADMIN_BRAND_LOGOS,
  detectAdminBrandLogoKey,
  getAdminBrandLogo,
  getAdminBrandLogoOverride,
  replaceAdminBrandLogoOverrides,
} from './adminBrandLogos'

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
})
