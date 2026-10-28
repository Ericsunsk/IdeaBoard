import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCreditHealthFeedback } from './useCreditHealthFeedback'

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

describe('Credit health feedback', () => {
  it('routes health/queue/history load failures to toast with safe fallbacks', () => {
    const feedback = useCreditHealthFeedback()

    feedback.healthLoadFailed(new Error('unavailable'))
    feedback.healthLoadFailed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.queueLoadFailed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.runHistoryLoadFailed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.runDetailFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      'unavailable',
      '数据加载失败，请重试。',
      '异常队列加载失败。',
      '检查记录加载失败。',
      'Run 详情加载失败。',
    ])
  })
})
