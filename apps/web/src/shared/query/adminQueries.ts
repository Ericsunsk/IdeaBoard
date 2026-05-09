import { adminApi } from '../api/admin'
import { featurePromptApi } from '../api/featurePrompt'
import { channelApi } from '../api/channel'
import { accountApi } from '../api/account'
import { adminStatisticsApi } from '../api/adminStatistics'
import { storageApi } from '../api/storage'
import type { ApiResponse, UserListResponse } from '../types'
import type { ApiAccount } from '../api/account'
import type { AdminStatisticsBackfillResult, AdminStatisticsResponse } from '../api/adminStatistics'
import type { Channel } from '../api/channel'
import type { StorageProfile, StorageProfilePayload, StorageProviderMeta } from '../api/storage'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'
import { queryClient } from './queryClient'
import { queryKeys } from './keys'

export type AdminStatisticsRange = '7d' | '30d' | '1y'
export type AdminUserListParams = { keyword?: string; page?: number; page_size?: number }
export type RateLimitSettings = { window_seconds: number; max_requests: number }
export type RegistrationSetting = { enabled: boolean }
export type AdminAccountListParams = { keyword?: string; platform?: string; page?: number; page_size?: number }
export type AdminAccountListResult = { accounts: ApiAccount[]; total: number }

type SaveFeaturePromptInput = { slug: string; prompt: string }
type SaveStorageProfileInput = { id?: string; payload: StorageProfilePayload }

function requireApiData<T>(res: ApiResponse<T>, fallback: string): T {
  if (res.code !== 0 || res.data == null) {
    throw new Error(sanitizeApiErrorMessage(res.msg, fallback))
  }
  return res.data
}

function requireApiOk<T>(res: ApiResponse<T>, fallback: string): T | undefined {
  if (res.code !== 0) {
    throw new Error(sanitizeApiErrorMessage(res.msg, fallback))
  }
  return res.data
}

export function invalidateAdminUsers() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.admin.usersAll() })
}

export function invalidateRegistrationSetting() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.admin.registrationSetting() })
}

export function invalidateRateLimitSettings() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.admin.rateLimitSettings() })
}

export function invalidateFeaturePrompts() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.admin.featurePrompts() })
}

export function invalidateAdminStatistics(range?: AdminStatisticsRange) {
  return queryClient.invalidateQueries({ queryKey: range ? queryKeys.admin.statistics(range) : ['admin', 'statistics'] })
}

export function invalidateStorageProfiles() {
  return queryClient.invalidateQueries({ queryKey: queryKeys.admin.storage.profiles() })
}

export async function fetchAdminUsers(params?: AdminUserListParams): Promise<UserListResponse> {
  const res = await adminApi.getUsers(params ?? { page: 1, page_size: 100 })
  return requireApiData(res, '获取用户列表失败')
}

export async function fetchRegistrationSetting(): Promise<RegistrationSetting> {
  const res = await adminApi.getRegistrationSetting()
  return requireApiData(res, '获取注册设置失败')
}

export async function fetchRateLimitSettings(): Promise<RateLimitSettings> {
  const res = await adminApi.getRateLimitSettings()
  return requireApiData(res, '获取限流设置失败')
}

export async function updateRateLimitSettings(data: RateLimitSettings): Promise<RateLimitSettings> {
  const res = await adminApi.updateRateLimitSettings(data)
  return requireApiData(res, '保存限流配置失败')
}

export async function updateRegistrationSetting(enabled: boolean): Promise<RegistrationSetting> {
  const res = await adminApi.updateRegistrationSetting(enabled)
  return requireApiData(res, '更新注册状态失败')
}

export async function updateAdminUserStatus(input: { id: number; status: number }): Promise<void> {
  const res = await adminApi.updateUserStatus(input.id, input.status)
  requireApiOk(res, input.status === 1 ? '解封失败' : '封禁失败')
}

export async function updateAdminUserRole(input: { id: number; role: 'admin' | 'user' }): Promise<void> {
  const res = await adminApi.updateUserRole(input.id, input.role)
  requireApiOk(res, '角色更新失败')
}

export async function deleteAdminUser(id: number): Promise<void> {
  const res = await adminApi.deleteUser(id)
  requireApiOk(res, '注销失败')
}

export async function fetchFeaturePrompts(): Promise<Record<string, string>> {
  const res = await featurePromptApi.adminList()
  return requireApiData(res, '获取提示词配置失败').prompts
}

export async function saveFeaturePrompt(input: SaveFeaturePromptInput): Promise<{ slug: string; prompt: string }> {
  const res = await featurePromptApi.adminSet(input.slug, input.prompt)
  return requireApiData(res, '保存提示词失败')
}

export async function fetchAdminStatistics(range: AdminStatisticsRange): Promise<AdminStatisticsResponse> {
  const res = await adminStatisticsApi.get(range)
  return requireApiData(res, '获取统计数据失败')
}

export async function triggerAdminStatisticsBackfill(range: AdminStatisticsRange): Promise<AdminStatisticsBackfillResult> {
  const res = await adminStatisticsApi.backfill(range)
  return requireApiData(res, '补录失败')
}

export async function fetchChannels(): Promise<Channel[]> {
  const res = await channelApi.list()
  return requireApiData(res, '获取渠道列表失败').list
}

export async function fetchAllAccounts(params?: AdminAccountListParams): Promise<AdminAccountListResult> {
  const pageSize = params?.page_size ?? 100
  const firstRes = await accountApi.listAccounts({ ...params, page: 1, page_size: pageSize })
  const firstPage = requireApiData(firstRes, '获取账号列表失败')
  const accounts = [...firstPage.list]
  const total = firstPage.total
  let page = 2

  while (accounts.length < total) {
    const res = await accountApi.listAccounts({ ...params, page, page_size: pageSize })
    const data = requireApiData(res, '获取账号列表失败')
    if (data.list.length === 0) break
    accounts.push(...data.list)
    page += 1
  }

  return { accounts, total }
}

export async function fetchStorageProviders(): Promise<StorageProviderMeta[]> {
  const res = await storageApi.getProviders()
  return requireApiData(res, '获取存储提供商失败').providers
}

export async function fetchStorageProfiles(): Promise<StorageProfile[]> {
  const res = await storageApi.getProfiles()
  return requireApiData(res, '获取存储配置失败').profiles
}

export async function saveStorageProfile(input: SaveStorageProfileInput): Promise<{ profile: StorageProfile; message: string }> {
  const res = input.id
    ? await storageApi.updateProfile(input.id, input.payload)
    : await storageApi.createProfile(input.payload)
  return requireApiData(res, '保存存储配置失败')
}

export async function applyStorageProfile(id: string): Promise<{ profile: StorageProfile; message: string }> {
  const res = await storageApi.applyProfile(id)
  return requireApiData(res, '设为当前失败')
}

export async function testStorageProfile(id: string): Promise<{ message: string }> {
  const res = await storageApi.testProfile(id)
  return requireApiData(res, '连接失败')
}

export async function deleteStorageProfile(id: string): Promise<{ message: string }> {
  const res = await storageApi.deleteProfile(id)
  return requireApiData(res, '删除失败')
}

export async function fetchStorageBucketDomains(profileId: string): Promise<string[]> {
  const res = await storageApi.getBucketInfo(profileId)
  const data = requireApiData(res, '获取 Bucket 信息失败')
  return data.info.domains ?? []
}
