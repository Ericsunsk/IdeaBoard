import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useRewardCodesFeedback } from './useRewardCodesFeedback'

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

describe('Reward codes feedback', () => {
  it('routes save/list/detail failures to toast with safe fallbacks', () => {
    const feedback = useRewardCodesFeedback()

    feedback.saveFailed(new Error('创建冲突'), false)
    feedback.saveFailed(new Error('dial tcp 127.0.0.1:5432'), true)
    feedback.redemptionsLoadFailed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.redemptionDetailFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '创建冲突',
      '更新兑换码失败',
      '兑换记录加载失败，请重试。',
      '兑换详情加载失败。',
    ])
  })
})
