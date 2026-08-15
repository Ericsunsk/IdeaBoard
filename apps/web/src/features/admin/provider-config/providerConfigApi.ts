import {
  createAdminAccount,
  deleteAdminAccount,
  getAdminAccountLimits,
  listAdminAccounts,
  patchAdminAccountModel,
  updateAdminAccount,
} from '../../../shared/api/generated-query/admin-accounts/admin-accounts'
import {
  createAdminProviderEndpoint,
  deleteAdminProviderEndpoint as deleteAdminProviderEndpointRequest,
  detectAdminProviderEndpointModels as detectAdminProviderEndpointModelsRequest,
  listAdminProviderEndpoints,
  updateAdminProviderEndpoint,
} from '../../../shared/api/generated-query/admin-provider-endpoints/admin-provider-endpoints'
import type {
  AccountLimitRange,
  AccountLimits,
  AdminAccount as AdminAccountDto,
  AdminAccountCreateRequest,
  AdminAccountModelPatchRequest,
  AdminAccountUpdateRequest,
  ProviderEndpoint as ProviderEndpointDto,
} from '../../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../../shared/api/apiResponse'
import { FALLBACK_ADMIN_ACCOUNT_LIMITS } from './providerConfigLimits'
import type {
  AdminProviderAccountLimits,
  ProviderModelDetectionInput,
  ProviderAccountRecord,
  ProviderApiTypeValue,
  ProviderEndpointFormData,
  ProviderEndpointRecord,
} from './providerConfigTypes'
import { createProviderEndpointSavePayload } from './providerConfigPayload'

type SaveAdminProviderEndpointInput = {
  id?: number
  data: ProviderEndpointFormData
}

const ADMIN_ACCOUNT_LIST_PAGE_SIZE = 100
const PROVIDER_MODEL_DETECTION_TIMEOUT_MS = 20_000

export async function fetchAdminProviderEndpoints(): Promise<ProviderEndpointRecord[]> {
  const data = await resolveApiData(listAdminProviderEndpoints(), '加载供应商端点失败')
  return (data.list ?? []).map(toProviderEndpointRecord)
}

export async function saveAdminProviderEndpoint(
  input: SaveAdminProviderEndpointInput,
): Promise<void> {
  const payload = createProviderEndpointSavePayload(input.data)
  const request =
    input.id != null
      ? updateAdminProviderEndpoint(input.id, payload)
      : createAdminProviderEndpoint(payload)
  await resolveApiOk(request, '保存供应商端点失败')
}

export async function deleteAdminProviderEndpoint(id: number): Promise<void> {
  await resolveApiOk(deleteAdminProviderEndpointRequest(id), '删除供应商端点失败')
}

export async function detectAdminProviderEndpointModels(
  input: ProviderModelDetectionInput,
): Promise<string[]> {
  const data = await resolveApiData(
    detectAdminProviderEndpointModelsRequest(toDetectModelsRequest(input), {
      timeout: PROVIDER_MODEL_DETECTION_TIMEOUT_MS,
    }),
    '自动检测模型失败',
  )
  return data.models ?? []
}

export async function fetchAdminAccountLimits(): Promise<AdminProviderAccountLimits> {
  const data = await resolveApiData(getAdminAccountLimits(), '加载账号限制失败')
  return normalizeAdminAccountLimits(data.limits)
}

export async function fetchAllAdminAccounts(): Promise<ProviderAccountRecord[]> {
  const all: ProviderAccountRecord[] = []
  let page = 1
  let total = Number.POSITIVE_INFINITY

  while (true) {
    const data = await resolveApiData(
      listAdminAccounts({
        page,
        page_size: ADMIN_ACCOUNT_LIST_PAGE_SIZE,
      }),
      '加载账号失败',
    )
    const list = (data.list ?? []).map(toProviderAccountRecord)
    total = data.total ?? total
    all.push(...list)
    if (all.length >= total || list.length === 0) break
    page += 1
  }

  return all
}

export async function createAdminAccountRecord(data: AdminAccountCreateRequest): Promise<void> {
  await resolveApiOk(createAdminAccount(data), '创建账号失败')
}

export async function updateAdminAccountRecord(
  id: number,
  data: AdminAccountUpdateRequest,
): Promise<void> {
  await resolveApiOk(updateAdminAccount(id, data), '更新账号失败')
}

export async function patchAdminAccountModelRecord(
  id: number,
  data: AdminAccountModelPatchRequest,
): Promise<ProviderAccountRecord> {
  const responseData = await resolveApiData(patchAdminAccountModel(id, data), '更新账号模型失败')
  return toProviderAccountRecord(responseData.account)
}

export async function deleteAdminAccountRecord(id: number): Promise<void> {
  await resolveApiOk(deleteAdminAccount(id), '删除账号失败')
}

function normalizeAdminAccountLimits(limits?: AccountLimits): AdminProviderAccountLimits {
  return {
    maxConcurrent: normalizeAdminAccountLimitRange(
      limits?.max_concurrent,
      FALLBACK_ADMIN_ACCOUNT_LIMITS.maxConcurrent,
    ),
    timeoutSeconds: normalizeAdminAccountLimitRange(
      limits?.timeout_seconds,
      FALLBACK_ADMIN_ACCOUNT_LIMITS.timeoutSeconds,
    ),
  }
}

function normalizeAdminAccountLimitRange(
  range: AccountLimitRange | undefined,
  fallback: AccountLimitRange,
): AccountLimitRange {
  const rawMin = range?.min
  const rawMax = range?.max
  const rawDefault = range?.default
  const min =
    typeof rawMin === 'number' && Number.isFinite(rawMin) && rawMin > 0 ? rawMin : fallback.min
  const max =
    typeof rawMax === 'number' && Number.isFinite(rawMax) && rawMax >= min ? rawMax : fallback.max
  const defaultValue =
    typeof rawDefault === 'number' &&
    Number.isFinite(rawDefault) &&
    rawDefault >= min &&
    rawDefault <= max
      ? rawDefault
      : fallback.default
  return { min, max, default: defaultValue }
}

function toProviderEndpointRecord(providerEndpoint: ProviderEndpointDto): ProviderEndpointRecord {
  return {
    id: providerEndpoint.id,
    apiType: providerEndpoint.api_type as ProviderApiTypeValue,
    name: providerEndpoint.name,
    baseUrl: providerEndpoint.base_url,
    status: providerEndpoint.status,
    createdAt: providerEndpoint.created_at,
    updatedAt: providerEndpoint.updated_at,
  }
}

function toProviderAccountRecord(account: AdminAccountDto): ProviderAccountRecord {
  return {
    id: account.id,
    name: account.name,
    providerEndpointId: account.provider_endpoint_id,
    apiType: account.provider_endpoint_api_type,
    baseUrl: account.provider_endpoint_base_url,
    models: account.models.map((model) => ({
      name: model.name,
      enabled: model.enabled,
      weight: model.weight,
      maxConcurrent: model.max_concurrent,
      sortOrder: model.sort_order,
      supports2K: model.supports_2k,
      supports4K: model.supports_4k,
    })),
    secretConfigured: account.secret_configured,
    maxConcurrent: account.max_concurrent,
    currentConcurrent: account.current_concurrent ?? 0,
    waitQueueCount: account.wait_queue_count ?? 0,
    runningStatus: account.running_status ?? '',
    failCount: account.fail_count,
    weight: account.weight,
    timeoutSeconds: account.timeout,
    extraConfig: account.extra_config,
    status: account.status,
    createdAt: account.created_at,
    updatedAt: account.updated_at,
  }
}

function toDetectModelsRequest(input: ProviderModelDetectionInput) {
  return {
    base_url: input.baseUrl,
    api_key: input.apiKey,
    api_type: input.apiType,
  }
}
