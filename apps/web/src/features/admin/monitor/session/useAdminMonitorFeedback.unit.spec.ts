import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminTaskLoadFeedback } from './useAdminMonitorFeedback'

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

describe('Admin task load feedback', () => {
  it('routes snapshot/list/detail load failures to toast with safe fallbacks', () => {
    const feedback = useAdminTaskLoadFeedback()

    feedback.snapshotLoadFailed(new Error('snapshot unavailable'))
    feedback.listLoadFailed(new Error('dial tcp 127.0.0.1:5432'))
    feedback.detailLoadFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      'snapshot unavailable',
      '加载任务列表失败',
      '加载任务详情失败',
    ])
  })
})
