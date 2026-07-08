import { useAppToast } from '../../../shared/composables/useAppToast'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'

type AdminStorageToast = Pick<
  ReturnType<typeof useAppToast>,
  'success' | 'warning' | 'error' | 'info'
>

export type AdminStorageFeedback = {
  loadProfilesFailed: (error: unknown) => void
  refreshFailed: (error: unknown) => void
  staleProfile: () => void
  unsupportedVendorPreset: (label: string) => void
  noCreatableProviders: () => void
  missingEditableTarget: () => void
  saveDraftSucceeded: (message?: string) => void
  saveDraftFailed: (error: unknown) => void
  saveAndApplySucceeded: () => void
  saveAndApplyFailed: (error: unknown) => void
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

export function createAdminStorageFeedback(appToast: AdminStorageToast): AdminStorageFeedback {
  return {
    loadProfilesFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '加载存储配置失败'))
    },
    refreshFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '刷新存储配置失败'))
    },
    staleProfile() {
      appToast.warning('存储配置已变化，请刷新后重试')
    },
    unsupportedVendorPreset(label) {
      appToast.info(`${label} 暂未接入后端，当前先作为预设展示`)
    },
    noCreatableProviders() {
      appToast.warning('当前没有可新增的存储类型')
    },
    missingEditableTarget() {
      appToast.error('未找到可编辑的存储配置')
    },
    saveDraftSucceeded(message) {
      appToast.success(message || '保存成功')
    },
    saveDraftFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '保存失败'))
    },
    saveAndApplySucceeded() {
      appToast.success('已保存并设为当前存储')
    },
    saveAndApplyFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '设为当前失败'))
    },
    applyCurrentSucceeded(message) {
      appToast.success(message || '已设为当前存储')
    },
    applyCurrentFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '设为当前失败'))
    },
    testConnectionSucceeded(message) {
      appToast.success(message || '连接成功')
    },
    testConnectionFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '连接失败'))
    },
    bucketDomainsDetected(count) {
      appToast.success(`检测到 ${count} 个域名`)
    },
    bucketDomainsEmpty() {
      appToast.warning('未检测到绑定域名')
    },
    fetchBucketDomainsFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '获取失败'))
    },
    profileDeleted(message) {
      appToast.success(message || '删除成功')
    },
    deleteProfileFailed(error) {
      appToast.error(formatDisplayErrorMessage(error, '删除失败'))
    },
  }
}

export function useAdminStorageFeedback() {
  return createAdminStorageFeedback(useAppToast())
}
