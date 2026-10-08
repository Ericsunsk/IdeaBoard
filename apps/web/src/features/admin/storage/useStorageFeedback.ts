import { formatAdminDisplayError, type AdminToastPort, useAdminToast } from '../shared/lib/toast'

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

function createAdminStorageFeedback(toast: AdminToastPort): AdminStorageFeedback {
  return {
    loadProfilesFailed(error) {
      toast.error(formatAdminDisplayError(error, '加载存储配置失败'))
    },
    refreshFailed(error) {
      toast.error(formatAdminDisplayError(error, '刷新存储配置失败'))
    },
    staleProfile() {
      toast.warning('存储配置已变化，请刷新后重试')
    },
    unsupportedVendorPreset(label) {
      toast.info(`${label} 暂未接入后端，当前先作为预设展示`)
    },
    noCreatableProviders() {
      toast.warning('当前没有可新增的存储类型')
    },
    missingEditableTarget() {
      toast.error('未找到可编辑的存储配置')
    },
    saveDraftSucceeded(message) {
      toast.success(message || '保存成功')
    },
    saveDraftFailed(error) {
      toast.error(formatAdminDisplayError(error, '保存失败'))
    },
    applyCurrentSucceeded(message) {
      toast.success(message || '已设为当前存储')
    },
    applyCurrentFailed(error) {
      toast.error(formatAdminDisplayError(error, '设为当前失败'))
    },
    testConnectionSucceeded(message) {
      toast.success(message || '连接成功')
    },
    testConnectionFailed(error) {
      toast.error(formatAdminDisplayError(error, '连接失败'))
    },
    bucketDomainsDetected(count) {
      toast.success(`检测到 ${count} 个域名`)
    },
    bucketDomainsEmpty() {
      toast.warning('未检测到绑定域名')
    },
    fetchBucketDomainsFailed(error) {
      toast.error(formatAdminDisplayError(error, '获取失败'))
    },
    profileDeleted(message) {
      toast.success(message || '删除成功')
    },
    deleteProfileFailed(error) {
      toast.error(formatAdminDisplayError(error, '删除失败'))
    },
  }
}

export function useStorageFeedback() {
  return createAdminStorageFeedback(useAdminToast())
}
