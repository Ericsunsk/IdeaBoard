import {
  createAdminAccount,
  deleteAdminAccount,
  getAdminAccountLimits,
  listAdminAccounts,
  updateAdminAccount,
} from '../../../shared/api/generated-query/admin-accounts/admin-accounts'
import {
  createAdminChannel,
  deleteAdminChannel as deleteAdminChannelRequest,
  detectAdminChannelModels as detectAdminChannelModelsRequest,
  listAdminChannels,
  updateAdminChannel,
} from '../../../shared/api/generated-query/admin-channels/admin-channels'
import type {
  AccountLimitRange,
  AccountLimits,
  AdminAccount as AdminAccountDto,
  AdminAccountCreateRequest,
  AdminAccountUpdateRequest,
  Channel as ChannelDto,
} from '../../../shared/api/generated-query/model'
import { assertApiOk, requireApiData } from '../shared/queries/adminApiResponse'
import type {
  AdminProviderAccountLimits,
  ProviderModelDetectionInput,
  ProviderAccountRecord,
  ProviderApiTypeValue,
  ProviderChannelFormData,
  ProviderChannelRecord,
} from './providerConfigTypes'
import { createChannelSavePayload } from './providerConfigPayload'

export const FALLBACK_ADMIN_ACCOUNT_LIMITS: AdminProviderAccountLimits = {
  maxConcurrent: { min: 1, max: 64, default: 3 },
  timeoutSeconds: { min: 1, max: 600, default: 120 },
}

type SaveAdminChannelInput = {
  id?: number
  data: ProviderChannelFormData
}

const ADMIN_ACCOUNT_LIST_PAGE_SIZE = 100

export async function fetchAdminChannels(): Promise<ProviderChannelRecord[]> {
  const res = await listAdminChannels()
  const data = requireApiData(res, '加载渠道商失败')
  return (data.list ?? []).map(toProviderChannelRecord)
}

export async function saveAdminChannel(input: SaveAdminChannelInput): Promise<void> {
  const payload = createChannelSavePayload(input.data)
  const res =
    input.id != null
      ? await updateAdminChannel(input.id, payload)
      : await createAdminChannel(payload)
  assertApiOk(res, '保存渠道商失败')
}

export async function deleteAdminChannel(id: number): Promise<void> {
  const res = await deleteAdminChannelRequest(id)
  assertApiOk(res, '删除渠道商失败')
}

export async function detectAdminChannelModels(input: ProviderModelDetectionInput): Promise<string[]> {
  const res = await detectAdminChannelModelsRequest(toDetectModelsRequest(input))
  const data = requireApiData(res, '自动检测模型失败')
  return data.models ?? []
}

export async function fetchAdminAccountLimits(): Promise<AdminProviderAccountLimits> {
  const res = await getAdminAccountLimits()
  const data = requireApiData(res, '加载账号限制失败')
  return normalizeAdminAccountLimits(data.limits)
}

export async function fetchAllAdminAccounts(): Promise<ProviderAccountRecord[]> {
  const all: ProviderAccountRecord[] = []
  let page = 1
  let total = Number.POSITIVE_INFINITY

  while (true) {
    const res = await listAdminAccounts({
      page,
      page_size: ADMIN_ACCOUNT_LIST_PAGE_SIZE,
    })
    const data = requireApiData(res, '加载账号失败')
    const list = (data.list ?? []).map(toProviderAccountRecord)
    total = data.total ?? total
    all.push(...list)
    if (all.length >= total || list.length === 0) break
    page += 1
  }

  return all
}

export async function createAdminAccountRecord(data: AdminAccountCreateRequest): Promise<void> {
  const res = await createAdminAccount(data)
  assertApiOk(res, '创建账号失败')
}

export async function updateAdminAccountRecord(
  id: number,
  data: AdminAccountUpdateRequest,
): Promise<void> {
  const res = await updateAdminAccount(id, data)
  assertApiOk(res, '更新账号失败')
}

export async function deleteAdminAccountRecord(id: number): Promise<void> {
  const res = await deleteAdminAccount(id)
  assertApiOk(res, '删除账号失败')
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

function toProviderChannelRecord(channel: ChannelDto): ProviderChannelRecord {
  return {
    id: channel.id,
    apiType: channel.type as ProviderApiTypeValue,
    name: channel.name,
    baseUrl: channel.api_url,
    status: channel.status,
    createdAt: channel.created_at,
    updatedAt: channel.updated_at,
  }
}

function toProviderAccountRecord(account: AdminAccountDto): ProviderAccountRecord {
  return {
    id: account.id,
    name: account.name,
    channelId: account.channel_id,
    platform: account.platform,
    apiType: account.api_type,
    baseUrl: account.proxy_url,
    models: account.models.map((model) => ({
      name: model.name,
      enabled: model.enabled,
      weight: model.weight,
      maxConcurrent: model.max_concurrent,
      featureSlugs: model.feature_slugs,
      sortOrder: model.sort_order,
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
