import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useUsageStatisticsFeedback } from './useUsageStatisticsFeedback'

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

describe('Usage statistics feedback', () => {
  it('routes statistics load failures to toast with a safe fallback', () => {
    const feedback = useUsageStatisticsFeedback()

    feedback.loadFailed(new Error('统计查询失败'))
    feedback.loadFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '统计查询失败',
      '加载统计数据失败',
    ])
  })
})
