import { formatAdminDisplayError, type AdminToastPort, useAdminToast } from '../../shared/index'
import { unsupportedUploadImageMessage } from '@/core/utils/imageUploadSupport'

// 反馈公约（admin）：领域文案收口到本文件的 feedback 工厂，调用方只表达意图
// （saved/saveFailed/testPassed/…），不直接拼 toast 文案。`useAdminToast` 直调仅
// 保留给无领域归属的一次性提示；新增领域提示先在这里加工厂方法。

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

function createAdminBrandingSettingsFeedback(toast: AdminToastPort): AdminBrandingSettingsFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '加载系统设置失败')),
    validationFailed: (message) => toast.error(message),
    saved: () => toast.success('项目品牌已保存'),
    saveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存项目品牌失败')),
    logoUploaded: () => toast.success('Logo 已上传，保存后生效'),
    logoUploadFailed: (error) => toast.error(formatAdminDisplayError(error, '上传 Logo 失败')),
  }
}

function createAdminAccessSettingsFeedback(toast: AdminToastPort): AdminAccessSettingsFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '加载访问控制设置失败')),
    rateLimitValidationFailed: (message) => toast.error(message),
    rateLimitSaved: () => toast.success('访问限流已保存'),
    rateLimitSaveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存失败')),
    loginLockoutValidationFailed: (message) => toast.error(message),
    loginLockoutSaved: () => toast.success('登录锁定已保存'),
    loginLockoutSaveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存失败')),
    registrationUpdated: (enabled) => toast.success(enabled ? '已开放注册' : '已关闭注册'),
    registrationUpdateFailed: (error) =>
      toast.error(formatAdminDisplayError(error, '更新注册状态失败')),
  }
}

function createAdminMediaUploadSettingsFeedback(
  toast: AdminToastPort,
): AdminMediaUploadSettingsFeedback {
  return createAdminSettingFeedback(toast, '加载上传设置失败', '上传设置已保存', '保存上传设置失败')
}

function createAdminOutboundPolicyFeedback(toast: AdminToastPort): AdminOutboundPolicyFeedback {
  return createAdminSettingFeedback(
    toast,
    '加载私有地址访问策略失败',
    '私有地址访问策略已保存',
    '保存私有地址访问策略失败',
  )
}

export function useAdminBrandingSettingsFeedback() {
  return createAdminBrandingSettingsFeedback(useAdminToast())
}

export function useAdminAccessSettingsFeedback() {
  return createAdminAccessSettingsFeedback(useAdminToast())
}

export function useAdminMediaUploadSettingsFeedback() {
  return createAdminMediaUploadSettingsFeedback(useAdminToast())
}

export function useAdminOutboundPolicyFeedback() {
  return createAdminOutboundPolicyFeedback(useAdminToast())
}

function createAdminMailSettingsFeedback(toast: AdminToastPort): AdminMailSettingsFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '加载邮件配置失败')),
    saved: () => toast.success('邮件配置已保存，请发送 Test Email'),
    saveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存邮件配置失败')),
    testSent: () => toast.success('Test Email 已发送'),
    testFailed: (error) => toast.error(formatAdminDisplayError(error, '发送 Test Email 失败')),
    cleared: () => toast.success('邮件配置已删除'),
    clearFailed: (error) => toast.error(formatAdminDisplayError(error, '删除邮件配置失败')),
  }
}

function createAdminSearchProviderFeedback(toast: AdminToastPort): AdminSearchProviderFeedback {
  return {
    loadFailed: (error) =>
      toast.error(formatAdminDisplayError(error, '加载图片搜索供应商配置失败')),
    keySaved: (providerLabel, wasEnabled) =>
      toast.success(
        wasEnabled
          ? `${providerLabel} API Key 已保存并已停用，请测试连接后重新启用`
          : `${providerLabel} API Key 已保存，请测试连接`,
      ),
    saveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存 API Key 失败')),
    testPassed: (providerLabel) => toast.success(`${providerLabel} 连接测试通过`),
    testFailed: (error) => toast.error(formatAdminDisplayError(error, '连接测试失败')),
    toggled: (providerLabel, enabled) =>
      toast.success(enabled ? `${providerLabel} 已启用` : `${providerLabel} 已停用`),
    toggleFailed: (error, enabled) =>
      toast.error(formatAdminDisplayError(error, enabled ? '启用失败' : '停用失败')),
  }
}

export function useAdminMailSettingsFeedback() {
  return createAdminMailSettingsFeedback(useAdminToast())
}

export function useAdminSearchProviderFeedback() {
  return createAdminSearchProviderFeedback(useAdminToast())
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

export function useAdminSupportChannelsFeedback() {
  return createAdminSettingFeedback(
    useAdminToast(),
    '读取客户支持失败',
    '客户支持已保存',
    '保存客户支持失败',
  )
}

function createAdminSettingFeedback(
  toast: AdminToastPort,
  loadFallback: string,
  savedMessage: string,
  saveFallback: string,
): AdminSettingFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, loadFallback)),
    validationFailed: (message) => toast.error(message),
    saved: () => toast.success(savedMessage),
    saveFailed: (error) => toast.error(formatAdminDisplayError(error, saveFallback)),
  }
}
