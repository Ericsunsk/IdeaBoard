import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useUsersFeedback } from './useUsersFeedback'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('../../shared/session/feedback/toast', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../../shared/session/feedback/toast')>()
  return { ...mod, useAdminToast: () => toast }
})

beforeEach(() => {
  vi.clearAllMocks()
})

// 对称约定：每个成功 toast 都有失败 toast 对应，失败走 formatAdminDisplayError +
// 固定 fallback，不透出内部错误与 secret。
describe('Admin users feedback', () => {
  it('keeps permission-sensitive failures explicit and safe', () => {
    const feedback = useUsersFeedback()

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

  it('covers ban/unban success-failure pairs with safe fallbacks', () => {
    const feedback = useUsersFeedback()

    feedback.unbanSucceeded()
    feedback.banSucceeded('Ada')
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      '已解除封禁',
      '已高级封禁用户 Ada',
    ])

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.unbanFailed(internalError)
    feedback.banFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual(['解封失败', '封禁失败'])

    feedback.banFailed(new Error('api-key: sk_live_abc123'))
    expect(toast.error).toHaveBeenLastCalledWith('封禁失败')
  })

  it('covers revoke/impersonate/delete/role pairs with safe fallbacks', () => {
    const feedback = useUsersFeedback()

    feedback.revokeSessionsSucceeded()
    feedback.impersonationStarted('Ada')
    feedback.userDeleted()
    feedback.roleUpdated('admin')
    feedback.roleUpdated('user')
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual([
      '已强制吊销所有会话',
      '正在以 Ada 的身份登录...',
      '用户已受控彻底注销',
      '已设为管理员',
      '已取消管理员',
    ])

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.revokeSessionsFailed(internalError)
    feedback.impersonationFailed(internalError)
    feedback.userDeleteFailed(internalError)
    feedback.roleUpdateFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '操作失败',
      '伪装失败',
      '注销失败',
      '角色更新失败',
    ])
  })
})
