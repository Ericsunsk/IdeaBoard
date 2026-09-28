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
  // prompt caching 能力位（展示用，不进计费）。
  supportsPromptCaching?: boolean | undefined
}

// models.dev 参照价快照（domain 侧 camelCase）：经 adminGatewayResource 由后端
// GET /admin/settings/ai-gateway/models-reference 透传，前端永不直连 models.dev。
// 结算唯一依据 billing SKU UnitCredits，此处仅参照展示。
// Enrichment 字段（能力 booleans、modalities、knowledge、releaseDate、family、
// status）均为上游原名直透的可选值：undefined/空即未知，调用方一律暂无数据。
// description 永不透传（网关侧运营文案优先）。
export interface ReferenceModalities {
  input?: string[] | undefined
  output?: string[] | undefined
}

export interface ModelReferenceSnapshot {
  modelId: string
  provider?: string | undefined
  name?: string | undefined
  inputUsdPerMtok?: number | undefined
  outputUsdPerMtok?: number | undefined
  contextLimit?: number | undefined
  lastUpdated?: string | undefined
  reasoning?: boolean | undefined
  toolCall?: boolean | undefined
  temperature?: boolean | undefined
  attachment?: boolean | undefined
  structuredOutput?: boolean | undefined
  openWeights?: boolean | undefined
  modalities?: ReferenceModalities | undefined
  knowledge?: string | undefined
  releaseDate?: string | undefined
  family?: string | undefined
  status?: string | undefined
  source: string
  note: string
  found: boolean
  errorCode?: string | undefined
  message?: string | undefined
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

// 参照价展示单源：USD/MTok 直展（后端已按百万 token 计价，无需二次换算），
// 缺失即未知（返回 '' 不展示）。
// Why 按 1M 计：models.dev 上游即以 USD/MTok 计价。
export function formatReferencePricing(
  item: Pick<ModelReferenceSnapshot, 'inputUsdPerMtok' | 'outputUsdPerMtok'>,
): string {
  const parts: string[] = []
  if (isPriced(item.inputUsdPerMtok)) parts.push(`$${trimNumber(item.inputUsdPerMtok)} in`)
  if (isPriced(item.outputUsdPerMtok)) parts.push(`$${trimNumber(item.outputUsdPerMtok)} out`)
  if (parts.length === 0) return ''
  return `${parts.join(' · ')} /1M`
}

// 上下文展示单源：128000 → "128K"，不足千位原样展示；缺失即未知（返回 ''）。
export function formatReferenceContext(contextLimit: number | undefined): string {
  if (typeof contextLimit !== 'number' || !Number.isFinite(contextLimit) || contextLimit <= 0)
    return ''
  if (contextLimit >= 1000 && contextLimit % 1000 === 0) return `${contextLimit / 1000}K`
  return String(contextLimit)
}

// fal 缺省判定单源：models.dev 无 fal provider（api.json?type=all 实测 223 providers
// 无 fal），fal 模型参照恒为 miss；命中即短路展示优雅缺省，不发参照请求。
// Why 词边界：'Fal AI'/'fal_ai' 命中，'Falcon' 类不误伤。
export function isFalReferenceModel(input: {
  provider?: string | undefined
  modelId?: string | undefined
}): boolean {
  const provider = (input.provider || '').toLowerCase()
  if (/(^|[^a-z])fal([^a-z]|$)/.test(provider)) return true
  const id = (input.modelId || '').toLowerCase().trim()
  return id.startsWith('fal_ai/') || id.startsWith('fal/')
}

// 新鲜度展示单源：last_updated → '3 天前'；非法/缺失/未来返回 ''（调用方显示未知）。
// Why 注入 nowMs：单测定死时间，主流程默认 Date.now。
export function formatReferenceFreshness(
  lastUpdated: string | undefined,
  nowMs: number = Date.now(),
): string {
  if (!lastUpdated) return ''
  const time = Date.parse(lastUpdated)
  if (!Number.isFinite(time)) return ''
  const diffMs = nowMs - time
  if (diffMs < 0) return ''
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour
  if (diffMs < minute) return '刚刚'
  if (diffMs < hour) return `${Math.floor(diffMs / minute)} 分钟前`
  if (diffMs < day) return `${Math.floor(diffMs / hour)} 小时前`
  if (diffMs < 30 * day) return `${Math.floor(diffMs / day)} 天前`
  if (diffMs < 365 * day) return `${Math.floor(diffMs / (30 * day))} 个月前`
  return `${Math.floor(diffMs / (365 * day))} 年前`
}

// 能力 enrichment 展示单源：6 个上游 boolean 原样透传，undefined 即未知。
// Why 固定顺序：推理/工具/温度/附件/结构化/权重，面板与单测共享同一顺序。
export interface ReferenceCapability {
  key: 'reasoning' | 'toolCall' | 'temperature' | 'attachment' | 'structuredOutput' | 'openWeights'
  label: string
  supported?: boolean | undefined
}

export function referenceCapabilities(
  item: Pick<
    ModelReferenceSnapshot,
    'reasoning' | 'toolCall' | 'temperature' | 'attachment' | 'structuredOutput' | 'openWeights'
  >,
): ReferenceCapability[] {
  return [
    { key: 'reasoning', label: '推理', supported: item.reasoning },
    { key: 'toolCall', label: '工具调用', supported: item.toolCall },
    { key: 'temperature', label: '温度参数', supported: item.temperature },
    { key: 'attachment', label: '附件输入', supported: item.attachment },
    { key: 'structuredOutput', label: '结构化输出', supported: item.structuredOutput },
    { key: 'openWeights', label: '开放权重', supported: item.openWeights },
  ]
}

// 任一能力已知即有数据：全 undefined 时面板一律暂无数据。
export function hasReferenceCapabilityData(
  item: Pick<
    ModelReferenceSnapshot,
    'reasoning' | 'toolCall' | 'temperature' | 'attachment' | 'structuredOutput' | 'openWeights'
  >,
): boolean {
  return referenceCapabilities(item).some((capability) => capability.supported !== undefined)
}

// 模态展示单源：input → output；任一缺失只展已知侧；全缺失返回 ''（未知）。
export function formatReferenceModalities(modalities: ReferenceModalities | undefined): string {
  const input = (modalities?.input ?? []).map((entry) => entry.trim()).filter(Boolean)
  const output = (modalities?.output ?? []).map((entry) => entry.trim()).filter(Boolean)
  if (input.length === 0 && output.length === 0) return ''
  if (input.length === 0) return output.join(' / ')
  if (output.length === 0) return input.join(' / ')
  return `${input.join(' / ')} → ${output.join(' / ')}`
}

// 发布/知识截止展示单源：releaseDate 原样，knowledge 前缀“截止”；全缺失返回 ''。
export function formatReferenceReleaseKnowledge(
  releaseDate: string | undefined,
  knowledge: string | undefined,
): string {
  const release = (releaseDate ?? '').trim()
  const cutoff = (knowledge ?? '').trim()
  if (release && cutoff) return `${release} / 截止 ${cutoff}`
  if (release) return release
  if (cutoff) return `截止 ${cutoff}`
  return ''
}

// 上游状态判定单源：大小写不敏感；缺失即 unknown（调用方无预警，不误报）。
export type ReferenceStatusKind = 'deprecated' | 'beta' | 'other' | 'unknown'

export function referenceStatusKind(status: string | undefined): ReferenceStatusKind {
  const normalized = (status ?? '').trim().toLowerCase()
  if (!normalized) return 'unknown'
  if (normalized === 'deprecated') return 'deprecated'
  if (normalized === 'beta') return 'beta'
  return 'other'
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

function isPriced(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function trimNumber(value: number): string {
  // 保留最多 3 位小数去尾零：0.15→"0.15"，0.075→"0.075"，2→"2"。
  return String(Number(value.toFixed(3)))
}

// 提供商徽标单源：AddDialog 行徽标唯一出处。
export function modelProviderBadgeClass(provider: string): string {
  const p = (provider || '').toLowerCase()
  if (p.includes('workers')) return 'bg-amber-50 text-amber-700 border border-amber-200'
  if (p.includes('openai')) return 'bg-emerald-50 text-emerald-700 border border-emerald-200'
  if (p.includes('fal')) return 'bg-purple-50 text-purple-700 border border-purple-200'
  if (p.includes('anthropic')) return 'bg-orange-50 text-orange-700 border border-orange-200'
  if (p.includes('deepseek')) return 'bg-blue-50 text-blue-700 border border-blue-200'
  if (p.includes('google') || p.includes('vertex'))
    return 'bg-sky-50 text-sky-700 border border-sky-200'
  if (p.includes('groq')) return 'bg-rose-50 text-rose-700 border border-rose-200'
  return 'bg-admin-surface-muted text-admin-text-2 border border-admin-border-soft'
}
