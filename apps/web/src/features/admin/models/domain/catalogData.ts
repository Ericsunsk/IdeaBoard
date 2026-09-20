export interface AdminModelItem {
  id: string
  name: string
  provider: string
  // 社区 mode 原样透传（chat / image_generation …）：展示与动态过滤用，
  // 不做站内分类体系，缺失即未知。
  // NOTE: optional 字段一律显式 `| undefined`（exactOptionalPropertyTypes 下
  // 直接透传上游 optional 值，否则整字面量 TS2375）。
  mode?: string | undefined
  description: string
  contextOrResolution: string
  capabilities: string[]
  hosting: 'Cloudflare 托管' | '第三方'
  termsUrl: string
  docsUrl: string
  customPath?: string | undefined
  customParams?: string | undefined
  // 富化位：true = 简介来自探测自动填充，可被更新的探测覆盖；
  // false/undefined = 运营手写，刷新富化永不触碰。空简介一律视为缺失可补。
  descriptionEnriched?: boolean | undefined
  // 社区参考价（USD/token），展示用，不进计费；缺失即未知，不展示。
  inputCostPerToken?: number | undefined
  outputCostPerToken?: number | undefined
  cacheReadCostPerToken?: number | undefined
  cacheCreateCostPerToken?: number | undefined
  supportsPromptCaching?: boolean | undefined
}

// 托管文案单源：逻辑层一律引用常量，模板展示绑定除外。
export const MODEL_HOSTING_CLOUDFLARE = 'Cloudflare 托管'
export const MODEL_HOSTING_THIRD_PARTY = '第三方'
export const MODEL_HOSTING_OPTIONS = [MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY] as const

export function isCloudflareHosted(provider: string): boolean {
  const normalized = (provider || '').toLowerCase()
  return normalized.includes('workers') || normalized.includes('cloudflare')
}

// 关键词过滤单源：列表页与下拉选用同一匹配语义
//（名称/标识/供应商/mode/描述），新增字段只改这里。
export function matchesModelKeyword(model: AdminModelItem, keyword: string): boolean {
  const kw = keyword.trim().toLowerCase()
  if (!kw) return true
  return (
    model.name.toLowerCase().includes(kw) ||
    model.id.toLowerCase().includes(kw) ||
    model.provider.toLowerCase().includes(kw) ||
    (model.mode || '').toLowerCase().includes(kw) ||
    model.description.toLowerCase().includes(kw)
  )
}

// 价格展示单源：USD/token 换算为 $/1M tokens，缺失即未知（返回 '' 不展示）。
// Why 按 1M 计：社区价多为 1e-7 量级，直接展示 token 单价不可读。
export function formatModelPricing(item: Pick<AdminModelItem, 'inputCostPerToken' | 'outputCostPerToken'>): string {
  const parts: string[] = []
  if (isPriced(item.inputCostPerToken)) parts.push(`$${perMillion(item.inputCostPerToken)} in`)
  if (isPriced(item.outputCostPerToken)) parts.push(`$${perMillion(item.outputCostPerToken)} out`)
  if (parts.length === 0) return ''
  return `${parts.join(' · ')} /1M`
}

export function formatCachePricing(item: Pick<AdminModelItem, 'cacheReadCostPerToken' | 'cacheCreateCostPerToken'>): string {
  const parts: string[] = []
  if (isPriced(item.cacheReadCostPerToken)) parts.push(`读 $${perMillion(item.cacheReadCostPerToken)}`)
  if (isPriced(item.cacheCreateCostPerToken)) parts.push(`写 $${perMillion(item.cacheCreateCostPerToken)}`)
  if (parts.length === 0) return ''
  return `缓存 ${parts.join(' · ')} /1M`
}

function isPriced(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function perMillion(costPerToken: number): string {
  const scaled = costPerToken * 1_000_000
  // 保留最多 3 位小数去尾零：0.15→"0.15"，0.075→"0.075"，2→"2"。
  return String(Number(scaled.toFixed(3)))
}

// 提供商徽标单源：AddDialog 行徽标唯一出处。
export function modelProviderBadgeClass(provider: string): string {
  const p = (provider || '').toLowerCase()
  if (p.includes('workers')) return 'bg-amber-50 text-amber-700 border border-amber-200'
  if (p.includes('openai')) return 'bg-emerald-50 text-emerald-700 border border-emerald-200'
  if (p.includes('fal')) return 'bg-purple-50 text-purple-700 border border-purple-200'
  if (p.includes('anthropic')) return 'bg-orange-50 text-orange-700 border border-orange-200'
  if (p.includes('deepseek')) return 'bg-blue-50 text-blue-700 border border-blue-200'
  if (p.includes('google') || p.includes('vertex')) return 'bg-sky-50 text-sky-700 border border-sky-200'
  if (p.includes('groq')) return 'bg-rose-50 text-rose-700 border border-rose-200'
  return 'bg-admin-surface-muted text-admin-text-2 border border-admin-border-soft'
}
