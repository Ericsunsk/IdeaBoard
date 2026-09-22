import {
  detectAdminGatewayModels,
  getAdminAIGatewayConfig,
  getAdminGatewayModelReference,
  listAdminGatewayModelCatalog,
  listAdminGatewayModelMappings,
  listAdminGatewayModelsObserved,
  listAdminGatewayProviders,
  previewAdminGatewayRoute,
  saveAdminGatewayModelCatalog,
  saveAdminGatewayModelMapping,
  testAdminGatewayConnection,
  updateAdminAIGatewayConfig,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  GatewayConfigSummary,
  GatewayConnectionTest,
  GatewayModelCatalogItem,
  GatewayModelDetection,
  GatewayModelObserved,
  GatewayModelReference,
  GatewayProviderOption,
  GatewayRoutePreview,
} from '../../../../shared/api/generated-query/model'
import type { UpdateGatewayConfigRequest } from '../../../../shared/api/generated-query/model/updateGatewayConfigRequest'
import { resolveApiData, resolveApiOk } from '../../../../shared/api/apiResponse'
import type {
  AdminModelItem,
  ModelObservedSnapshot,
  ModelReferenceSnapshot,
} from '../domain/catalogData'
import { MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY } from '../domain/catalogData'

export type {
  DetectedGatewayModel,
  GatewayProviderOption,
} from '../../../../shared/api/generated-query/model'

export type GatewayConnectionSummary = GatewayConfigSummary

export interface AdminGatewayModelMapping {
  feature: string
  model: string
}

export interface AdminGatewayCatalog {
  list: () => Promise<AdminModelItem[]>
  save: (items: AdminModelItem[]) => Promise<void>
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
  update: (input: UpdateGatewayConfigRequest) => Promise<void>
  testConnection: (input: {
    base_url: string
    token?: string | undefined
  }) => Promise<{ success: boolean; latencyMs?: number; message?: string }>
}

export interface AdminGatewayMappings {
  list: () => Promise<AdminGatewayModelMapping[]>
  save: (feature: string, model: string) => Promise<void>
}

export interface AdminGatewayResource {
  catalog: AdminGatewayCatalog
  connection: AdminGatewayConnection
  mappings: AdminGatewayMappings
}

// AI 网关设置面唯一出口：目录、连接、模型映射三组共享 adapter 与归一，
// 调用方按组取用（catalog / connection / mappings），不得直调 generated client。
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
    mappings: {
      list: listGatewayModelMappings,
      save: saveGatewayModelMapping,
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

async function listModelCatalog(): Promise<AdminModelItem[]> {
  const data = await resolveApiData(listAdminGatewayModelCatalog(), '加载模型库失败')
  if (!Array.isArray(data)) throw new Error('加载模型库失败')
  return data.map(toAdminModelItem)
}

async function saveModelCatalog(items: AdminModelItem[]): Promise<void> {
  await resolveApiOk(
    saveAdminGatewayModelCatalog({ items: items.map(toGatewayCatalogItem) }),
    '保存模型库失败',
  )
}

async function previewModelRoute(input: {
  modelId: string
  customPath?: string | undefined
  customParams?: string | undefined
}): Promise<GatewayRoutePreview> {
  const data = await resolveApiData(
    previewAdminGatewayRoute({
      model_id: input.modelId,
      ...(input.customPath ? { custom_path: input.customPath } : {}),
      ...(input.customParams ? { custom_params: input.customParams } : {}),
    }),
    '路由透视失败',
  )
  if (!data) throw new Error('路由透视失败')
  return data
}

async function detectGatewayModels(provider?: string): Promise<GatewayModelDetection> {
  const data = await resolveApiData(
    detectAdminGatewayModels(provider ? { provider } : undefined),
    '探测网关模型失败',
  )
  if (!data) throw new Error('探测网关模型失败')
  return data
}

async function listGatewayProviders(): Promise<GatewayProviderOption[]> {
  const data = await resolveApiData(listAdminGatewayProviders(), '加载供应商列表失败')
  if (!Array.isArray(data)) throw new Error('加载供应商列表失败')
  return data
}

function toAdminModelReference(item: GatewayModelReference): ModelReferenceSnapshot {
  return {
    modelId: item.model_id,
    provider: item.provider,
    name: item.name,
    inputUsdPerMtok: item.input_usd_per_mtok,
    outputUsdPerMtok: item.output_usd_per_mtok,
    contextLimit: item.context_limit,
    lastUpdated: item.last_updated,
    reasoning: item.reasoning,
    toolCall: item.tool_call,
    temperature: item.temperature,
    attachment: item.attachment,
    structuredOutput: item.structured_output,
    openWeights: item.open_weights,
    modalities:
      item.modalities == null
        ? undefined
        : { input: item.modalities.input, output: item.modalities.output },
    knowledge: item.knowledge,
    releaseDate: item.release_date,
    family: item.family,
    status: item.status,
    source: item.source,
    note: item.note,
    found: item.found,
    errorCode: item.error_code,
    message: item.message,
  }
}

async function getModelReference(input: {
  modelId?: string | undefined
  provider?: string | undefined
  model?: string | undefined
  refresh?: boolean | undefined
}): Promise<ModelReferenceSnapshot> {
  const data = await resolveApiData(
    getAdminGatewayModelReference({
      ...(input.modelId?.trim() ? { model_id: input.modelId.trim() } : {}),
      ...(input.provider?.trim() ? { provider: input.provider.trim() } : {}),
      ...(input.model?.trim() ? { model: input.model.trim() } : {}),
      ...(input.refresh ? { refresh: true } : {}),
    }),
    '加载参照价失败',
  )
  if (!data) throw new Error('加载参照价失败')
  return toAdminModelReference(data)
}

function toAdminModelObserved(item: GatewayModelObserved): ModelObservedSnapshot {
  return {
    model: item.model,
    provider: item.provider,
    windowDays: item.window_days,
    windowSince: item.window_since,
    attemptsTotal: item.attempts_total,
    completed: item.completed,
    failed: item.failed,
    cacheHits: item.cache_hits,
    // exactOptionalPropertyTypes：nullable 可选值直接透传（null 即未知，调用方暂无数据）。
    successRate: item.success_rate,
    cacheHitRate: item.cache_hit_rate,
    topFailureStage: item.top_failure_stage,
    topFailureCategory: item.top_failure_category,
    topFailureCount: item.top_failure_count,
    hasObservations: item.has_observations,
    hasSkuMargin: item.has_sku_margin,
    skuMargins: (item.sku_margins ?? []).map((margin) => ({
      sku: margin.sku,
      displayName: margin.display_name,
      feature: margin.feature,
      onSale: margin.on_sale,
      unitCreditsByVariant: { ...margin.unit_credits_by_variant },
      actualCostCredits: margin.actual_cost_credits,
      hasActualCost: margin.has_actual_cost,
      costSource: margin.cost_source,
      costNote: margin.cost_note,
      marginCredits: margin.margin_credits,
      marginStatus: margin.margin_status,
    })),
  }
}

async function listModelsObserved(): Promise<ModelObservedSnapshot[]> {
  const data = await resolveApiData(listAdminGatewayModelsObserved(), '加载观测数据失败')
  if (!Array.isArray(data)) throw new Error('加载观测数据失败')
  return data.map(toAdminModelObserved)
}

async function getGatewayConnection(): Promise<GatewayConnectionSummary> {
  const response = await getAdminAIGatewayConfig()
  if (!response.data) throw new Error('AI 网关配置不存在')
  return response.data
}

async function updateGatewayConnection(input: UpdateGatewayConfigRequest): Promise<void> {
  await resolveApiOk(updateAdminAIGatewayConfig(input), '保存 AI 网关配置失败')
}

async function testGatewayConnection(input: {
  base_url: string
  token?: string | undefined
}): Promise<{ success: boolean; latencyMs?: number; message?: string }> {
  // 服务端探活：浏览器读不到跨域状态码，fetch 恒报成功是误报。
  // token 仅用于本次探测，不做任何持久化。
  const data: GatewayConnectionTest | null | undefined = await resolveApiData(
    testAdminGatewayConnection({
      base_url: input.base_url.trim(),
      ...(input.token?.trim() ? { token: input.token.trim() } : {}),
    }),
    '网关连接测试失败',
  )
  if (!data) throw new Error('网关连接测试失败')
  return {
    success: data.reachable,
    latencyMs: data.latency_ms,
    message: data.message,
  }
}

async function listGatewayModelMappings(): Promise<AdminGatewayModelMapping[]> {
  const data = await resolveApiData(listAdminGatewayModelMappings(), '加载模型映射失败')
  return (data ?? []).map((item) => ({
    feature: item.feature,
    model: item.model,
  }))
}

async function saveGatewayModelMapping(feature: string, model: string): Promise<void> {
  await resolveApiOk(saveAdminGatewayModelMapping(feature, { model }), '保存模型映射失败')
}
