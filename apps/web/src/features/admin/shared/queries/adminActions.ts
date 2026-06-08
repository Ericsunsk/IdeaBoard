import {
  updateAdminRateLimitSettings,
  updateAdminRegistrationSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import {
  banAdminUser as requestBanAdminUser,
  deleteAdminUser as requestDeleteAdminUser,
  revokeAdminUserSessions as requestRevokeAdminUserSessions,
  unbanAdminUser as requestUnbanAdminUser,
  updateAdminUserRole as requestUpdateAdminUserRole,
} from '../../../../shared/api/generated-query/admin-users/admin-users'
import { setAdminFeaturePrompt } from '../../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import { backfillAdminStatistics } from '../../../../shared/api/generated-query/admin-statistics/admin-statistics'
import type { ApiResponse } from '../../../../shared/types'
import type {
  AdminStatisticsBackfillResult,
  ApiResponseRegistrationSettingData,
  GetAdminStatisticsRange,
  RateLimitSetting,
  UpdateAdminRegistrationSettingBody,
} from '../../../../shared/api/generated-query/model'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'
import { storageApi, type StorageProfile, type StorageProfilePayload } from '../api/storage'

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

export async function updateRateLimitSettings(data: RateLimitSetting): Promise<RateLimitSetting> {
  const res = await updateAdminRateLimitSettings(data)
  return requireApiData(res, '保存限流配置失败')
}

export async function updateRegistrationSetting(
  enabled: UpdateAdminRegistrationSettingBody['enabled'],
): Promise<ApiResponseRegistrationSettingData> {
  const res = await updateAdminRegistrationSetting({ enabled })
  return requireApiData(res, '更新注册状态失败')
}

export async function banAdminUser(input: {
  id: number
  reason?: string
  expires_in_days?: number
}): Promise<void> {
  const res = await requestBanAdminUser(input.id, {
    reason: input.reason,
    expires_in_days: input.expires_in_days,
  })
  requireApiOk(res, '封禁失败')
}

export async function unbanAdminUser(id: number): Promise<void> {
  const res = await requestUnbanAdminUser(id)
  requireApiOk(res, '解封失败')
}

export async function revokeAdminUserSessions(id: number): Promise<void> {
  const res = await requestRevokeAdminUserSessions(id)
  requireApiOk(res, '吊销会话失败')
}

export async function updateAdminUserRole(input: {
  id: number
  role: 'admin' | 'user'
}): Promise<void> {
  const res = await requestUpdateAdminUserRole(input.id, { role: input.role })
  requireApiOk(res, '角色更新失败')
}

export async function deleteAdminUser(id: number): Promise<void> {
  const res = await requestDeleteAdminUser(id)
  requireApiOk(res, '注销失败')
}

export async function saveFeaturePrompt(
  input: SaveFeaturePromptInput,
): Promise<{ slug: string; prompt: string }> {
  const res = await setAdminFeaturePrompt(input.slug, { prompt: input.prompt })
  return requireApiData(res, '保存提示词失败')
}

export async function triggerAdminStatisticsBackfill(
  range: GetAdminStatisticsRange,
): Promise<AdminStatisticsBackfillResult> {
  const res = await backfillAdminStatistics({ range })
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
