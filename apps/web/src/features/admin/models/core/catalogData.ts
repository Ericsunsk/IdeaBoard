export interface AdminModelItem {
  id: string
  name: string
  provider: string
  // mode 原样透传（chat / image_generation …）：展示与动态过滤用，
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
  // 富化位：true 为可覆盖的导入文本，false/undefined 为运营手写（永不覆盖）。
  descriptionEnriched?: boolean | undefined
  // prompt caching 能力位（展示用，不进计费）。
  supportsPromptCaching?: boolean | undefined
}

// 托管文案单源：逻辑层一律引用常量，模板展示绑定除外。
export const MODEL_HOSTING_CLOUDFLARE = 'Cloudflare 托管'
export const MODEL_HOSTING_THIRD_PARTY = '第三方'
export const MODEL_HOSTING_OPTIONS = [MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY] as const

// 7 天观测快照（domain 侧 camelCase）：经 adminGatewayResource 由后端
// GET /admin/settings/ai-gateway/models-observed 透传，只读展示。
// rates 无终结 attempts 时为 null（调用方一律暂无数据）；margin/cost 恒 null
//（网关侧暂无成本来源，后端 cost_source=unavailable/margin_status=unknown_no_cost），
// 前端绝不编造数字，只展示暂无数据。
export interface ModelObservedSkuMargin {
  sku: string
  displayName: string
  service: string
  onSale: boolean
  unitCreditsByVariant: Record<string, number>
  actualCostCredits?: number | null | undefined
  hasActualCost: boolean
  costSource: string
  costNote: string
  marginCredits?: number | null | undefined
  marginStatus: string
}

export interface ModelObservedSnapshot {
  model: string
  provider: string
  windowDays: number
  windowSince: string
  attemptsTotal: number
  completed: number
  failed: number
  cacheHits: number
  successRate?: number | null | undefined
  cacheHitRate?: number | null | undefined
  topFailureStage?: string | undefined
  topFailureCategory?: string | undefined
  topFailureCount?: number | undefined
  hasObservations: boolean
  hasSkuMargin: boolean
  skuMargins: ModelObservedSkuMargin[]
}

export function isCloudflareHosted(provider: string): boolean {
  const normalized = (provider || '').toLowerCase()
  return normalized.includes('workers') || normalized.includes('cloudflare')
}

// Catalog identity is the provider plus its native model name, including nested slashes.
// Display casing is preserved; comparison keys fold case just like the backend validator.
export function displayModelId(item: Pick<AdminModelItem, 'id' | 'provider'>): string {
  return `${item.provider.trim()}/${item.id.trim()}`
}

export function modelIdentityKey(item: Pick<AdminModelItem, 'id' | 'provider'>): string {
  return displayModelId(item).toLowerCase()
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

// 观测成功率/cache 命中率展示单源：0..1 → 百分比（最多 3 位小数去尾零，与 trimNumber 同语义）；
// null/undefined/非法即未知（返回 ''，调用方显示暂无数据）。
// Why 去尾零：92% 不展 92.0%。
export function formatObservedRate(rate: number | null | undefined): string {
  if (typeof rate !== 'number' || !Number.isFinite(rate) || rate < 0 || rate > 1) return ''
  return `${trimNumber(rate * 100)}%`
}

// 头号失败分类展示单源：stage/category 任一缺失只展已知侧；
// 无失败（count 缺失/0 且 stage/category 均空）返回 ''（未知）。
export function formatObservedFailure(
  item: Pick<ModelObservedSnapshot, 'topFailureStage' | 'topFailureCategory' | 'topFailureCount'>,
): string {
  const stage = (item.topFailureStage ?? '').trim()
  const category = (item.topFailureCategory ?? '').trim()
  const count = item.topFailureCount
  const hasCount = typeof count === 'number' && Number.isFinite(count) && count > 0
  const parts = [stage, category].filter(Boolean).join(' / ')
  if (!parts) return ''
  return hasCount ? `${parts} · ${count} 次` : parts
}

// 在售 SKU 筛选单源：在售（active 计价卡）优先展示，非在售仍保留由调用方置灰。
export function observedOnSaleMargins(snapshot: ModelObservedSnapshot): ModelObservedSkuMargin[] {
  return snapshot.skuMargins.filter((margin) => margin.onSale)
}

// 毛利展示单源：恒 null（无成本源）即未知（返回 ''，调用方显示暂无数据，绝不编造）。
// cost 侧有来源后负值由调用方标红，此处只做数字直展。
export function formatObservedMargin(marginCredits: number | null | undefined): string {
  if (typeof marginCredits !== 'number' || !Number.isFinite(marginCredits)) return ''
  return `${marginCredits > 0 ? '+' : ''}${marginCredits}`
}

function trimNumber(value: number): string {
  // 保留最多 3 位小数去尾零：0.15→"0.15"，0.075→"0.075"，2→"2"。
  return String(Number(value.toFixed(3)))
}
