import {
  detectControlAIGatewayModels,
  getControlAIGateway,
  getControlAIGatewayModelReference,
  listControlAIGatewayModelsObserved,
  listControlAIGatewayCatalog,
  listControlAIGatewayProviders,
  previewControlAIGatewayRoute,
  saveControlAIGatewayCatalog,
  testControlAIGatewayConnection,
  updateControlAIGateway,
} from '../../../../api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '../../../../api/contractKernel'
import {
  buildControlMutationHeaders,
  buildRoutineControlMutationHeaders,
  createControlError,
  requireOperatorReason,
  type ControlMutationOptions,
} from '../../shared/lib/controlMutation'
import { withControlCopy } from '../../shared/lib/controlCopy'
import type {
  AdminModelItem,
  ModelObservedSkuMargin,
  ModelObservedSnapshot,
  ModelReferenceSnapshot,
} from '../domain/catalogData'
import { MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY } from '../domain/catalogData'

// Admin AI gateway boundary (W2 transports). Reads with a Control
// equivalent ride the Control Plane (`GET
// /api/control/v1/platform/ai-gateway*`, unified contract envelope
// `{ data }` / `{ error: { code, message, request_id } }`) through the
// generated hooks: catalog list, providers list, connection get. The
// read-only extras moved to the Control Plane the same way: route preview,
// model detection, pricing-reference lookup, and the 7-day observation
// window. Writes moved to the Control Plane with the same envelope
// (nil-ack mutations resolve `{ data: {} }`): catalog save and connection
// update are routine config writes (backend ReasonOptional, no reason
// ceremony, audit records empty), while the server-side connection probe
// (provider/path required since #847 removed the mapping fallback) keeps
// its probe note. Callers keep importing these names, never generated code
// directly.
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

export type DetectedGatewayModel = {
  id: string
  name: string
  provider: string
  mode?: string
  description?: string
  description_enriched?: boolean
  context_or_resolution?: string
  context_limit?: number
  capabilities?: string[]
  modalities?: { input?: string[]; output?: string[] }
  supports_prompt_caching?: boolean
}

type GatewayModelDetection = {
  models: DetectedGatewayModel[]
  configured_providers: number
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
  save: (items: AdminModelItem[], options: GatewayMutationOptions) => Promise<void>
  previewRoute: (input: {
    modelId: string
    customPath?: string | undefined
    customParams?: string | undefined
  }) => Promise<GatewayRoutePreview>
  detect: (provider?: string) => Promise<GatewayModelDetection>
  listProviders: () => Promise<GatewayProviderOption[]>
  // models.dev 参照价（只读透传）：USD/MTok 单价、context limit、last_updated，
  // 外加能力 booleans、modalities、knowledge、release_date、family、status
  // enrichment（均为可选，缺失即未知）。description 永不透传。
  // source 恒为 models.dev。found=false 为上游降级/miss，HTTP 仍 200，调用方按快照展示。
  getReference: (input: {
    modelId?: string | undefined
    provider?: string | undefined
    model?: string | undefined
    refresh?: boolean | undefined
  }) => Promise<ModelReferenceSnapshot>
  // 7 天观测（只读透传）：成功率/头号失败/cache 命中率（无终结 attempts 时 null），
  // 外加 SKU 在售标记与 margin 行（实际成本恒 null，未编造）。
  // 缺席接口的 model 调用方按暂无数据渲染。
  listObserved: () => Promise<ModelObservedSnapshot[]>
}

export interface AdminGatewayConnection {
  get: () => Promise<GatewayConnectionSummary>
  update: (input: UpdateGatewayConfigRequest, options: GatewayMutationOptions) => Promise<void>
  testConnection: (
    input: {
      base_url: string
      token?: string | undefined
      provider: string
      model?: string | undefined
      path: string
    },
    options: GatewayMutationOptions,
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
      detect: detectGatewayModels,
      listProviders: listGatewayProviders,
      getReference: getModelReference,
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

function pickBoolean(raw: Record<string, unknown>, key: string): boolean | undefined {
  const value = raw[key]
  return typeof value === 'boolean' ? value : undefined
}

function pickStringArray(raw: Record<string, unknown>, key: string): string[] | undefined {
  const value = raw[key]
  if (!Array.isArray(value)) return undefined
  return value.filter((item): item is string => typeof item === 'string')
}

function pickModalities(
  raw: Record<string, unknown>,
): { input?: string[]; output?: string[] } | undefined {
  const value = raw.modalities
  if (!isRecord(value)) return undefined
  const modalities: { input?: string[]; output?: string[] } = {}
  const input = pickStringArray(value, 'input')
  const output = pickStringArray(value, 'output')
  if (input != null) modalities.input = input
  if (output != null) modalities.output = output
  return modalities
}

function toDetectedGatewayModel(raw: unknown): DetectedGatewayModel {
  if (!isRecord(raw)) throw createControlError('control_failed', '模型探测失败')
  const id = pickString(raw, 'id')
  const name = pickString(raw, 'name')
  const provider = pickString(raw, 'provider')
  if (!id || !name || !provider) throw createControlError('control_failed', '模型探测失败')
  const model: DetectedGatewayModel = { id, name, provider }
  const mode = pickString(raw, 'mode')
  if (mode != null) model.mode = mode
  if (typeof raw.description === 'string') model.description = raw.description
  const enriched = pickBoolean(raw, 'description_enriched')
  if (enriched != null) model.description_enriched = enriched
  const contextOrResolution = pickString(raw, 'context_or_resolution')
  if (contextOrResolution != null) model.context_or_resolution = contextOrResolution
  const contextLimit = pickNumber(raw, 'context_limit')
  if (contextLimit != null) model.context_limit = contextLimit
  const capabilities = pickStringArray(raw, 'capabilities')
  if (capabilities != null) model.capabilities = capabilities
  const modalities = pickModalities(raw)
  if (modalities != null) model.modalities = modalities
  const supportsPromptCaching = pickBoolean(raw, 'supports_prompt_caching')
  if (supportsPromptCaching != null) model.supports_prompt_caching = supportsPromptCaching
  return model
}

function toModelReferenceSnapshot(raw: Record<string, unknown>): ModelReferenceSnapshot {
  const modelId = pickString(raw, 'model_id')
  const source = pickString(raw, 'source')
  const note = typeof raw.note === 'string' ? raw.note : ''
  if (!modelId || !source) throw createControlError('control_failed', '参照价查询失败')
  const snapshot: ModelReferenceSnapshot = { modelId, source, note, found: raw.found === true }
  assignOptionalText(snapshot, raw, [
    ['provider', 'provider'],
    ['name', 'name'],
    ['knowledge', 'knowledge'],
    ['family', 'family'],
    ['status', 'status'],
    ['last_updated', 'lastUpdated'],
    ['release_date', 'releaseDate'],
    ['error_code', 'errorCode'],
    ['message', 'message'],
  ] as const)
  assignOptionalNumbers(snapshot, raw, [
    ['input_usd_per_mtok', 'inputUsdPerMtok'],
    ['output_usd_per_mtok', 'outputUsdPerMtok'],
    ['context_limit', 'contextLimit'],
  ] as const)
  for (const [snake, camel] of [
    ['reasoning', 'reasoning'],
    ['tool_call', 'toolCall'],
    ['temperature', 'temperature'],
    ['attachment', 'attachment'],
    ['structured_output', 'structuredOutput'],
    ['open_weights', 'openWeights'],
  ] as const) {
    const flag = pickBoolean(raw, snake)
    if (flag != null) {
      ;(snapshot as unknown as Record<string, unknown>)[camel] = flag
    }
  }
  const modalities = pickModalities(raw)
  if (modalities != null) snapshot.modalities = modalities
  return snapshot
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
  options: GatewayMutationOptions,
): Promise<void> {
  try {
    const body = await saveControlAIGatewayCatalog(
      { items: items.map(toGatewayCatalogItem) },
      { headers: buildRoutineControlMutationHeaders(options) },
    )
    // Nil-ack mutation: success is `{ data: {} }`.
    requireContractData<Record<string, unknown>>(body, '保存模型库失败')
  } catch (error) {
    throw withControlCopy(error, '保存模型库失败')
  }
}

async function previewModelRoute(input: {
  modelId: string
  customPath?: string | undefined
  customParams?: string | undefined
}): Promise<GatewayRoutePreview> {
  const payload: { model_id: string; custom_path?: string; custom_params?: string } = {
    model_id: input.modelId,
  }
  if (input.customPath != null) payload.custom_path = input.customPath
  if (input.customParams != null) payload.custom_params = input.customParams
  const body = await previewControlAIGatewayRoute(payload)
  return requireContractData<GatewayRoutePreview>(body, '路由透视失败')
}

async function detectGatewayModels(provider?: string): Promise<GatewayModelDetection> {
  const body = await detectControlAIGatewayModels(
    provider?.trim() ? { provider: provider.trim() } : undefined,
  )
  const result = requireContractData<{ models: unknown; configured_providers: unknown }>(
    body,
    '模型探测失败',
  )
  if (!Array.isArray(result.models) || typeof result.configured_providers !== 'number') {
    throw createControlError('control_failed', '模型探测失败')
  }
  return {
    models: result.models.map(toDetectedGatewayModel),
    configured_providers: result.configured_providers,
  }
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

async function getModelReference(input: {
  modelId?: string | undefined
  provider?: string | undefined
  model?: string | undefined
  refresh?: boolean | undefined
}): Promise<ModelReferenceSnapshot> {
  const params: { model_id?: string; provider?: string; model?: string; refresh?: string } = {}
  if (input.modelId?.trim()) params.model_id = input.modelId.trim()
  if (input.provider?.trim()) params.provider = input.provider.trim()
  if (input.model?.trim()) params.model = input.model.trim()
  if (input.refresh != null) params.refresh = String(input.refresh)
  const body = await getControlAIGatewayModelReference(params)
  return toModelReferenceSnapshot(
    requireContractData<Record<string, unknown>>(body, '参照价查询失败'),
  )
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
  options: GatewayMutationOptions,
): Promise<void> {
  try {
    const body = await updateControlAIGateway(
      { ...input },
      { headers: buildRoutineControlMutationHeaders(options) },
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
    provider: string
    model?: string | undefined
    path: string
  },
  options: GatewayMutationOptions,
): Promise<{ success: boolean; latencyMs?: number; message?: string }> {
  // Server-side probe (browsers cannot read cross-origin status); the token
  // is used for this probe only and never persisted. Mutation-guarded, so
  // it needs an operator reason like any other control write.
  const operatorReason = requireOperatorReason(options.operatorReason)
  const payload: {
    base_url: string
    token?: string
    provider: string
    model?: string
    path: string
    reason: string
  } = {
    base_url: input.base_url,
    provider: input.provider,
    path: input.path,
    reason: operatorReason,
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

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
