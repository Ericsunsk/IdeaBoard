import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePromptConfigFeedback } from './usePromptConfigFeedback'

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

describe('Admin prompt config feedback', () => {
  it('covers save/load pairs with safe fallbacks and validation passthrough', () => {
    const feedback = usePromptConfigFeedback()

    feedback.promptSaved('作图')
    expect(toast.success).toHaveBeenCalledWith('「作图」提示词已保存')

    feedback.promptDraftInvalid('标题不能为空')
    expect(toast.warning).toHaveBeenCalledWith('标题不能为空')

    const internalError = new Error('dial tcp 127.0.0.1:5432: connection refused')
    feedback.loadPromptsFailed(internalError)
    feedback.savePromptFailed(internalError)
    expect(toast.error.mock.calls.map(([message]) => message)).toEqual([
      '加载提示词失败',
      '保存提示词失败',
    ])

    feedback.savePromptFailed(new Error('api-key: sk_live_abc123'))
    expect(toast.error).toHaveBeenLastCalledWith('保存提示词失败')
  })
})
