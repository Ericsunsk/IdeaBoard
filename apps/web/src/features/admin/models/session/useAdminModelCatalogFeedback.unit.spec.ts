import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAdminModelCatalogFeedback } from './useAdminModelCatalogFeedback'

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

describe('Admin model catalog feedback additions', () => {
  it('routes catalog list failures to toast with safe fallbacks', () => {
    const feedback = useAdminModelCatalogFeedback()

    feedback.listFailed(new Error('catalog down'))
    feedback.listFailed(new Error('dial tcp 127.0.0.1:5432'))

    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      'catalog down',
      '加载模型目录失败',
    ])
  })
})
