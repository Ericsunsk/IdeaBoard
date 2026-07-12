import { useAppToast } from '../../../shared/composables/useAppToast'
import {
  extractSafeApiErrorMessage,
  formatDisplayErrorMessage,
} from '../../../shared/utils/errorMessages'
import { unsupportedUploadImageMessage } from '../../../shared/utils/imageUploadSupport'

type AdminToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error'>

export type AdminProviderConfigFeedback = {
  accountLimitsFallback: (error: unknown) => void
  loadFailed: (error: unknown) => void
  staleProviderEndpoint: () => void
  staleAccount: () => void
  staleModel: () => void
  providerEndpointFormInvalid: () => void
  accountFormInvalid: () => void
  providerEndpointSaved: (isEditing: boolean) => void
  providerEndpointSaveFailed: (error: unknown) => void
  providerEndpointDeleted: () => void
  providerEndpointDeleteFailed: (error: unknown) => void
  accountSaved: (isEditing: boolean) => void
  accountSaveFailed: (error: unknown) => void
  accountDeleted: () => void
  accountDeleteFailed: (error: unknown) => void
  modelStatusChanged: (enabled: boolean) => void
  modelStatusChangeFailed: (error: unknown) => void
  missingApiKeyForModelDetection: () => void
  modelDetectionFailed: (error: unknown) => void
}

export type AdminUsersFeedback = {
  loadFailed: (error: unknown) => void
  adminStatusChangeForbidden: () => void
  unbanSucceeded: () => void
  unbanFailed: (error: unknown) => void
  banSucceeded: (username: string) => void
  banFailed: (error: unknown) => void
  missingAuthUserForSessionRevoke: () => void
  revokeSessionsSucceeded: () => void
  revokeSessionsFailed: (error: unknown) => void
  missingAuthUserForImpersonation: () => void
  impersonationStarted: (username: string) => void
  impersonationFailed: (error: unknown) => void
  userDeleted: () => void
  userDeleteFailed: (error: unknown) => void
  roleUpdated: (nextRole: 'admin' | 'user') => void
  roleUpdateFailed: (error: unknown) => void
}

export type AdminTaskResultFeedback = {
  unsafeImageUrlRejected: () => void
  copiedToClipboard: () => void
  copyFailed: () => void
}

export type AdminTaskDeletionFeedback = {
  noTasksSelected: () => void
  tasksDeleted: (deletedCount: number) => void
  deleteTasksFailed: (error: unknown) => void
}

export type AdminPromptConfigFeedback = {
  loadPromptsFailed: (error: unknown) => void
  promptDraftInvalid: (message: string) => void
  promptSaved: (featureTitle: string) => void
  savePromptFailed: (error: unknown) => void
}

export type AdminBrandingSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
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

export type AdminTaskMonitorSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

export type AdminPublicURLSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

export type AdminMediaUploadSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

export type AdminGoogleNativeGenerationSettingsFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed: (message: string) => void
  saved: () => void
  saveFailed: (error: unknown) => void
}

export type AdminCreditsBillingOperationsFeedback = {
  walletLoadFailed: (error: unknown) => void
  ledgerLoadFailed: (error: unknown) => void
  commandSucceeded: (label: string, replayed: boolean) => void
  commandFailed: (error: unknown, label: string) => void
  reconciliationLoadFailed: (error: unknown) => void
  reconciliationStarted: () => void
  reconciliationStartFailed: (error: unknown) => void
}

export type AdminBillingPricingFeedback = {
  success: (message: string) => void
  error: (error: unknown, fallback: string) => void
}

export type AdminFeedback = {
  login: {
    succeeded: () => void
    sessionRevokeFailed: () => void
    nonAdminAccount: () => void
    failed: (error: unknown) => void
  }
  providerEndpoints: {
    providerConfig: AdminProviderConfigFeedback
  }
  users: {
    management: AdminUsersFeedback
  }
  promptConfig: AdminPromptConfigFeedback
  systemSettings: {
    branding: AdminBrandingSettingsFeedback
    access: AdminAccessSettingsFeedback
    taskMonitor: AdminTaskMonitorSettingsFeedback
    publicURL: AdminPublicURLSettingsFeedback
    mediaUpload: AdminMediaUploadSettingsFeedback
    googleNativeGeneration: AdminGoogleNativeGenerationSettingsFeedback
    unsupportedLogoFile: () => void
  }
  taskMonitor: {
    resultDetail: AdminTaskResultFeedback
    taskDeletion: AdminTaskDeletionFeedback
  }
  creditsBilling: {
    operations: AdminCreditsBillingOperationsFeedback
    pricing: AdminBillingPricingFeedback
  }
}

export function createAdminFeedback(appToast: AdminToast): AdminFeedback {
  return {
    login: {
      succeeded() {
        appToast.success('管理员登录成功')
      },
      sessionRevokeFailed() {
        appToast.warning('会话撤销失败，请稍后重试')
      },
      nonAdminAccount() {
        appToast.error('该账号不是管理员')
      },
      failed(error) {
        appToast.error(formatDisplayErrorMessage(error, '管理员登录失败'))
      },
    },
    providerEndpoints: {
      providerConfig: {
        accountLimitsFallback(error) {
          appToast.warning(extractSafeApiErrorMessage(error, '账号限制加载失败，已使用默认限制'))
        },
        loadFailed(error) {
          appToast.error(extractSafeApiErrorMessage(error, '加载数据失败'))
        },
        staleProviderEndpoint() {
          appToast.warning('供应商端点已刷新，请重新选择')
        },
        staleAccount() {
          appToast.warning('账号已刷新，请重新选择')
        },
        staleModel() {
          appToast.warning('模型已刷新，请重新选择')
        },
        providerEndpointFormInvalid() {
          appToast.warning('请修正供应商端点表单错误')
        },
        accountFormInvalid() {
          appToast.warning('请修正 APIKey 表单错误')
        },
        providerEndpointSaved(isEditing) {
          appToast.success(isEditing ? '供应商端点已更新' : '供应商端点已创建')
        },
        providerEndpointSaveFailed(error) {
          appToast.error(extractSafeApiErrorMessage(error, '保存供应商端点失败'))
        },
        providerEndpointDeleted() {
          appToast.success('删除成功')
        },
        providerEndpointDeleteFailed(error) {
          appToast.error(extractSafeApiErrorMessage(error, '删除供应商端点失败'))
        },
        accountSaved(isEditing) {
          appToast.success(isEditing ? '账号已更新' : '账号已创建')
        },
        accountSaveFailed(error) {
          appToast.error(extractSafeApiErrorMessage(error, '保存账号失败'))
        },
        accountDeleted() {
          appToast.success('账号已删除')
        },
        accountDeleteFailed(error) {
          appToast.error(extractSafeApiErrorMessage(error, '删除账号失败'))
        },
        modelStatusChanged(enabled) {
          appToast.success(enabled ? '模型已启动' : '模型已停止')
        },
        modelStatusChangeFailed(error) {
          appToast.error(extractSafeApiErrorMessage(error, '模型状态修改失败'))
        },
        missingApiKeyForModelDetection() {
          appToast.warning('请输入 API 密钥后再检测模型')
        },
        modelDetectionFailed(error) {
          appToast.warning(extractSafeApiErrorMessage(error, '自动检测模型失败'))
        },
      },
    },
    users: {
      management: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载用户失败'))
        },
        adminStatusChangeForbidden() {
          appToast.warning('管理员账号不可封禁或解封')
        },
        unbanSucceeded() {
          appToast.success('已解除封禁')
        },
        unbanFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '解封失败'))
        },
        banSucceeded(username) {
          appToast.success(`已高级封禁用户 ${username}`)
        },
        banFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '封禁失败'))
        },
        missingAuthUserForSessionRevoke() {
          appToast.warning('目标用户缺少 Auth 标识，无法吊销会话')
        },
        revokeSessionsSucceeded() {
          appToast.success('已强制吊销所有会话')
        },
        revokeSessionsFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '操作失败'))
        },
        missingAuthUserForImpersonation() {
          appToast.error('目标用户缺少 Auth 标识，无法伪装登录')
        },
        impersonationStarted(username) {
          appToast.success(`正在以 ${username} 的身份登录...`)
        },
        impersonationFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '伪装失败'))
        },
        userDeleted() {
          appToast.success('用户已受控彻底注销')
        },
        userDeleteFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '注销失败'))
        },
        roleUpdated(nextRole) {
          appToast.success(nextRole === 'admin' ? '已设为管理员' : '已取消管理员')
        },
        roleUpdateFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '角色更新失败'))
        },
      },
    },
    promptConfig: {
      loadPromptsFailed(error) {
        appToast.error(formatDisplayErrorMessage(error, '加载提示词失败'))
      },
      promptDraftInvalid(message) {
        appToast.warning(message)
      },
      promptSaved(featureTitle) {
        appToast.success(`「${featureTitle}」提示词已保存`)
      },
      savePromptFailed(error) {
        appToast.error(formatDisplayErrorMessage(error, '保存失败'))
      },
    },
    systemSettings: {
      branding: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载系统设置失败'))
        },
        validationFailed(message) {
          appToast.error(message)
        },
        saved() {
          appToast.success('项目品牌已保存')
        },
        saveFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '保存项目品牌失败'))
        },
        logoUploaded() {
          appToast.success('Logo 已上传，保存后生效')
        },
        logoUploadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '上传 Logo 失败'))
        },
      },
      access: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载访问控制设置失败'))
        },
        rateLimitValidationFailed(message) {
          appToast.error(message)
        },
        rateLimitSaved() {
          appToast.success('访问限流已保存')
        },
        rateLimitSaveFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '保存失败'))
        },
        registrationUpdated(enabled) {
          appToast.success(enabled ? '已开放注册' : '已关闭注册')
        },
        registrationUpdateFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '更新注册状态失败'))
        },
      },
      taskMonitor: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载任务监控设置失败'))
        },
        validationFailed(message) {
          appToast.error(message)
        },
        saved() {
          appToast.success('任务监控设置已保存')
        },
        saveFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '保存任务监控设置失败'))
        },
      },
      publicURL: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载链接与域名失败'))
        },
        validationFailed(message) {
          appToast.error(message)
        },
        saved() {
          appToast.success('链接与域名已保存')
        },
        saveFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '保存链接与域名失败'))
        },
      },
      mediaUpload: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载上传设置失败'))
        },
        validationFailed(message) {
          appToast.error(message)
        },
        saved() {
          appToast.success('上传设置已保存')
        },
        saveFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '保存上传设置失败'))
        },
      },
      googleNativeGeneration: {
        loadFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '加载生成策略灰度失败'))
        },
        validationFailed(message) {
          appToast.error(message)
        },
        saved() {
          appToast.success('生成策略灰度已保存')
        },
        saveFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '保存生成策略灰度失败'))
        },
      },
      unsupportedLogoFile() {
        appToast.error(unsupportedUploadImageMessage())
      },
    },
    taskMonitor: {
      resultDetail: {
        unsafeImageUrlRejected() {
          appToast.error('图片链接不安全，已拒绝打开')
        },
        copiedToClipboard() {
          appToast.success('已复制到剪贴板')
        },
        copyFailed() {
          appToast.error('复制失败')
        },
      },
      taskDeletion: {
        noTasksSelected() {
          appToast.warning('请选择要删除的任务')
        },
        tasksDeleted(deletedCount) {
          if (deletedCount <= 0) {
            appToast.warning('没有可删除的任务，列表已刷新')
            return
          }
          appToast.success(`已删除 ${deletedCount} 个任务`)
        },
        deleteTasksFailed(error) {
          appToast.error(formatDisplayErrorMessage(error, '删除任务失败'))
        },
      },
    },
    creditsBilling: {
      operations: {
        walletLoadFailed(error) {
          appToast.error(formatCreditsBillingError(error, '加载 Wallet 失败，请稍后重试'))
        },
        ledgerLoadFailed(error) {
          appToast.error(formatCreditsBillingError(error, '加载点子流水失败，请稍后重试'))
        },
        commandSucceeded(label, replayed) {
          appToast.success(replayed ? `${label}已安全重放，未重复记账` : `${label}成功`)
        },
        commandFailed(error, label) {
          appToast.error(formatCreditsBillingError(error, `${label}失败，请刷新后重试`))
        },
        reconciliationLoadFailed(error) {
          appToast.error(formatCreditsBillingError(error, '加载对账报告失败，请稍后重试'))
        },
        reconciliationStarted() {
          appToast.success('对账任务已提交')
        },
        reconciliationStartFailed(error) {
          appToast.error(formatCreditsBillingError(error, '启动对账失败，请稍后重试'))
        },
      },
      pricing: {
        success(message) {
          appToast.success(message)
        },
        error(error, fallback) {
          appToast.error(extractSafeApiErrorMessage(error, fallback))
        },
      },
    },
  }
}

function formatCreditsBillingError(error: unknown, fallback: string) {
  if (error && typeof error === 'object') {
    const candidate = error as { status?: unknown; code?: unknown }
    if (candidate.status === 409 || candidate.code === 409) {
      return '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。'
    }
  }
  return extractSafeApiErrorMessage(error, fallback)
}

export function useAdminFeedback() {
  return createAdminFeedback(useAppToast())
}
