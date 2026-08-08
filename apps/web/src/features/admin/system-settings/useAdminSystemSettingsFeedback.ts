import {
  formatAdminDisplayError,
  type AdminToastPort,
  useAdminToast,
} from '../shared/lib/adminToast'
import { unsupportedUploadImageMessage } from '../../../shared/utils/imageUploadSupport'

type AdminSettingFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
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
  registrationUpdated: (enabled: boolean) => void
  registrationUpdateFailed: (error: unknown) => void
}

export type AdminMediaUploadSettingsFeedback = AdminSettingFeedback
export type AdminOutboundPolicyFeedback = AdminSettingFeedback

export function createAdminBrandingSettingsFeedback(
  toast: AdminToastPort,
): AdminBrandingSettingsFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '加载系统设置失败')),
    validationFailed: (message) => toast.error(message),
    saved: () => toast.success('项目品牌已保存'),
    saveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存项目品牌失败')),
    logoUploaded: () => toast.success('Logo 已上传，保存后生效'),
    logoUploadFailed: (error) => toast.error(formatAdminDisplayError(error, '上传 Logo 失败')),
  }
}

export function createAdminAccessSettingsFeedback(
  toast: AdminToastPort,
): AdminAccessSettingsFeedback {
  return {
    loadFailed: (error) => toast.error(formatAdminDisplayError(error, '加载访问控制设置失败')),
    rateLimitValidationFailed: (message) => toast.error(message),
    rateLimitSaved: () => toast.success('访问限流已保存'),
    rateLimitSaveFailed: (error) => toast.error(formatAdminDisplayError(error, '保存失败')),
    registrationUpdated: (enabled) => toast.success(enabled ? '已开放注册' : '已关闭注册'),
    registrationUpdateFailed: (error) =>
      toast.error(formatAdminDisplayError(error, '更新注册状态失败')),
  }
}

export function createAdminMediaUploadSettingsFeedback(
  toast: AdminToastPort,
): AdminMediaUploadSettingsFeedback {
  return createAdminSettingFeedback(toast, '加载上传设置失败', '上传设置已保存', '保存上传设置失败')
}

export function createAdminOutboundPolicyFeedback(
  toast: AdminToastPort,
): AdminOutboundPolicyFeedback {
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

export function useAdminUnsupportedLogoFileFeedback() {
  const toast = useAdminToast()
  return () => toast.error(unsupportedUploadImageMessage())
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
