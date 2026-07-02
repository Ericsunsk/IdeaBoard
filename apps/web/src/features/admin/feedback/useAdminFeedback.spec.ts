import { describe, expect, it, vi } from 'vitest'
import { createAdminFeedback } from './useAdminFeedback'

describe('createAdminFeedback', () => {
  it('groups admin feedback by domain area', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createAdminFeedback(appToast)

    feedback.login.succeeded()
    feedback.login.sessionRevokeFailed()
    feedback.login.nonAdminAccount()
    feedback.login.failed(new Error('登录失败'))
    feedback.promptConfig.loadPromptsFailed(new Error('加载失败'))
    feedback.promptConfig.promptSaved('图像生成')
    feedback.promptConfig.savePromptFailed(new Error('保存坏了'))
    feedback.channels.providerConfig.warning('渠道警告')
    feedback.users.management.success('用户已更新')
    feedback.systemSettings.branding.error('品牌设置失败')
    feedback.taskMonitor.resultDetail.success('复制成功')

    expect(appToast.success).toHaveBeenNthCalledWith(1, '管理员登录成功')
    expect(appToast.warning).toHaveBeenNthCalledWith(1, '会话撤销失败，请稍后重试')
    expect(appToast.error).toHaveBeenNthCalledWith(1, '该账号不是管理员')
    expect(appToast.error).toHaveBeenNthCalledWith(2, '登录失败')
    expect(appToast.error).toHaveBeenNthCalledWith(3, '加载失败')
    expect(appToast.success).toHaveBeenNthCalledWith(2, '「图像生成」提示词已保存')
    expect(appToast.error).toHaveBeenNthCalledWith(4, '保存坏了')
    expect(appToast.warning).toHaveBeenNthCalledWith(2, '渠道警告')
    expect(appToast.success).toHaveBeenNthCalledWith(3, '用户已更新')
    expect(appToast.error).toHaveBeenNthCalledWith(5, '品牌设置失败')
    expect(appToast.success).toHaveBeenNthCalledWith(4, '复制成功')
  })
})
