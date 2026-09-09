import { ref } from 'vue'
import type {
  AdminBrandLogoOverrideScope,
  AdminBrandLogoOverrideUpdateLogoKey,
} from '../../../../shared/api/generated-query/model'

// 生图与多模态
import logoFlux from '../../../../shared/assets/provider-logos/flux.svg'
import logoMidjourney from '../../../../shared/assets/provider-logos/midjourney.svg'
import logoStability from '../../../../shared/assets/provider-logos/stability.svg'
import logoFal from '../../../../shared/assets/provider-logos/fal.svg'
import logoReplicate from '../../../../shared/assets/provider-logos/replicate.svg'
import logoIdeogram from '../../../../shared/assets/provider-logos/ideogram.svg'
import logoRecraft from '../../../../shared/assets/provider-logos/recraft.svg'
import logoRunway from '../../../../shared/assets/provider-logos/runway.svg'

// 国内大模型
import logoZhipu from '../../../../shared/assets/provider-logos/zhipu.svg'
import logoAliyun from '../../../../shared/assets/provider-logos/aliyun.svg'
import logoDoubao from '../../../../shared/assets/provider-logos/doubao.svg'
import logoMoonshot from '../../../../shared/assets/provider-logos/moonshot.svg'
import logoMinimax from '../../../../shared/assets/provider-logos/minimax.svg'
import logoBaidu from '../../../../shared/assets/provider-logos/baidu.svg'
import logoTencent from '../../../../shared/assets/provider-logos/tencent.svg'
import logoLingyi from '../../../../shared/assets/provider-logos/lingyi.svg'

// 国际大模型与网关
import logoOpenAI from '../../../../shared/assets/provider-logos/openai.svg'
import logoGemini from '../../../../shared/assets/provider-logos/gemini.svg'
import logoAnthropic from '../../../../shared/assets/provider-logos/anthropic.svg'
import logoDeepSeek from '../../../../shared/assets/provider-logos/deepseek.svg'
import logoCloudflare from '../../../../shared/assets/provider-logos/cloudflare.svg'
import logoSiliconFlow from '../../../../shared/assets/provider-logos/siliconflow.svg'
import logoMistral from '../../../../shared/assets/provider-logos/mistral.svg'
import logoGroq from '../../../../shared/assets/provider-logos/groq.svg'
import logoTogether from '../../../../shared/assets/provider-logos/together.svg'
import logoFireworks from '../../../../shared/assets/provider-logos/fireworks.svg'
import logoHuggingFace from '../../../../shared/assets/provider-logos/huggingface.svg'
import logoPerplexity from '../../../../shared/assets/provider-logos/perplexity.svg'
import logoCohere from '../../../../shared/assets/provider-logos/cohere.svg'
import logoNovita from '../../../../shared/assets/provider-logos/novita.svg'
import logoLepton from '../../../../shared/assets/provider-logos/lepton.svg'

// 云计算与私有化部署
import logoGoogle from '../../../../shared/assets/provider-logos/google.svg'
import logoAzure from '../../../../shared/assets/provider-logos/azure.svg'
import logoBedrock from '../../../../shared/assets/provider-logos/bedrock.svg'
import logoOllama from '../../../../shared/assets/provider-logos/ollama.svg'

// 存储专用资产
import logoAmazonS3 from '../../../../shared/assets/storage-vendors/amazons3.svg'
import logoAlibabaCloud from '../../../../shared/assets/storage-vendors/alibabacloud.svg'
import logoMinio from '../../../../shared/assets/storage-vendors/minio.svg'

// 搜索素材库
import logoUnsplash from '../../../../shared/assets/provider-logos/unsplash.svg'
import logoPexels from '../../../../shared/assets/provider-logos/pexels.svg'

// 身份与认证
import logoGithub from '../../../../shared/assets/oauth/github.svg'
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
  { key: 'flux', label: 'Flux (Black Forest Labs)', logoSrc: logoFlux, category: 'image-gen' },
  { key: 'midjourney', label: 'Midjourney', logoSrc: logoMidjourney, category: 'image-gen' },
  { key: 'stability', label: 'Stability AI', logoSrc: logoStability, category: 'image-gen' },
  { key: 'fal', label: 'Fal.ai', logoSrc: logoFal, category: 'image-gen' },
  { key: 'replicate', label: 'Replicate', logoSrc: logoReplicate, category: 'image-gen' },
  { key: 'ideogram', label: 'Ideogram', logoSrc: logoIdeogram, category: 'image-gen' },
  { key: 'recraft', label: 'Recraft', logoSrc: logoRecraft, category: 'image-gen' },
  { key: 'runway', label: 'Runway', logoSrc: logoRunway, category: 'image-gen' },

  // 2. 国内主流模型
  { key: 'zhipu', label: '智谱 AI (GLM / CogView)', logoSrc: logoZhipu, category: 'china-ai' },
  {
    key: 'aliyun',
    label: '阿里云 / 百炼 (DashScope / 万相)',
    logoSrc: logoAliyun,
    category: 'china-ai',
  },
  {
    key: 'doubao',
    label: '火山引擎 / 豆包 (ByteDance)',
    logoSrc: logoDoubao,
    category: 'china-ai',
  },
  {
    key: 'moonshot',
    label: '月之暗面 (Moonshot / Kimi)',
    logoSrc: logoMoonshot,
    category: 'china-ai',
  },
  { key: 'minimax', label: 'MiniMax (海螺)', logoSrc: logoMinimax, category: 'china-ai' },
  { key: 'baidu', label: '百度千帆 (文心一言)', logoSrc: logoBaidu, category: 'china-ai' },
  { key: 'tencent', label: '腾讯混元 (Hunyuan)', logoSrc: logoTencent, category: 'china-ai' },
  { key: 'lingyi', label: '零一万物 (01.AI)', logoSrc: logoLingyi, category: 'china-ai' },

  // 3. 国际主流模型与网关
  { key: 'openai', label: 'OpenAI', logoSrc: logoOpenAI, category: 'global-ai' },
  { key: 'gemini', label: 'Gemini', logoSrc: logoGemini, category: 'global-ai' },
  { key: 'anthropic', label: 'Claude / Anthropic', logoSrc: logoAnthropic, category: 'global-ai' },
  { key: 'deepseek', label: 'DeepSeek', logoSrc: logoDeepSeek, category: 'global-ai' },
  {
    key: 'cloudflare',
    label: 'Cloudflare Workers AI',
    logoSrc: logoCloudflare,
    category: 'global-ai',
  },
  {
    key: 'siliconflow',
    label: 'SiliconFlow (硅基流动)',
    logoSrc: logoSiliconFlow,
    category: 'global-ai',
  },
  { key: 'mistral', label: 'Mistral AI', logoSrc: logoMistral, category: 'global-ai' },
  { key: 'groq', label: 'Groq', logoSrc: logoGroq, category: 'global-ai' },
  { key: 'together', label: 'Together AI', logoSrc: logoTogether, category: 'global-ai' },
  { key: 'fireworks', label: 'Fireworks AI', logoSrc: logoFireworks, category: 'global-ai' },
  { key: 'huggingface', label: 'Hugging Face', logoSrc: logoHuggingFace, category: 'global-ai' },
  { key: 'perplexity', label: 'Perplexity AI', logoSrc: logoPerplexity, category: 'global-ai' },
  { key: 'cohere', label: 'Cohere', logoSrc: logoCohere, category: 'global-ai' },
  { key: 'novita', label: 'Novita AI', logoSrc: logoNovita, category: 'global-ai' },
  { key: 'lepton', label: 'Lepton AI', logoSrc: logoLepton, category: 'global-ai' },
  { key: 'google', label: 'Google Cloud / Vertex AI', logoSrc: logoGoogle, category: 'global-ai' },
  { key: 'azure', label: 'Microsoft Azure OpenAI', logoSrc: logoAzure, category: 'global-ai' },
  { key: 'bedrock', label: 'Amazon Bedrock / AWS', logoSrc: logoBedrock, category: 'global-ai' },
  { key: 'ollama', label: 'Ollama (本地部署)', logoSrc: logoOllama, category: 'global-ai' },

  // 4. 对象存储
  { key: 'amazons3', label: 'Amazon S3', logoSrc: logoAmazonS3, category: 'storage' },
  { key: 'cloudflare-r2', label: 'Cloudflare R2', logoSrc: logoCloudflare, category: 'storage' },
  { key: 'alibabacloud', label: '阿里云 OSS', logoSrc: logoAlibabaCloud, category: 'storage' },
  { key: 'tencent-cos', label: '腾讯云 COS', logoSrc: logoTencent, category: 'storage' },
  { key: 'minio', label: 'MinIO', logoSrc: logoMinio, category: 'storage' },

  // 5. 搜索素材库
  { key: 'unsplash', label: 'Unsplash', logoSrc: logoUnsplash, category: 'search' },
  { key: 'pexels', label: 'Pexels', logoSrc: logoPexels, category: 'search' },

  // 6. 身份与登录源
  { key: 'github', label: 'GitHub', logoSrc: logoGithub, category: 'auth' },
  { key: 'google-oauth', label: 'Google 账号', logoSrc: logoGoogle, category: 'auth' },
  { key: 'discord', label: 'Discord', logoSrc: logoDiscord, category: 'auth' },
]

export const adminBrandLogoMap: Record<string, string> = Object.fromEntries(
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
  { key: 'cloudflare', anyOf: ['cloudflare', 'workers.dev'] },
  { key: 'flux', anyOf: ['flux', 'bfl', 'blackforestlabs'] },
  { key: 'fal', anyOf: ['fal.ai', 'fal-ai', 'fal_ai', 'fal.'] },
  { key: 'replicate', anyOf: ['replicate'] },
  { key: 'ideogram', anyOf: ['ideogram'] },
  { key: 'recraft', anyOf: ['recraft'] },
  { key: 'runway', anyOf: ['runway', 'runwayml'] },
  { key: 'zhipu', anyOf: ['zhipu', 'bigmodel', 'cogview', 'glm'] },
  { key: 'aliyun', anyOf: ['dashscope', 'aliyun', 'wanx', 'qwen'] },
  { key: 'doubao', anyOf: ['volcengine', 'volces', 'doubao', 'bytedance'] },
  { key: 'moonshot', anyOf: ['moonshot', 'kimi'] },
  { key: 'minimax', anyOf: ['minimax', 'hailuo'] },
  { key: 'baidu', anyOf: ['baidu', 'baidubce', 'qianfan', 'wenxin'] },
  { key: 'tencent', anyOf: ['tencent', 'hunyuan'] },
  { key: 'lingyi', anyOf: ['lingyi', '01.ai', 'yi-'] },
  { key: 'gemini', anyOf: ['gemini', 'google-ai', 'aiplatform', 'generativelanguage'] },
  { key: 'google', anyOf: ['google', 'vertex'] },
  { key: 'anthropic', anyOf: ['anthropic', 'claude'] },
  { key: 'deepseek', anyOf: ['deepseek'] },
  { key: 'siliconflow', anyOf: ['siliconflow', 'silicon'] },
  { key: 'stability', anyOf: ['stability', 'dreamstudio', 'stable-diffusion'] },
  { key: 'midjourney', anyOf: ['midjourney'] },
  { key: 'mistral', anyOf: ['mistral'] },
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
  { key: 'openai', anyOf: ['openai', 'chatgpt', 'gpt'] },
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
