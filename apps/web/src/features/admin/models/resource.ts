import {
  getControlAIGateway,
  listControlAIGatewayModelsObserved,
  listControlAIGatewayCatalog,
  listControlAIGatewayProviders,
  previewControlAIGatewayRoute,
  saveControlAIGatewayCatalog,
  updateControlAIGateway,
} from '@/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '@/api/http/contractKernel'
import {
  buildControlMutationHeaders,
  createControlError,
  withControlCopy,
  type ControlMutationOptions,
} from '../shared'
import { isRecord } from '@/core/utils/guards'
import type {
  AdminModelItem,
  ModelObservedSkuMargin,
  ModelObservedSnapshot,
} from './core/catalogData'
import { MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY } from './core/catalogData'

/** @externalContract AI Gateway configuration summary wire response. */
export type GatewayConnectionSummary = {
  base_url?: string
  account_id?: string
  gateway_id?: string
  token_configured: boolean
  is_first_party?: boolean
  updated_at?: string | null
}

/** @externalContract AI Gateway model catalog item wire payload. */
type GatewayModelCatalogItem = {
  id: string
  name: string
  provider: string
  mode?: string
  description: string
  context_or_resolution: string
  capabilities: string[]
  hosting: string
  terms_url: string
  docs_url: string
  custom_path?: string
  custom_params?: string
  description_enriched?: boolean
  supports_prompt_caching?: boolean
}

export type GatewayProviderOption = {
  slug: string
  display_name: string
  id_prefix: string
}

type WorkerRoutePreview = {
  provider: string
  model: string
  // eslint-disable-next-line governance/boolean-prefix -- Wire contract field (Worker dry-run preview).
  valid: boolean
  // eslint-disable-next-line governance/boolean-prefix -- Wire contract field (Worker dry-run preview).
  reachable: boolean
  status_code: number
  latency_ms: number
  message: string
}

type UpdateGatewayConfigRequest = {
  base_url?: string
  token?: string
  account_id?: string
  gateway_id?: string
}

export interface AdminGatewayCatalog {
  list: () => Promise<AdminModelItem[]>
  save: (items: AdminModelItem[], options?: ControlMutationOptions) => Promise<void>
  previewRoute: (input: {
    modelId: string
    provider?: string | undefined
    customParams?: string | undefined
  }) => Promise<WorkerRoutePreview>
  listProviders: () => Promise<GatewayProviderOption[]>
  listObserved: () => Promise<ModelObservedSnapshot[]>
}

export interface AdminGatewayConnection {
  get: () => Promise<GatewayConnectionSummary>
  update: (input: UpdateGatewayConfigRequest, options?: ControlMutationOptions) => Promise<void>
}

export interface AdminGatewayResource {
  catalog: AdminGatewayCatalog
  connection: AdminGatewayConnection
}

// AI 网关设置面唯一出口：目录、连接两组共享 adapter 与归一，
// 调用方按组取用（catalog / connection），不得直调 generated client。
export function createAdminGatewayResource(): AdminGatewayResource {
  return {
    catalog: {
      list: listModelCatalog,
      save: saveModelCatalog,
      previewRoute: previewModelRoute,
      listProviders: listGatewayProviders,
      listObserved: listModelsObserved,
    },
    connection: {
      get: getGatewayConnection,
      update: updateGatewayConnection,
    },
  }
}

function toAdminModelItem(item: GatewayModelCatalogItem): AdminModelItem {
  return {
    id: item.id,
    name: item.name,
    provider: item.provider,
    mode: item.mode,
    description: item.description,
    contextOrResolution: item.context_or_resolution,
    capabilities: [...item.capabilities],
    hosting:
      item.hosting === MODEL_HOSTING_THIRD_PARTY
        ? MODEL_HOSTING_THIRD_PARTY
        : MODEL_HOSTING_CLOUDFLARE,
    termsUrl: item.terms_url,
    docsUrl: item.docs_url,
    customPath: item.custom_path || '',
    customParams: item.custom_params || '',
    descriptionEnriched: item.description_enriched,
    supportsPromptCaching: item.supports_prompt_caching,
  }
}

function toGatewayCatalogItem(item: AdminModelItem): GatewayModelCatalogItem {
  return {
    id: item.id,
    name: item.name,
    provider: item.provider,
    ...(item.mode != null ? { mode: item.mode } : {}),
    description: item.description,
    context_or_resolution: item.contextOrResolution,
    capabilities: [...item.capabilities],
    hosting: item.hosting,
    terms_url: item.termsUrl,
    docs_url: item.docsUrl,
    ...(item.customPath ? { custom_path: item.customPath } : {}),
    ...(item.customParams ? { custom_params: item.customParams } : {}),
    ...(item.descriptionEnriched != null ? { description_enriched: item.descriptionEnriched } : {}),
    ...(item.supportsPromptCaching != null
      ? { supports_prompt_caching: item.supportsPromptCaching }
      : {}),
  }
}

const asString = (val: unknown) => (typeof val === 'string' && val ? val : undefined)
const asNumber = (val: unknown) => (typeof val === 'number' ? val : undefined)
const asNullableNumber = (val: unknown) =>
  typeof val === 'number' || val === null ? (val as number | null) : undefined

function toObservedSkuMargin(raw: unknown): ModelObservedSkuMargin {
  if (!isRecord(raw)) throw createControlError('control_failed', '观测数据加载失败')
  const sku = asString(raw.sku)
  const service = asString(raw.service)
  if (!sku || !service) throw createControlError('control_failed', '观测数据加载失败')

  const unitCreditsByVariant: Record<string, number> = {}
  if (isRecord(raw.unit_credits_by_variant)) {
    for (const [key, value] of Object.entries(raw.unit_credits_by_variant)) {
      if (typeof value === 'number') unitCreditsByVariant[key] = value
    }
  }

  const margin: ModelObservedSkuMargin = {
    sku,
    displayName: asString(raw.display_name) ?? '',
    service,
    onSale: raw.on_sale === true,
    unitCreditsByVariant,
    hasActualCost: raw.has_actual_cost === true,
    costSource: asString(raw.cost_source) ?? '',
    costNote: asString(raw.cost_note) ?? '',
    marginStatus: asString(raw.margin_status) ?? '',
  }
  const actualCostCredits = asNullableNumber(raw.actual_cost_credits)
  if (actualCostCredits !== undefined) margin.actualCostCredits = actualCostCredits
  const marginCredits = asNullableNumber(raw.margin_credits)
  if (marginCredits !== undefined) margin.marginCredits = marginCredits
  return margin
}

function assignObservedExtras(snapshot: ModelObservedSnapshot, raw: Record<string, unknown>): void {
  const topFailureStage = asString(raw.top_failure_stage)
  if (topFailureStage != null) snapshot.topFailureStage = topFailureStage
  const topFailureCategory = asString(raw.top_failure_category)
  if (topFailureCategory != null) snapshot.topFailureCategory = topFailureCategory
  const topFailureCount = asNumber(raw.top_failure_count)
  if (topFailureCount != null) snapshot.topFailureCount = topFailureCount
  const successRate = asNullableNumber(raw.success_rate)
  if (successRate !== undefined) snapshot.successRate = successRate
  const cacheHitRate = asNullableNumber(raw.cache_hit_rate)
  if (cacheHitRate !== undefined) snapshot.cacheHitRate = cacheHitRate
}

function toModelObservedSnapshot(raw: unknown): ModelObservedSnapshot {
  if (!isRecord(raw)) throw createControlError('control_failed', '观测数据加载失败')
  const model = asString(raw.model)
  const provider = asString(raw.provider)
  if (!model || !provider) throw createControlError('control_failed', '观测数据加载失败')

  const snapshot: ModelObservedSnapshot = {
    model,
    provider,
    windowDays: asNumber(raw.window_days) ?? 0,
    windowSince: asString(raw.window_since) ?? '',
    attemptsTotal: asNumber(raw.attempts_total) ?? 0,
    completed: asNumber(raw.completed) ?? 0,
    failed: asNumber(raw.failed) ?? 0,
    cacheHits: asNumber(raw.cache_hits) ?? 0,
    hasObservations: raw.has_observations === true,
    hasSkuMargin: raw.has_sku_margin === true,
    skuMargins: Array.isArray(raw.sku_margins) ? raw.sku_margins.map(toObservedSkuMargin) : [],
  }
  assignObservedExtras(snapshot, raw)
  return snapshot
}

async function listModelCatalog(): Promise<AdminModelItem[]> {
  try {
    const body = await listControlAIGatewayCatalog()
    const data = requireContractData<GatewayModelCatalogItem[]>(body, '加载模型库失败')
    if (!Array.isArray(data)) throw new Error('加载模型库失败')
    return data.map(toAdminModelItem)
  } catch (error) {
    throw withControlCopy(error, '加载模型库失败')
  }
}

async function saveModelCatalog(
  items: AdminModelItem[],
  options: ControlMutationOptions = {},
): Promise<void> {
  try {
    const body = await saveControlAIGatewayCatalog(
      { items: items.map(toGatewayCatalogItem) },
      { headers: buildControlMutationHeaders(options) },
    )
    requireContractData<Record<string, unknown>>(body, '保存模型库失败')
  } catch (error) {
    throw withControlCopy(error, '保存模型库失败')
  }
}

async function previewModelRoute(input: {
  modelId: string
  provider?: string | undefined
  customParams?: string | undefined
}): Promise<WorkerRoutePreview> {
  const bare = input.modelId.trim()
  const provider = (input.provider ?? '').trim()
  const body = await previewControlAIGatewayRoute({
    model_id: provider ? `${provider}/${bare}` : bare,
    ...(input.customParams != null ? { custom_params: input.customParams } : {}),
  })
  return requireContractData<WorkerRoutePreview>(body, '路由透视失败')
}

async function listGatewayProviders(): Promise<GatewayProviderOption[]> {
  try {
    const body = await listControlAIGatewayProviders()
    const data = requireContractData<GatewayProviderOption[]>(body, '加载供应商列表失败')
    if (!Array.isArray(data)) throw new Error('加载供应商列表失败')
    return data
  } catch (error) {
    throw withControlCopy(error, '加载供应商列表失败')
  }
}

async function listModelsObserved(): Promise<ModelObservedSnapshot[]> {
  const body = await listControlAIGatewayModelsObserved()
  const rows = requireContractData<unknown>(body, '观测数据加载失败')
  if (!Array.isArray(rows)) throw createControlError('control_failed', '观测数据加载失败')
  return rows.map(toModelObservedSnapshot)
}

async function getGatewayConnection(): Promise<GatewayConnectionSummary> {
  try {
    const body = await getControlAIGateway()
    const data = requireContractData<GatewayConnectionSummary>(body, 'AI 网关配置不存在')
    if (!data) throw new Error('AI 网关配置不存在')
    return data
  } catch (error) {
    throw withControlCopy(error, 'AI 网关配置不存在')
  }
}

async function updateGatewayConnection(
  input: UpdateGatewayConfigRequest,
  options: ControlMutationOptions = {},
): Promise<void> {
  try {
    const body = await updateControlAIGateway(input, {
      headers: buildControlMutationHeaders(options),
    })
    requireContractData<Record<string, unknown>>(body, '保存 AI 网关配置失败')
  } catch (error) {
    throw withControlCopy(error, '保存 AI 网关配置失败')
  }
}
