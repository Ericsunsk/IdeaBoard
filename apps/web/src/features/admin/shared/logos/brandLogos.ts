import { ref } from 'vue'
import { resolveApiUrl } from '../../../../shared/config/runtimeUrls'

// Brand logo override vocabulary (issue #809). The legacy generated models
// are gone; this module owns the scope/key sets its pickers and resources
// share. Values mirror the backend override registry.
const AdminBrandLogoOverrideScope = {
  'provider-endpoint': 'provider-endpoint',
  'storage-profile': 'storage-profile',
  'search-provider': 'search-provider',
  'oauth-provider': 'oauth-provider',
} as const

type AdminBrandLogoOverrideScope =
  (typeof AdminBrandLogoOverrideScope)[keyof typeof AdminBrandLogoOverrideScope]

const AdminBrandLogoOverrideUpdateLogoKey = {
  flux: 'flux',
  midjourney: 'midjourney',
  stability: 'stability',
  fal: 'fal',
  replicate: 'replicate',
  ideogram: 'ideogram',
  recraft: 'recraft',
  runway: 'runway',
  zhipu: 'zhipu',
  aliyun: 'aliyun',
  doubao: 'doubao',
  moonshot: 'moonshot',
  minimax: 'minimax',
  baidu: 'baidu',
  tencent: 'tencent',
  lingyi: 'lingyi',
  openai: 'openai',
  gemini: 'gemini',
  anthropic: 'anthropic',
  deepseek: 'deepseek',
  cloudflare: 'cloudflare',
  siliconflow: 'siliconflow',
  mistral: 'mistral',
  groq: 'groq',
  together: 'together',
  fireworks: 'fireworks',
  huggingface: 'huggingface',
  perplexity: 'perplexity',
  cohere: 'cohere',
  novita: 'novita',
  lepton: 'lepton',
  google: 'google',
  azure: 'azure',
  bedrock: 'bedrock',
  ollama: 'ollama',
  amazons3: 'amazons3',
  'cloudflare-r2': 'cloudflare-r2',
  alibabacloud: 'alibabacloud',
  'tencent-cos': 'tencent-cos',
  minio: 'minio',
  unsplash: 'unsplash',
  pexels: 'pexels',
  github: 'github',
  'google-oauth': 'google-oauth',
  discord: 'discord',
} as const

type AdminBrandLogoOverrideUpdateLogoKey =
  (typeof AdminBrandLogoOverrideUpdateLogoKey)[keyof typeof AdminBrandLogoOverrideUpdateLogoKey]

// Provider 品牌图走后端 models.dev 透传（GET /admin/settings/ai-gateway/brand-logo），
// 前端不再 bundle 任何 provider SVG：上游缺失时后端回默认图，卡片不断图。
// 存储与身份类目仍用站内现成资产（storage-vendors / oauth），不在本次迁移范围。
import logoAmazonS3 from '../../../../shared/assets/storage-vendors/amazons3.svg'
import logoAlibabaCloud from '../../../../shared/assets/storage-vendors/alibabacloud.svg'
import logoCloudflareVendor from '../../../../shared/assets/storage-vendors/cloudflare.svg'
import logoMinio from '../../../../shared/assets/storage-vendors/minio.svg'
import logoTencentCloud from '../../../../shared/assets/storage-vendors/tencentcloud.ico'

// 身份与认证
import logoGithub from '../../../../shared/assets/oauth/github.svg'
import logoGoogleOAuth from '../../../../shared/assets/oauth/google.svg'
import logoDiscord from '../../../../shared/assets/oauth/discord.svg'

export type AdminBrandCategoryKey =
  'all' | 'image-gen' | 'china-ai' | 'global-ai' | 'storage' | 'search' | 'auth'

export interface AdminBrandCategory {
  key: AdminBrandCategoryKey
  label: string
}
export interface AdminBrandLogoItem {
  key: AdminBrandLogoOverrideUpdateLogoKey
  label: string
  logoSrc: string
  category: AdminBrandCategoryKey
}

export type AdminBrandLogoScope = AdminBrandLogoOverrideScope
export type AdminBrandLogoKey = AdminBrandLogoOverrideUpdateLogoKey

const ADMIN_BRAND_LOGO_ENDPOINT = '/admin/settings/ai-gateway/brand-logo'

// models.dev 上游 provider ID 与站内 logoKey 不一致时的别名（均经 /logos/{id}.svg 实测命中）。
// 未列出的 key 按原样透传：上游命中即真图，未收录即回默认图（不断图），由 admin overrides 兜底。
const MODELS_DEV_LOGO_ALIASES: Partial<Record<AdminBrandLogoKey, string>> = {
  gemini: 'google',
  zhipu: 'zhipuai',
  moonshot: 'moonshotai',
  aliyun: 'alibaba',
  doubao: 'volcengine',
  bedrock: 'amazon-bedrock',
  together: 'togetherai',
  fireworks: 'fireworks-ai',
  novita: 'novita-ai',
}

export function adminBrandLogoProxyUrl(key: string): string {
  const provider = MODELS_DEV_LOGO_ALIASES[key as AdminBrandLogoKey] ?? key
  return resolveApiUrl(`${ADMIN_BRAND_LOGO_ENDPOINT}?provider=${encodeURIComponent(provider)}`)
}

export const ADMIN_BRAND_CATEGORIES: AdminBrandCategory[] = [
  { key: 'all', label: '全部' },
  { key: 'image-gen', label: '生图' },
  { key: 'china-ai', label: '国内' },
  { key: 'global-ai', label: '国际' },
  { key: 'storage', label: '存储' },
  { key: 'search', label: '素材' },
  { key: 'auth', label: '身份' },
]

export const ALL_ADMIN_BRAND_LOGOS: AdminBrandLogoItem[] = [
  // 1. 生图与多模态
  {
    key: 'flux',
    label: 'Flux (Black Forest Labs)',
    logoSrc: adminBrandLogoProxyUrl('flux'),
    category: 'image-gen',
  },
  {
    key: 'midjourney',
    label: 'Midjourney',
    logoSrc: adminBrandLogoProxyUrl('midjourney'),
    category: 'image-gen',
  },
  {
    key: 'stability',
    label: 'Stability AI',
    logoSrc: adminBrandLogoProxyUrl('stability'),
    category: 'image-gen',
  },
  { key: 'fal', label: 'Fal.ai', logoSrc: adminBrandLogoProxyUrl('fal'), category: 'image-gen' },
  {
    key: 'replicate',
    label: 'Replicate',
    logoSrc: adminBrandLogoProxyUrl('replicate'),
    category: 'image-gen',
  },
  {
    key: 'ideogram',
    label: 'Ideogram',
    logoSrc: adminBrandLogoProxyUrl('ideogram'),
    category: 'image-gen',
  },
  {
    key: 'recraft',
    label: 'Recraft',
    logoSrc: adminBrandLogoProxyUrl('recraft'),
    category: 'image-gen',
  },
  {
    key: 'runway',
    label: 'Runway',
    logoSrc: adminBrandLogoProxyUrl('runway'),
    category: 'image-gen',
  },

  // 2. 国内主流模型
  {
    key: 'zhipu',
    label: '智谱 AI (GLM / CogView)',
    logoSrc: adminBrandLogoProxyUrl('zhipu'),
    category: 'china-ai',
  },
  {
    key: 'aliyun',
    label: '阿里云 / 百炼 (DashScope / 万相)',
    logoSrc: adminBrandLogoProxyUrl('aliyun'),
    category: 'china-ai',
  },
  {
    key: 'doubao',
    label: '火山引擎 / 豆包 (ByteDance)',
    logoSrc: adminBrandLogoProxyUrl('doubao'),
    category: 'china-ai',
  },
  {
    key: 'moonshot',
    label: '月之暗面 (Moonshot / Kimi)',
    logoSrc: adminBrandLogoProxyUrl('moonshot'),
    category: 'china-ai',
  },
  {
    key: 'minimax',
    label: 'MiniMax (海螺)',
    logoSrc: adminBrandLogoProxyUrl('minimax'),
    category: 'china-ai',
  },
  {
    key: 'baidu',
    label: '百度千帆 (文心一言)',
    logoSrc: adminBrandLogoProxyUrl('baidu'),
    category: 'china-ai',
  },
  {
    key: 'tencent',
    label: '腾讯混元 (Hunyuan)',
    logoSrc: adminBrandLogoProxyUrl('tencent'),
    category: 'china-ai',
  },
  {
    key: 'lingyi',
    label: '零一万物 (01.AI)',
    logoSrc: adminBrandLogoProxyUrl('lingyi'),
    category: 'china-ai',
  },

  // 3. 国际主流模型与网关
  {
    key: 'openai',
    label: 'OpenAI',
    logoSrc: adminBrandLogoProxyUrl('openai'),
    category: 'global-ai',
  },
  {
    key: 'gemini',
    label: 'Gemini',
    logoSrc: adminBrandLogoProxyUrl('gemini'),
    category: 'global-ai',
  },
  {
    key: 'anthropic',
    label: 'Claude / Anthropic',
    logoSrc: adminBrandLogoProxyUrl('anthropic'),
    category: 'global-ai',
  },
  {
    key: 'deepseek',
    label: 'DeepSeek',
    logoSrc: adminBrandLogoProxyUrl('deepseek'),
    category: 'global-ai',
  },
  {
    key: 'cloudflare',
    label: 'Cloudflare Workers AI',
    logoSrc: adminBrandLogoProxyUrl('cloudflare'),
    category: 'global-ai',
  },
  {
    key: 'siliconflow',
    label: 'SiliconFlow (硅基流动)',
    logoSrc: adminBrandLogoProxyUrl('siliconflow'),
    category: 'global-ai',
  },
  {
    key: 'mistral',
    label: 'Mistral AI',
    logoSrc: adminBrandLogoProxyUrl('mistral'),
    category: 'global-ai',
  },
  { key: 'groq', label: 'Groq', logoSrc: adminBrandLogoProxyUrl('groq'), category: 'global-ai' },
  {
    key: 'together',
    label: 'Together AI',
    logoSrc: adminBrandLogoProxyUrl('together'),
    category: 'global-ai',
  },
  {
    key: 'fireworks',
    label: 'Fireworks AI',
    logoSrc: adminBrandLogoProxyUrl('fireworks'),
    category: 'global-ai',
  },
  {
    key: 'huggingface',
    label: 'Hugging Face',
    logoSrc: adminBrandLogoProxyUrl('huggingface'),
    category: 'global-ai',
  },
  {
    key: 'perplexity',
    label: 'Perplexity AI',
    logoSrc: adminBrandLogoProxyUrl('perplexity'),
    category: 'global-ai',
  },
  {
    key: 'cohere',
    label: 'Cohere',
    logoSrc: adminBrandLogoProxyUrl('cohere'),
    category: 'global-ai',
  },
  {
    key: 'novita',
    label: 'Novita AI',
    logoSrc: adminBrandLogoProxyUrl('novita'),
    category: 'global-ai',
  },
  {
    key: 'lepton',
    label: 'Lepton AI',
    logoSrc: adminBrandLogoProxyUrl('lepton'),
    category: 'global-ai',
  },
  {
    key: 'google',
    label: 'Google Cloud / Vertex AI',
    logoSrc: adminBrandLogoProxyUrl('google'),
    category: 'global-ai',
  },
  {
    key: 'azure',
    label: 'Microsoft Azure OpenAI',
    logoSrc: adminBrandLogoProxyUrl('azure'),
    category: 'global-ai',
  },
  {
    key: 'bedrock',
    label: 'Amazon Bedrock / AWS',
    logoSrc: adminBrandLogoProxyUrl('bedrock'),
    category: 'global-ai',
  },
  {
    key: 'ollama',
    label: 'Ollama (本地部署)',
    logoSrc: adminBrandLogoProxyUrl('ollama'),
    category: 'global-ai',
  },

  // 4. 对象存储（站内现成资产，不走 models.dev）
  { key: 'amazons3', label: 'Amazon S3', logoSrc: logoAmazonS3, category: 'storage' },
  { key: 'cloudflare-r2', label: 'Cloudflare R2', logoSrc: logoCloudflareVendor, category: 'storage' },
  { key: 'alibabacloud', label: '阿里云 OSS', logoSrc: logoAlibabaCloud, category: 'storage' },
  { key: 'tencent-cos', label: '腾讯云 COS', logoSrc: logoTencentCloud, category: 'storage' },
  { key: 'minio', label: 'MinIO', logoSrc: logoMinio, category: 'storage' },

  // 5. 搜索素材库
  {
    key: 'unsplash',
    label: 'Unsplash',
    logoSrc: adminBrandLogoProxyUrl('unsplash'),
    category: 'search',
  },
  {
    key: 'pexels',
    label: 'Pexels',
    logoSrc: adminBrandLogoProxyUrl('pexels'),
    category: 'search',
  },

  // 6. 身份与登录源（站内现成资产，不走 models.dev）
  { key: 'github', label: 'GitHub', logoSrc: logoGithub, category: 'auth' },
  { key: 'google-oauth', label: 'Google 账号', logoSrc: logoGoogleOAuth, category: 'auth' },
  { key: 'discord', label: 'Discord', logoSrc: logoDiscord, category: 'auth' },
]

const adminBrandLogoMap: Record<string, string> = Object.fromEntries(
  ALL_ADMIN_BRAND_LOGOS.map((item) => [item.key, item.logoSrc]),
)

const adminBrandLogoOverrides = ref<Record<string, string>>({})

export function replaceAdminBrandLogoOverrides(
  overrides: Array<{ scope: AdminBrandLogoScope; targetKey: string; logoKey: AdminBrandLogoKey }>,
) {
  adminBrandLogoOverrides.value = Object.fromEntries(
    overrides.map((override) => [`${override.scope}:${override.targetKey}`, override.logoKey]),
  )
}

export function getAdminBrandLogoOverride(
  scope: AdminBrandLogoScope,
  id: string | number,
): string | undefined {
  const compoundKey = `${scope}:${id}`
  return adminBrandLogoOverrides.value[compoundKey]
}

type AdminBrandLogoMatchRule = {
  key: string
  anyOf: readonly string[]
  allOf?: readonly string[]
}

// Ordered: first matching rule wins. Keep this order in sync with the
// previous if-chain semantics when adding providers.
const ADMIN_BRAND_LOGO_MATCH_RULES: readonly AdminBrandLogoMatchRule[] = [
  { key: 'minio', anyOf: ['minio', ':9000'] },
  { key: 'cloudflare-r2', anyOf: ['r2.cloudflarestorage.com', 'cloudflare-r2'] },
  { key: 'alibabacloud', allOf: ['aliyuncs.com'], anyOf: ['oss-', 's3-compatible'] },
  { key: 'tencent-cos', allOf: ['myqcloud.com'], anyOf: ['cos.', 'tencent-cos'] },
  { key: 'amazons3', allOf: ['amazonaws.com'], anyOf: ['s3.', 'aws-s3', 's3-compatible'] },
  { key: 'cloudflare', anyOf: ['cloudflare', 'workers.dev', 'workers ai', 'workers-ai'] },
  { key: 'flux', anyOf: ['flux', 'bfl', 'blackforestlabs'] },
  { key: 'fal', anyOf: ['fal.ai', 'fal-ai', 'fal_ai', 'fal.'] },
  { key: 'replicate', anyOf: ['replicate'] },
  { key: 'ideogram', anyOf: ['ideogram'] },
  { key: 'recraft', anyOf: ['recraft'] },
  { key: 'runway', anyOf: ['runway', 'runwayml'] },
  { key: 'zhipu', anyOf: ['zhipu', 'bigmodel', 'cogview', 'glm', '智谱'] },
  { key: 'aliyun', anyOf: ['dashscope', 'aliyun', 'wanx', 'qwen', '通义', '千问'] },
  { key: 'doubao', anyOf: ['volcengine', 'volces', 'doubao', 'bytedance', '豆包'] },
  { key: 'moonshot', anyOf: ['moonshot', 'kimi', '月之暗面'] },
  { key: 'minimax', anyOf: ['minimax', 'hailuo', '海螺'] },
  { key: 'baidu', anyOf: ['baidu', 'baidubce', 'qianfan', 'wenxin', '文心'] },
  { key: 'tencent', anyOf: ['tencent', 'hunyuan', '混元'] },
  { key: 'lingyi', anyOf: ['lingyi', '01.ai', 'yi-'] },
  { key: 'gemini', anyOf: ['gemini', 'google-ai', 'aiplatform', 'generativelanguage'] },
  { key: 'google', anyOf: ['google', 'vertex', 'imagen'] },
  { key: 'anthropic', anyOf: ['anthropic', 'claude'] },
  { key: 'deepseek', anyOf: ['deepseek'] },
  { key: 'siliconflow', anyOf: ['siliconflow', 'silicon'] },
  { key: 'stability', anyOf: ['stability', 'dreamstudio', 'stable-diffusion', 'sdxl', 'sd3'] },
  { key: 'midjourney', anyOf: ['midjourney'] },
  { key: 'mistral', anyOf: ['mistral', 'mixtral', 'codestral', 'pixtral'] },
  { key: 'groq', anyOf: ['groq'] },
  { key: 'together', anyOf: ['together'] },
  { key: 'fireworks', anyOf: ['fireworks'] },
  { key: 'huggingface', anyOf: ['huggingface', 'hf.co'] },
  { key: 'perplexity', anyOf: ['perplexity'] },
  { key: 'cohere', anyOf: ['cohere'] },
  { key: 'azure', anyOf: ['azure', 'openai.azure.com'] },
  { key: 'bedrock', anyOf: ['bedrock'] },
  { key: 'ollama', anyOf: ['ollama', ':11434'] },
  { key: 'novita', anyOf: ['novita'] },
  { key: 'lepton', anyOf: ['lepton'] },
  { key: 'unsplash', anyOf: ['unsplash'] },
  { key: 'pexels', anyOf: ['pexels'] },
  { key: 'github', anyOf: ['github'] },
  { key: 'discord', anyOf: ['discord'] },
  { key: 'openai', anyOf: ['openai', 'chatgpt', 'gpt', 'dall-e', 'dalle', 'o1-', 'o3-', 'sora'] },
]

function matchesBrandLogoRule(text: string, rule: AdminBrandLogoMatchRule): boolean {
  if (rule.allOf && !rule.allOf.every((part) => text.includes(part))) return false
  return rule.anyOf.some((part) => text.includes(part))
}

export function detectAdminBrandLogoKey(query: {
  name?: string | undefined
  baseUrl?: string | undefined
  apiType?: string | undefined
  provider?: string | undefined
  endpoint?: string | undefined
}): string | undefined {
  const text =
    `${query.name || ''} ${query.baseUrl || ''} ${query.apiType || ''} ${query.provider || ''} ${query.endpoint || ''}`.toLowerCase()

  return ADMIN_BRAND_LOGO_MATCH_RULES.find((rule) => matchesBrandLogoRule(text, rule))?.key
}

export function getAdminBrandLogo(
  query: {
    name?: string | undefined
    baseUrl?: string | undefined
    apiType?: string
    provider?: string
    endpoint?: string
  },
  scope?: AdminBrandLogoScope,
  targetId?: string | number,
): string | undefined {
  if (scope && targetId !== undefined) {
    const override = getAdminBrandLogoOverride(scope, targetId)
    if (override && adminBrandLogoMap[override]) {
      return adminBrandLogoMap[override]
    }
  }

  const detectedKey = detectAdminBrandLogoKey(query)
  return detectedKey ? adminBrandLogoMap[detectedKey] : undefined
}

const modelBrandLogoCache = new Map<string, string | undefined>()

export function resolveModelBrandLogo(model: {
  id?: string | undefined
  name?: string | undefined
  provider?: string | undefined
}): string | undefined {
  // 列表每行模板多次调用：按 identity 缓存，~40 条规则只扫一次。
  const cacheKey = `${model.id || ''}\n${model.name || ''}\n${model.provider || ''}`
  const cached = modelBrandLogoCache.get(cacheKey)
  if (cached !== undefined || modelBrandLogoCache.has(cacheKey)) return cached
  // 1. 优先根据模型 ID 和模型名称识别具体品牌 (例如 gemini, gpt, claude, deepseek, flux, sdxl 等)
  const specificText = `${model.id || ''} ${model.name || ''}`.trim()
  if (specificText) {
    const key = detectAdminBrandLogoKey({ name: specificText })
    if (key && adminBrandLogoMap[key]) {
      const hit = adminBrandLogoMap[key]
      modelBrandLogoCache.set(cacheKey, hit)
      return hit
    }
  }

  // 2. 若模型名称未命中，尝试根据提供商名称识别 (例如 Fal AI, Cloudflare, OpenAI, 智谱 等)
  if (model.provider) {
    const key = detectAdminBrandLogoKey({ provider: model.provider })
    if (key && adminBrandLogoMap[key]) {
      const hit = adminBrandLogoMap[key]
      modelBrandLogoCache.set(cacheKey, hit)
      return hit
    }
  }

  // 3. 兜底尝试所有字段组合
  const fallbackKey = detectAdminBrandLogoKey({
    name: model.name,
    endpoint: model.id,
    provider: model.provider,
  })
  const fallback = fallbackKey ? adminBrandLogoMap[fallbackKey] : undefined
  modelBrandLogoCache.set(cacheKey, fallback)
  return fallback
}
