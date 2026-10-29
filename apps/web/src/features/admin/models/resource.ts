import {
  getControlAIGateway,
  listControlAIGatewayModelsObserved,
  listControlAIGatewayCatalog,
  listControlAIGatewayProviders,
  previewControlAIGatewayRoute,
  saveControlAIGatewayCatalog,
  testControlAIGatewayConnection,
  updateControlAIGateway,
} from '@/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '@/api/contractKernel'
import {
  buildControlMutationHeaders,
  createControlError,
  type ControlMutationOptions,
} from '../shared'
import { withControlCopy } from '../shared'
import { isRecord } from '@/core/utils/guards'
import type {
  AdminModelItem,
  ModelObservedSkuMargin,
  ModelObservedSnapshot,
} from './core/catalogData'
import { MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY } from './core/catalogData'

type GatewayMutationOptions = ControlMutationOptions
type GatewayConfigSummary = {
  base_url?: string
  account_id?: string
  gateway_id?: string
  token_configured: boolean
  is_first_party?: boolean
  updated_at?: string | null
}

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

type GatewayRoutePreview = {
  provider_slug: string
  is_first_party: boolean
  target_path: string
  path_source: string
  path_source_label: string
}

type UpdateGatewayConfigRequest = {
  base_url?: string
  token?: string
  account_id?: string
  gateway_id?: string
}

export type GatewayConnectionSummary = GatewayConfigSummary

export interface AdminGatewayCatalog {
  list: () => Promise<AdminModelItem[]>
  save: (items: AdminModelItem[], options?: GatewayMutationOptions) => Promise<void>
  previewRoute: (input: {
    modelId: string
    provider?: string | undefined
    customPath?: string | undefined
    customParams?: string | undefined
  }) => Promise<GatewayRoutePreview>
  listProviders: () => Promise<GatewayProviderOption[]>
  // 7 天观测（只读透传）：成功率/头号失败/cache 命中率（无终结 attempts 时 null），
  // 外加 SKU 在售标记与 margin 行（实际成本恒 null，未编造）。
  // 缺席接口的 model 调用方按暂无数据渲染。
  listObserved: () => Promise<ModelObservedSnapshot[]>
}

export interface AdminGatewayConnection {
  get: () => Promise<GatewayConnectionSummary>
  update: (input: UpdateGatewayConfigRequest, options?: GatewayMutationOptions) => Promise<void>
  testConnection: (
    input: {
      base_url: string
      token?: string | undefined
      provider?: string | undefined
      model?: string | undefined
      path?: string | undefined
    },
    options?: GatewayMutationOptions,
  ) => Promise<{ success: boolean; latencyMs?: number; message?: string }>
}

export interface AdminGatewayResource {
  catalog: AdminGatewayCatalog
  connection: AdminGatewayConnection
}

// AI 网关设置面唯一出口：目录、连接两组共享 adapter 与归一，
// 调用方按组取用（catalog / connection），不得直调 generated client。
// per-service 模型管理由 #849 的 pool editor 接管，不在此处另起映射面。
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
      testConnection: testGatewayConnection,
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

function pickString(raw: Record<string, unknown>, key: string): string | undefined {
  const value = raw[key]
  return typeof value === 'string' && value ? value : undefined
}

function pickNumber(raw: Record<string, unknown>, key: string): number | undefined {
  const value = raw[key]
  return typeof value === 'number' ? value : undefined
}

function assignOptionalText<T extends object>(
  target: T,
  raw: Record<string, unknown>,
  entries: ReadonlyArray<readonly [string, keyof T]>,
): void {
  for (const [snake, camel] of entries) {
    const value = pickString(raw, snake)
    if (value != null) target[camel] = value as T[keyof T]
  }
}

function assignOptionalNumbers<T extends object>(
  target: T,
  raw: Record<string, unknown>,
  entries: ReadonlyArray<readonly [string, keyof T]>,
): void {
  for (const [snake, camel] of entries) {
    const value = pickNumber(raw, snake)
    if (value != null) target[camel] = value as T[keyof T]
  }
}

function toModelObservedSnapshot(raw: unknown): ModelObservedSnapshot {
  if (!isRecord(raw)) throw createControlError('control_failed', '观测数据加载失败')
  const model = pickString(raw, 'model')
  const provider = pickString(raw, 'provider')
  if (!model || !provider) throw createControlError('control_failed', '观测数据加载失败')
  const snapshot: ModelObservedSnapshot = {
    model,
    provider,
    windowDays: pickNumber(raw, 'window_days') ?? 0,
    windowSince: pickString(raw, 'window_since') ?? '',
    attemptsTotal: pickNumber(raw, 'attempts_total') ?? 0,
    completed: pickNumber(raw, 'completed') ?? 0,
    failed: pickNumber(raw, 'failed') ?? 0,
    cacheHits: pickNumber(raw, 'cache_hits') ?? 0,
    hasObservations: raw.has_observations === true,
    hasSkuMargin: raw.has_sku_margin === true,
    skuMargins: Array.isArray(raw.sku_margins) ? raw.sku_margins.map(toObservedSkuMargin) : [],
  }
  assignOptionalText(snapshot, raw, [
    ['top_failure_stage', 'topFailureStage'],
    ['top_failure_category', 'topFailureCategory'],
  ] as const)
  assignOptionalNumbers(snapshot, raw, [['top_failure_count', 'topFailureCount']] as const)
  assignNullableNumbers(snapshot, raw, [
    ['success_rate', 'successRate'],
    ['cache_hit_rate', 'cacheHitRate'],
  ] as const)
  return snapshot
}

function assignNullableNumbers<T extends object>(
  target: T,
  raw: Record<string, unknown>,
  entries: ReadonlyArray<readonly [string, keyof T]>,
): void {
  for (const [snake, camel] of entries) {
    const value = raw[snake]
    if (typeof value === 'number' || value === null) {
      target[camel] = value as T[keyof T]
    }
  }
}

function toObservedSkuMargin(raw: unknown): ModelObservedSkuMargin {
  if (!isRecord(raw)) throw createControlError('control_failed', '观测数据加载失败')
  const sku = pickString(raw, 'sku')
  const service = pickString(raw, 'service')
  if (!sku || !service) throw createControlError('control_failed', '观测数据加载失败')
  const margin: ModelObservedSkuMargin = {
    sku,
    displayName: pickString(raw, 'display_name') ?? '',
    service,
    onSale: raw.on_sale === true,
    unitCreditsByVariant: pickNumberRecord(raw, 'unit_credits_by_variant'),
    hasActualCost: raw.has_actual_cost === true,
    costSource: pickString(raw, 'cost_source') ?? '',
    costNote: pickString(raw, 'cost_note') ?? '',
    marginStatus: pickString(raw, 'margin_status') ?? '',
  }
  assignNullableNumbers(margin, raw, [
    ['actual_cost_credits', 'actualCostCredits'],
    ['margin_credits', 'marginCredits'],
  ] as const)
  return margin
}

function pickNumberRecord(raw: Record<string, unknown>, key: string): Record<string, number> {
  const result: Record<string, number> = {}
  const value = raw[key]
  if (!isRecord(value)) return result
  for (const [entryKey, entryValue] of Object.entries(value)) {
    if (typeof entryValue === 'number') result[entryKey] = entryValue
  }
  return result
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
  options: GatewayMutationOptions = {},
): Promise<void> {
  try {
    const body = await saveControlAIGatewayCatalog(
      { items: items.map(toGatewayCatalogItem) },
      { headers: buildControlMutationHeaders(options) },
    )
    // Nil-ack mutation: success is `{ data: {} }`.
    requireContractData<Record<string, unknown>>(body, '保存模型库失败')
  } catch (error) {
    throw withControlCopy(error, '保存模型库失败')
  }
}

async function previewModelRoute(input: {
  modelId: string
  provider?: string | undefined
  customPath?: string | undefined
  customParams?: string | undefined
}): Promise<GatewayRoutePreview> {
  // 后端透视仍消费组合 model_id：裸名在此拼接 provider（仅传输，不存储）。
  const bare = input.modelId.trim()
  const provider = (input.provider ?? '').trim()
  const combinedId = provider ? `${provider}/${bare}` : bare
  const payload: { model_id: string; custom_path?: string; custom_params?: string } = {
    model_id: combinedId,
  }
  if (input.customPath != null) payload.custom_path = input.customPath
  if (input.customParams != null) payload.custom_params = input.customParams
  const body = await previewControlAIGatewayRoute(payload)
  return requireContractData<GatewayRoutePreview>(body, '路由透视失败')
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
    const data = requireContractData<GatewayConfigSummary>(body, 'AI 网关配置不存在')
    if (!data) throw new Error('AI 网关配置不存在')
    return data
  } catch (error) {
    throw withControlCopy(error, 'AI 网关配置不存在')
  }
}

async function updateGatewayConnection(
  input: UpdateGatewayConfigRequest,
  options: GatewayMutationOptions = {},
): Promise<void> {
  try {
    const body = await updateControlAIGateway(
      { ...input },
      { headers: buildControlMutationHeaders(options) },
    )
    // Nil-ack mutation: success is `{ data: {} }`.
    requireContractData<Record<string, unknown>>(body, '保存 AI 网关配置失败')
  } catch (error) {
    throw withControlCopy(error, '保存 AI 网关配置失败')
  }
}

async function testGatewayConnection(
  input: {
    base_url: string
    token?: string | undefined
    provider?: string | undefined
    model?: string | undefined
    path?: string | undefined
  },
  options: GatewayMutationOptions = {},
): Promise<{ success: boolean; latencyMs?: number; message?: string }> {
  const payload: {
    base_url: string
    token?: string
    provider?: string
    model?: string
    path?: string
  } = {
    base_url: input.base_url,
    provider: input.provider ?? '',
    path: input.path ?? '',
  }
  if (input.token != null) payload.token = input.token
  if (input.model != null) payload.model = input.model
  let body: unknown
  try {
    body = await testControlAIGatewayConnection(payload, {
      headers: buildControlMutationHeaders(options),
    })
  } catch (error) {
    throw withControlCopy(error, '网关连接测试失败')
  }
  const probe = requireContractData<{
    reachable: boolean
    status_code: number
    latency_ms?: number
    message?: string
  }>(body, '网关连接测试失败')
  const result: { success: boolean; latencyMs?: number; message?: string } = {
    success: probe.reachable === true,
  }
  if (typeof probe.latency_ms === 'number') result.latencyMs = probe.latency_ms
  if (typeof probe.message === 'string' && probe.message) result.message = probe.message
  return result
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.
