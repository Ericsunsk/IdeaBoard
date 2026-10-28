import { defineFeedback } from '../../../shared/index'

export type AdminStorageFeedback = {
  loadProfilesFailed: (error: unknown) => void
  refreshFailed: (error: unknown) => void
  staleProfile: () => void
  unsupportedVendorPreset: (label: string) => void
  noCreatableProviders: () => void
  missingEditableTarget: () => void
  saveDraftSucceeded: (message?: string) => void
  saveDraftFailed: (error: unknown) => void
  applyCurrentSucceeded: (message?: string) => void
  applyCurrentFailed: (error: unknown) => void
  testConnectionSucceeded: (message?: string) => void
  testConnectionFailed: (error: unknown) => void
  bucketDomainsDetected: (count: number) => void
  bucketDomainsEmpty: () => void
  fetchBucketDomainsFailed: (error: unknown) => void
  profileDeleted: (message?: string) => void
  deleteProfileFailed: (error: unknown) => void
}

const ADMIN_STORAGE_COPY = {
  loadProfilesFailed: { error: '加载存储配置失败' },
  refreshFailed: { error: '刷新存储配置失败' },
  staleProfile: { warning: '存储配置已变化，请刷新后重试' },
  unsupportedVendorPreset: { info: (label: string) => `${label} 暂未接入后端，当前先作为预设展示` },
  noCreatableProviders: { warning: '当前没有可新增的存储类型' },
  missingEditableTarget: { errorText: '未找到可编辑的存储配置' },
  saveDraftSucceeded: { successMessage: '保存成功' },
  saveDraftFailed: { error: '保存失败' },
  applyCurrentSucceeded: { successMessage: '已设为当前存储' },
  applyCurrentFailed: { error: '设为当前失败' },
  testConnectionSucceeded: { successMessage: '连接成功' },
  testConnectionFailed: { error: '连接失败' },
  bucketDomainsDetected: { success: (count: number) => `检测到 ${count} 个域名` },
  bucketDomainsEmpty: { warning: '未检测到绑定域名' },
  fetchBucketDomainsFailed: { error: '获取失败' },
  profileDeleted: { successMessage: '删除成功' },
  deleteProfileFailed: { error: '删除失败' },
}

export function useStorageFeedback(): AdminStorageFeedback {
  return defineFeedback<AdminStorageFeedback>(ADMIN_STORAGE_COPY)
}
