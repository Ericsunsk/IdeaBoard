import { describe, expect, it, vi } from 'vitest'
import { createAdminUsersFeedback } from './useAdminUsersFeedback'

describe('Admin Users feedback', () => {
  it('keeps permission-sensitive failures explicit and safe', () => {
    const toast = {
      success: vi.fn(),
      warning: vi.fn(),
      error: vi.fn(),
      info: vi.fn(),
    }
    const feedback = createAdminUsersFeedback(toast)

    feedback.adminStatusChangeForbidden()
    feedback.missingAuthUserForSessionRevoke()
    feedback.missingAuthUserForImpersonation()
    feedback.loadFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.warning.mock.calls.map(([message]) => message)).toEqual([
      '管理员账号不可封禁或解封',
      '目标用户缺少 Auth 标识，无法吊销会话',
    ])
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '目标用户缺少 Auth 标识，无法伪装登录',
      '加载用户失败',
    ])
  })
})
