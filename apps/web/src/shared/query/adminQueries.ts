import { adminApi } from '../api/admin'
import { featurePromptApi } from '../api/featurePrompt'
import { adminStatisticsApi } from '../api/adminStatistics'
import { storageApi } from '../api/storage'
import type { ApiResponse } from '../types'
import type { AdminStatisticsBackfillResult } from '../api/adminStatistics'
import type { StorageProfile, StorageProfilePayload } from '../api/storage'
import { sanitizeApiErrorMessage } from '../utils/errorMessages'

export type AdminStatisticsRange = '7d' | '30d' | '1y'
export type RateLimitSettings = { window_seconds: number; max_requests: number }
export type RegistrationSetting = { enabled: boolean }

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

export async function banAdminUser(input: {
  id: number
  reason?: string
  expires_in_days?: number
}): Promise<void> {
  const res = await adminApi.banUser(input.id, {
    reason: input.reason,
    expires_in_days: input.expires_in_days,
  })
  requireApiOk(res, '封禁失败')
}

export async function unbanAdminUser(id: number): Promise<void> {
  const res = await adminApi.unbanUser(id)
  requireApiOk(res, '解封失败')
}

export async function revokeAdminUserSessions(id: number): Promise<void> {
  const res = await adminApi.revokeUserSessions(id)
  requireApiOk(res, '吊销会话失败')
}

export async function updateAdminUserRole(input: {
  id: number
  role: 'admin' | 'user'
}): Promise<void> {
  const res = await adminApi.updateUserRole(input.id, input.role)
  requireApiOk(res, '角色更新失败')
}

export async function deleteAdminUser(id: number): Promise<void> {
  const res = await adminApi.deleteUser(id)
  requireApiOk(res, '注销失败')
}

export async function saveFeaturePrompt(
  input: SaveFeaturePromptInput,
): Promise<{ slug: string; prompt: string }> {
  const res = await featurePromptApi.adminSet(input.slug, input.prompt)
  return requireApiData(res, '保存提示词失败')
}

export async function triggerAdminStatisticsBackfill(
  range: AdminStatisticsRange,
): Promise<AdminStatisticsBackfillResult> {
  const res = await adminStatisticsApi.backfill(range)
  return requireApiData(res, '补录失败')
}

export async function saveStorageProfile(
  input: SaveStorageProfileInput,
): Promise<{ profile: StorageProfile; message: string }> {
  const res = input.id
    ? await storageApi.updateProfile(input.id, input.payload)
    : await storageApi.createProfile(input.payload)
  return requireApiData(res, '保存存储配置失败')
}

export async function applyStorageProfile(
  id: string,
): Promise<{ profile: StorageProfile; message: string }> {
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
