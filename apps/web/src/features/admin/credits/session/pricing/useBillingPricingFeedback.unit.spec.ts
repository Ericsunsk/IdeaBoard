import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useBillingPricingFeedback } from './useBillingPricingFeedback'

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

describe('Billing pricing feedback', () => {
  it('joins failed tabs and formatted details without re-formatting', () => {
    const feedback = useBillingPricingFeedback()

    feedback.combinedSaveFailed(['定价设置', 'Model'], ['发布价格失败，请重试', '保存模型池失败，请重试'])
    expect(toast.error).toHaveBeenCalledWith(
      '部分配置保存失败，请重试（定价设置、Model）：发布价格失败，请重试；保存模型池失败，请重试',
    )
  })

  it('omits the detail suffix when no details are present', () => {
    const feedback = useBillingPricingFeedback()

    feedback.combinedSaveFailed(['定价设置'], [])
    expect(toast.error).toHaveBeenCalledWith('部分配置保存失败，请重试（定价设置）')
  })
})
