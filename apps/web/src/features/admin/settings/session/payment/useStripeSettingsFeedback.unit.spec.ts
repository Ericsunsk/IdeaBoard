import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStripeSettingsFeedback } from './useStripeSettingsFeedback'

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../../../shared/session/feedback/toast')>()
  return { ...mod, useAdminToast: () => toast }
})

beforeEach(() => {
  vi.clearAllMocks()
})

// 对称约定（以 settings useSettingsFeedback 为准）：每个成功 toast 都有失败 toast 对应，
// 失败走 formatAdminDisplayError + 固定 fallback，不透出内部错误与 secret。
describe('Stripe settings feedback', () => {
  it('covers save/load success-failure pairs with safe fallbacks', () => {
    const feedback = useStripeSettingsFeedback()

    feedback.saved()
    expect(toast.success.mock.calls.map(([message]) => message)).toEqual(['Stripe 设置已保存'])

    // 内部错误（dial tcp）收敛到 fallback，不透出底层细节。
    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadFailed(internalError)
    feedback.saveFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '加载 Stripe 设置失败，请重试',
      '保存 Stripe 设置失败，请重试',
    ])

    // 含 secret 的报错同样收敛到 fallback。
    feedback.saveFailed(new Error('api-key: sk_live_abc123'))
    expect(toast.error).toHaveBeenLastCalledWith('保存 Stripe 设置失败，请重试')
  })
})
