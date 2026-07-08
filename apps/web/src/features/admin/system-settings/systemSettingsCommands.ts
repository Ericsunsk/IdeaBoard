import {
  updateAdminBrandingSetting,
  updateAdminMediaUploadSetting,
  updateAdminGoogleNativeGenerationSetting,
  updateAdminPublicURLSetting,
  updateAdminRateLimitSettings,
  updateAdminRegistrationSetting,
  updateAdminTaskMonitorSetting,
} from '../../../shared/api/generated-query/admin-settings/admin-settings'
import type {
  ApiResponseRegistrationSettingData,
  BrandingSetting,
  GoogleNativeGenerationSetting,
  MediaUploadSetting,
  PublicURLSetting,
  RateLimitSetting,
  TaskMonitorSetting,
  UpdateAdminRegistrationSettingBody,
} from '../../../shared/api/generated-query/model'
import { requireApiData } from '../shared/queries/adminApiResponse'

export async function updateRateLimitSettings(data: RateLimitSetting): Promise<RateLimitSetting> {
  const res = await updateAdminRateLimitSettings(data)
  return requireApiData(res, '保存访问限流失败')
}

export async function updateBrandingSetting(data: BrandingSetting): Promise<BrandingSetting> {
  const res = await updateAdminBrandingSetting(data)
  return requireApiData(res, '保存项目品牌失败')
}

export async function updateTaskMonitorSetting(
  data: TaskMonitorSetting,
): Promise<TaskMonitorSetting> {
  const res = await updateAdminTaskMonitorSetting(data)
  return requireApiData(res, '保存任务监控设置失败')
}

export async function updateMediaUploadSetting(
  data: MediaUploadSetting,
): Promise<MediaUploadSetting> {
  const res = await updateAdminMediaUploadSetting(data)
  return requireApiData(res, '保存上传设置失败')
}

export async function updateGoogleNativeGenerationSetting(
  data: GoogleNativeGenerationSetting,
): Promise<GoogleNativeGenerationSetting> {
  const res = await updateAdminGoogleNativeGenerationSetting(data)
  return requireApiData(res, '保存生成策略灰度失败')
}

export async function updatePublicURLSetting(data: PublicURLSetting): Promise<PublicURLSetting> {
  const res = await updateAdminPublicURLSetting(data)
  return requireApiData(res, '保存链接与域名失败')
}

export async function updateRegistrationSetting(
  enabled: UpdateAdminRegistrationSettingBody['enabled'],
): Promise<ApiResponseRegistrationSettingData> {
  const res = await updateAdminRegistrationSetting({ enabled })
  return requireApiData(res, '更新注册状态失败')
}
