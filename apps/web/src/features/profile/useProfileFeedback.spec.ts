import { describe, expect, it, vi } from 'vitest'
import { createProfileFeedback } from './useProfileFeedback'

describe('createProfileFeedback', () => {
  it('groups avatar and password feedback messages', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
    }
    const feedback = createProfileFeedback(appToast)

    feedback.invalidAvatarFile()
    feedback.avatarUpload.success?.()
    feedback.avatarUpload.error?.(new Error('头像服务繁忙'))
    feedback.passwordChange.success?.(true)
    feedback.passwordChange.error?.(new Error('密码已更新，但退出登录失败，请手动退出后重新登录'))

    expect(appToast.warning).toHaveBeenCalledWith('仅支持上传图片文件')
    expect(appToast.success).toHaveBeenNthCalledWith(1, '头像已更新')
    expect(appToast.success).toHaveBeenNthCalledWith(2, '密码更新成功，请重新登录')
    expect(appToast.error).toHaveBeenNthCalledWith(1, '头像服务繁忙')
    expect(appToast.error).toHaveBeenNthCalledWith(
      2,
      '密码已更新，但退出登录失败，请手动退出后重新登录',
    )
  })
})
