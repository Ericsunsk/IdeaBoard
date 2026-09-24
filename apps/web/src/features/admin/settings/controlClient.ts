import {
  getGetControlBrandingSettingQueryKey,
  getGetControlMediaUploadSettingQueryKey,
  getGetControlRateLimitSettingQueryKey,
  getGetControlRegistrationSettingQueryKey,
  updateControlBrandingSetting,
  updateControlMediaUploadSetting,
  updateControlRateLimitSetting,
  updateControlRegistrationSetting,
} from '../../../shared/api/generated-query/control/control-platform/control-platform'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../shared/api/contractKernel'

// Settings view types: feature-owned copies of the runtime settings shapes
// served by the Control Plane. The backend serves the same settings structs
// as before; only the transport moved (/api/control/v1 with the contract
// envelope), so views keep working without generated legacy model code.
export type BrandingSetting = {
  project_name: string
  logo_url: string
}

export type MediaUploadSetting = {
  upload_max_concurrent: number
}

export type RateLimitSetting = {
  window_seconds: number
  max_requests: number
}

export type RegistrationSettingData = {
  enabled: boolean
}

export type SettingsMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

function requireMutationReason(options: SettingsMutationOptions): string {
  const reason = options.operatorReason.trim()
  if (!reason) throw new Error('需要填写操作原因')
  return reason
}

// Settings bodies are strict-decoded server-side: the operator reason
// travels ONLY in the X-Operator-Reason header, never in the JSON body.
function settingsMutationHeaders(options: SettingsMutationOptions) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey: options.idempotencyKey ?? newRequestId(),
    operatorReason: requireMutationReason(options),
  })
}

export const brandingSettingQueryKey = getGetControlBrandingSettingQueryKey()
export const mediaUploadSettingQueryKey = getGetControlMediaUploadSettingQueryKey()
export const rateLimitSettingQueryKey = getGetControlRateLimitSettingQueryKey()
export const registrationSettingQueryKey = getGetControlRegistrationSettingQueryKey()

export async function saveBrandingSetting(
  draft: BrandingSetting,
  options: SettingsMutationOptions,
): Promise<BrandingSetting> {
  const body = await updateControlBrandingSetting(
    { project_name: draft.project_name, logo_url: draft.logo_url },
    { headers: settingsMutationHeaders(options) },
  )
  return requireContractData<BrandingSetting>(body, '保存项目品牌失败')
}

export async function saveMediaUploadSetting(
  draft: MediaUploadSetting,
  options: SettingsMutationOptions,
): Promise<MediaUploadSetting> {
  const body = await updateControlMediaUploadSetting(
    { upload_max_concurrent: draft.upload_max_concurrent },
    { headers: settingsMutationHeaders(options) },
  )
  return requireContractData<MediaUploadSetting>(body, '保存上传设置失败')
}

export async function saveRateLimitSetting(
  draft: RateLimitSetting,
  options: SettingsMutationOptions,
): Promise<RateLimitSetting> {
  const body = await updateControlRateLimitSetting(
    { window_seconds: draft.window_seconds, max_requests: draft.max_requests },
    { headers: settingsMutationHeaders(options) },
  )
  return requireContractData<RateLimitSetting>(body, '保存访问限流失败')
}

export async function saveRegistrationSetting(
  enabled: boolean,
  options: SettingsMutationOptions,
): Promise<RegistrationSettingData> {
  const body = await updateControlRegistrationSetting(
    { enabled },
    { headers: settingsMutationHeaders(options) },
  )
  return requireContractData<RegistrationSettingData>(body, '更新注册状态失败')
}
