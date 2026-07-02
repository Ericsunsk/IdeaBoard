import { describe, expect, it, vi } from 'vitest'
import { createAdminStorageFeedback } from './useAdminStorageFeedback'

describe('createAdminStorageFeedback', () => {
  it('groups storage lifecycle messages', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
      info: vi.fn(),
    }
    const feedback = createAdminStorageFeedback(appToast)

    feedback.unsupportedVendorPreset('MinIO')
    feedback.noCreatableProviders()
    feedback.missingEditableTarget()
    feedback.saveDraftSucceeded('已保存')
    feedback.saveAndApplySucceeded()
    feedback.applyCurrentSucceeded()
    feedback.testConnectionSucceeded('连接正常')
    feedback.bucketDomainsDetected(2)
    feedback.bucketDomainsEmpty()
    feedback.profileDeleted()
    feedback.refreshFailed(new Error('刷新失败'))

    expect(appToast.info).toHaveBeenCalledWith('MinIO 暂未接入后端，当前先作为预设展示')
    expect(appToast.warning).toHaveBeenNthCalledWith(1, '当前没有可新增的存储类型')
    expect(appToast.error).toHaveBeenNthCalledWith(1, '未找到可编辑的存储配置')
    expect(appToast.success).toHaveBeenNthCalledWith(1, '已保存')
    expect(appToast.success).toHaveBeenNthCalledWith(2, '已保存并设为当前存储')
    expect(appToast.success).toHaveBeenNthCalledWith(3, '已设为当前存储')
    expect(appToast.success).toHaveBeenNthCalledWith(4, '连接正常')
    expect(appToast.success).toHaveBeenNthCalledWith(5, '检测到 2 个域名')
    expect(appToast.warning).toHaveBeenNthCalledWith(2, '未检测到绑定域名')
    expect(appToast.success).toHaveBeenNthCalledWith(6, '删除成功')
    expect(appToast.error).toHaveBeenNthCalledWith(2, '刷新失败')
  })
})
