import { defineFeedback, formatAdminDisplayError, useAdminToast } from '../../shared/index'
import { unsupportedUploadImageMessage } from '@/core/utils/imageUploadSupport'

type AdminSettingFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

export type AdminMailSettingsFeedback = {
  loadFailed: (error: unknown) => void
  saved: () => void
  saveFailed: (error: unknown) => void
  testSent: () => void
  testFailed: (error: unknown) => void
  cleared: () => void
  clearFailed: (error: unknown) => void
}

export type AdminSearchProviderFeedback = {
  loadFailed: (error: unknown) => void
  keySaved: (providerLabel: string, wasEnabled: boolean) => void
  saveFailed: (error: unknown) => void
  testPassed: (providerLabel: string) => void
  testFailed: (error: unknown) => void
  toggled: (providerLabel: string, enabled: boolean) => void
  toggleFailed: (error: unknown, enabled: boolean) => void
}

export type AdminBrandingSettingsFeedback = AdminSettingFeedback & {
  logoUploaded: () => void
  logoUploadFailed: (error: unknown) => void
}

export type AdminAccessSettingsFeedback = {
  loadFailed: (error: unknown) => void
  rateLimitValidationFailed: (message: string) => void
  rateLimitSaved: () => void
  rateLimitSaveFailed: (error: unknown) => void
  loginLockoutValidationFailed: (message: string) => void
  loginLockoutSaved: () => void
  loginLockoutSaveFailed: (error: unknown) => void
  registrationUpdated: (enabled: boolean) => void
  registrationUpdateFailed: (error: unknown) => void
}

export type AdminMediaUploadSettingsFeedback = AdminSettingFeedback
export type AdminOutboundPolicyFeedback = AdminSettingFeedback

const ADMIN_BRANDING_SETTINGS_COPY = {
  loadFailed: { error: '加载系统设置失败' },
  validationFailed: { errorText: (message: string) => message },
  saved: { success: '项目品牌已保存' },
  saveFailed: { error: '保存项目品牌失败' },
  logoUploaded: { success: 'Logo 已上传，保存后生效' },
  logoUploadFailed: { error: '上传 Logo 失败' },
}

const ADMIN_ACCESS_SETTINGS_COPY = {
  loadFailed: { error: '加载访问控制设置失败' },
  rateLimitValidationFailed: { errorText: (message: string) => message },
  rateLimitSaved: { success: '访问限流已保存' },
  rateLimitSaveFailed: { error: '保存访问限流失败' },
  loginLockoutValidationFailed: { errorText: (message: string) => message },
  loginLockoutSaved: { success: '登录锁定已保存' },
  loginLockoutSaveFailed: { error: '保存登录锁定失败' },
  registrationUpdated: {
    success: (isEnabled: boolean) => (isEnabled ? '已开放注册' : '已关闭注册'),
  },
  registrationUpdateFailed: { error: '更新注册状态失败' },
}

const ADMIN_MAIL_SETTINGS_COPY = {
  loadFailed: { error: '加载邮件配置失败' },
  saved: { success: '邮件配置已保存，请发送 Test Email' },
  saveFailed: { error: '保存邮件配置失败' },
  testSent: { success: 'Test Email 已发送' },
  testFailed: { error: '发送 Test Email 失败' },
  cleared: { success: '邮件配置已删除' },
  clearFailed: { error: '删除邮件配置失败' },
}

const ADMIN_SEARCH_PROVIDER_COPY = {
  loadFailed: { error: '加载图片搜索供应商配置失败' },
  keySaved: {
    success: (providerLabel: string, wasEnabled: boolean) =>
      wasEnabled
        ? `${providerLabel} API Key 已保存并已停用，请测试连接后重新启用`
        : `${providerLabel} API Key 已保存，请测试连接`,
  },
  saveFailed: { error: '保存 API Key 失败' },
  testPassed: { success: (providerLabel: string) => `${providerLabel} 连接测试通过` },
  testFailed: { error: '连接测试失败' },
  toggled: {
    success: (providerLabel: string, isEnabled: boolean) =>
      isEnabled ? `${providerLabel} 已启用` : `${providerLabel} 已停用`,
  },
  toggleFailed: {
    error: (isEnabled: boolean) => (isEnabled ? '启用失败' : '停用失败'),
  },
}

export function useAdminBrandingSettingsFeedback(): AdminBrandingSettingsFeedback {
  return defineFeedback<AdminBrandingSettingsFeedback>(ADMIN_BRANDING_SETTINGS_COPY)
}

export function useAdminAccessSettingsFeedback(): AdminAccessSettingsFeedback {
  return defineFeedback<AdminAccessSettingsFeedback>(ADMIN_ACCESS_SETTINGS_COPY)
}

export function useAdminMediaUploadSettingsFeedback(): AdminMediaUploadSettingsFeedback {
  return defineAdminSettingFeedback('加载上传设置失败', '上传设置已保存', '保存上传设置失败')
}

export function useAdminOutboundPolicyFeedback(): AdminOutboundPolicyFeedback {
  return defineAdminSettingFeedback(
    '加载私有地址访问策略失败',
    '私有地址访问策略已保存',
    '保存私有地址访问策略失败',
  )
}

export function useAdminMailSettingsFeedback(): AdminMailSettingsFeedback {
  return defineFeedback<AdminMailSettingsFeedback>(ADMIN_MAIL_SETTINGS_COPY)
}

export function useAdminSearchProviderFeedback(): AdminSearchProviderFeedback {
  return defineFeedback<AdminSearchProviderFeedback>(ADMIN_SEARCH_PROVIDER_COPY)
}

export function useAdminUnsupportedLogoFileFeedback() {
  const toast = useAdminToast()
  return (error?: unknown) =>
    toast.error(
      error
        ? formatAdminDisplayError(error, unsupportedUploadImageMessage())
        : unsupportedUploadImageMessage(),
    )
}

export function useAdminSupportChannelsFeedback(): AdminSettingFeedback {
  return defineAdminSettingFeedback('读取客户支持失败', '客户支持已保存', '保存客户支持失败')
}

function defineAdminSettingFeedback(
  loadFallback: string,
  savedMessage: string,
  saveFallback: string,
): AdminSettingFeedback {
  return defineFeedback<AdminSettingFeedback>({
    loadFailed: { error: loadFallback },
    validationFailed: { errorText: (message: string) => message },
    saved: { success: savedMessage },
    saveFailed: { error: saveFallback },
  })
}
