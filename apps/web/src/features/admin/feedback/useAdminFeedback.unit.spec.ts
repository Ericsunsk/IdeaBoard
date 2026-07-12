import { describe, expect, it, vi } from 'vitest'
import { createAdminFeedback } from './useAdminFeedback'

describe('createAdminFeedback', () => {
  it('keeps security and permission failures explicit', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createAdminFeedback(appToast)

    feedback.login.nonAdminAccount()
    feedback.users.management.adminStatusChangeForbidden()
    feedback.users.management.missingAuthUserForSessionRevoke()
    feedback.users.management.missingAuthUserForImpersonation()
    feedback.taskMonitor.resultDetail.unsafeImageUrlRejected()

    expect(appToast.warning.mock.calls.map(([message]) => message)).toEqual([
      '管理员账号不可封禁或解封',
      '目标用户缺少 Auth 标识，无法吊销会话',
    ])
    expect(appToast.error.mock.calls.map(([message]) => message)).toEqual([
      '该账号不是管理员',
      '目标用户缺少 Auth 标识，无法伪装登录',
      '图片链接不安全，已拒绝打开',
    ])
    expect(appToast.success).not.toHaveBeenCalled()
  })

  it('uses safe fallbacks for admin API errors', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createAdminFeedback(appToast)

    feedback.login.failed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.providerEndpoints.providerConfig.accountLimitsFallback({ msg: 'api key invalid' })
    feedback.providerEndpoints.providerConfig.loadFailed({ msg: '端点服务繁忙' })

    expect(appToast.error.mock.calls.map(([message]) => message)).toEqual([
      '管理员登录失败',
      '端点服务繁忙',
    ])
    expect(appToast.warning).toHaveBeenCalledWith('账号限制加载失败，已使用默认限制')
  })

  it('reports Credits idempotency conflicts without implying a repeated posting', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createAdminFeedback(appToast)

    feedback.creditsBilling.operations.commandFailed({ status: 409 }, '赠送')
    feedback.creditsBilling.operations.commandFailed({ code: 409 }, '退款')

    expect(appToast.error).toHaveBeenNthCalledWith(
      1,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
    expect(appToast.error).toHaveBeenNthCalledWith(
      2,
      '操作发生幂等冲突，未重复记账。请刷新 Wallet 与流水后重试。',
    )
    expect(appToast.success).not.toHaveBeenCalled()
  })
})
