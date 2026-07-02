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
    feedback.channels.providerConfig.staleChannel()
    feedback.channels.providerConfig.channelSaved(true)
    feedback.channels.providerConfig.modelStatusChanged(false)
    feedback.users.management.banSucceeded('alice')
    feedback.systemSettings.branding.saveFailed(new Error('品牌设置失败'))
    feedback.taskMonitor.resultDetail.copiedToClipboard()

    expect(appToast.success).toHaveBeenNthCalledWith(1, '管理员登录成功')
    expect(appToast.warning).toHaveBeenNthCalledWith(1, '会话撤销失败，请稍后重试')
    expect(appToast.error).toHaveBeenNthCalledWith(1, '该账号不是管理员')
    expect(appToast.error).toHaveBeenNthCalledWith(2, '登录失败')
    expect(appToast.error).toHaveBeenNthCalledWith(3, '加载失败')
    expect(appToast.success).toHaveBeenNthCalledWith(2, '「图像生成」提示词已保存')
    expect(appToast.error).toHaveBeenNthCalledWith(4, '保存坏了')
    expect(appToast.warning).toHaveBeenNthCalledWith(2, '渠道商已刷新，请重新选择')
    expect(appToast.success).toHaveBeenNthCalledWith(3, '渠道商已更新')
    expect(appToast.success).toHaveBeenNthCalledWith(4, '模型已停止')
    expect(appToast.success).toHaveBeenNthCalledWith(5, '已高级封禁用户 alice')
    expect(appToast.error).toHaveBeenNthCalledWith(5, '品牌设置失败')
    expect(appToast.success).toHaveBeenNthCalledWith(6, '已复制到剪贴板')
  })
})
