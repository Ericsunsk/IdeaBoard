import {
  updateAdminBrandingSetting,
  updateAdminMediaUploadSetting,
  updateAdminPublicURLSetting,
  updateAdminRateLimitSettings,
  updateAdminRegistrationSetting,
  updateAdminTaskMonitorSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseRegistrationSettingData,
  BrandingSetting,
  MediaUploadSetting,
  PublicURLSetting,
  RateLimitSetting,
  TaskMonitorSetting,
  UpdateAdminRegistrationSettingBody,
} from '../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../shared/api/apiResponse'

export async function updateRateLimitSettings(data: RateLimitSetting): Promise<RateLimitSetting> {
  return resolveApiData(updateAdminRateLimitSettings(data), '保存访问限流失败')
}

export async function updateBrandingSetting(data: BrandingSetting): Promise<BrandingSetting> {
  return resolveApiData(updateAdminBrandingSetting(data), '保存项目品牌失败')
}

export async function updateTaskMonitorSetting(
  data: TaskMonitorSetting,
): Promise<TaskMonitorSetting> {
  return resolveApiData(updateAdminTaskMonitorSetting(data), '保存任务监控设置失败')
}

export async function updateMediaUploadSetting(
  data: MediaUploadSetting,
): Promise<MediaUploadSetting> {
  return resolveApiData(updateAdminMediaUploadSetting(data), '保存上传设置失败')
}

export async function updatePublicURLSetting(data: PublicURLSetting): Promise<PublicURLSetting> {
  return resolveApiData(updateAdminPublicURLSetting(data), '保存链接与域名失败')
}

export async function updateRegistrationSetting(
  enabled: UpdateAdminRegistrationSettingBody['enabled'],
): Promise<ApiResponseRegistrationSettingData> {
  return resolveApiData(updateAdminRegistrationSetting({ enabled }), '更新注册状态失败')
}
