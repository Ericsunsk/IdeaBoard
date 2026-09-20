import {
  detectAdminGatewayModels,
  getAdminAIGatewayConfig,
  listAdminGatewayModelCatalog,
  listAdminGatewayModelMappings,
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
  GatewayProviderOption,
  GatewayRoutePreview,
} from '../../../../shared/api/generated-query/model'
import type { UpdateGatewayConfigRequest } from '../../../../shared/api/generated-query/model/updateGatewayConfigRequest'
import { resolveApiData, resolveApiOk } from '../../../../shared/api/apiResponse'
import type { AdminModelItem } from '../domain/catalogData'
import {
  MODEL_HOSTING_CLOUDFLARE,
  MODEL_HOSTING_THIRD_PARTY,
} from '../domain/catalogData'

export type { DetectedGatewayModel, GatewayProviderOption } from '../../../../shared/api/generated-query/model'

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
    hosting: item.hosting === MODEL_HOSTING_THIRD_PARTY ? MODEL_HOSTING_THIRD_PARTY : MODEL_HOSTING_CLOUDFLARE,
    termsUrl: item.terms_url,
    docsUrl: item.docs_url,
    customPath: item.custom_path || '',
    customParams: item.custom_params || '',
    descriptionEnriched: item.description_enriched,
    inputCostPerToken: item.input_cost_per_token,
    outputCostPerToken: item.output_cost_per_token,
    cacheReadCostPerToken: item.cache_read_input_token_cost,
    cacheCreateCostPerToken: item.cache_creation_input_token_cost,
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
    ...(item.inputCostPerToken != null ? { input_cost_per_token: item.inputCostPerToken } : {}),
    ...(item.outputCostPerToken != null ? { output_cost_per_token: item.outputCostPerToken } : {}),
    ...(item.cacheReadCostPerToken != null ? { cache_read_input_token_cost: item.cacheReadCostPerToken } : {}),
    ...(item.cacheCreateCostPerToken != null ? { cache_creation_input_token_cost: item.cacheCreateCostPerToken } : {}),
    ...(item.supportsPromptCaching != null ? { supports_prompt_caching: item.supportsPromptCaching } : {}),
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
  await resolveApiOk(
    saveAdminGatewayModelMapping(feature, { model }),
    '保存模型映射失败',
  )
}
