import { describe, expect, it, vi } from 'vitest'
import { createAuthFeedback } from './useAuthFeedback'

describe('createAuthFeedback', () => {
  it('groups account success and failure messages', () => {
    const appToast = {
      success: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createAuthFeedback(appToast)

    feedback.login.success?.(true)
    feedback.login.error?.(new Error('账号或密码错误'))
    feedback.register.success?.(true)
    feedback.register.error?.(new Error('注册失败'))
    feedback.stopImpersonating.success?.(true)
    feedback.stopImpersonating.error?.({})

    expect(appToast.success).toHaveBeenNthCalledWith(1, '登录成功')
    expect(appToast.success).toHaveBeenNthCalledWith(2, '注册成功')
    expect(appToast.success).toHaveBeenNthCalledWith(3, '已退出伪装身份')
    expect(appToast.error).toHaveBeenNthCalledWith(1, '账号或密码错误')
    expect(appToast.error).toHaveBeenNthCalledWith(2, '注册失败')
    expect(appToast.error).toHaveBeenNthCalledWith(3, '退出伪装失败，请重新登录管理员账号')
  })
})
