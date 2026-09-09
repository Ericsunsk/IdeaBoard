import { formatAdminApiError, type AdminToastPort, useAdminToast } from '../shared/lib/toast'

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

function createAdminProviderConfigFeedback(toast: AdminToastPort): AdminProviderConfigFeedback {
  return {
    accountLimitsFallback: (error) =>
      toast.warning(formatAdminApiError(error, '账号限制加载失败，已使用默认限制')),
    loadFailed: (error) => toast.error(formatAdminApiError(error, '加载数据失败')),
    staleProviderEndpoint: () => toast.warning('供应商端点已刷新，请重新选择'),
    staleAccount: () => toast.warning('账号已刷新，请重新选择'),
    staleModel: () => toast.warning('模型已刷新，请重新选择'),
    providerEndpointFormInvalid: () => toast.warning('请修正供应商端点表单错误'),
    accountFormInvalid: () => toast.warning('请修正 APIKey 表单错误'),
    providerEndpointSaved: (isEditing) =>
      toast.success(isEditing ? '供应商端点已更新' : '供应商端点已创建'),
    providerEndpointSaveFailed: (error) =>
      toast.error(formatAdminApiError(error, '保存供应商端点失败')),
    providerEndpointDeleted: () => toast.success('删除成功'),
    providerEndpointDeleteFailed: (error) =>
      toast.error(formatAdminApiError(error, '删除供应商端点失败')),
    accountSaved: (isEditing) => toast.success(isEditing ? '账号已更新' : '账号已创建'),
    accountSaveFailed: (error) => toast.error(formatAdminApiError(error, '保存账号失败')),
    accountDeleted: () => toast.success('删除成功'),
    accountDeleteFailed: (error) => toast.error(formatAdminApiError(error, '删除账号失败')),
    modelStatusChanged: (enabled) => toast.success(enabled ? '模型已启动' : '模型已停止'),
    modelStatusChangeFailed: (error) => toast.error(formatAdminApiError(error, '模型状态修改失败')),
    missingApiKeyForModelDetection: () => toast.warning('请输入 API 密钥后再检测模型'),
    modelDetectionFailed: (error) => toast.warning(formatAdminApiError(error, '自动检测模型失败')),
  }
}

export function useProviderConfigFeedback() {
  return createAdminProviderConfigFeedback(useAdminToast())
}
