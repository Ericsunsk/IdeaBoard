import { describe, expect, it, vi } from 'vitest'
import { createCanvasFeedback } from './useCanvasFeedback'

describe('createCanvasFeedback', () => {
  it('deduplicates insufficient-credits toasts for six seconds and opens recharge without retrying', () => {
    const appToast = {
      success: vi.fn(),
      warning: vi.fn().mockReturnValue('canvas-insufficient-credits'),
      error: vi.fn(),
      loading: vi.fn(),
      dismiss: vi.fn(),
    }
    const openRecharge = vi.fn()
    const feedback = createCanvasFeedback(appToast, openRecharge)

    feedback.taskExecution.insufficientCredits()
    feedback.taskExecution.insufficientCredits()

    expect(appToast.warning).toHaveBeenCalledTimes(2)
    expect(appToast.warning).toHaveBeenLastCalledWith(
      'You need more Ideas to run this action.',
      expect.objectContaining({
        id: 'canvas-insufficient-credits',
        duration: 6000,
        action: expect.objectContaining({ label: 'Top up now' }),
      }),
    )

    const options = appToast.warning.mock.calls.at(-1)?.[1]
    options.action.onClick()

    expect(openRecharge).toHaveBeenCalledTimes(1)
    expect(appToast.dismiss).toHaveBeenCalledWith('canvas-insufficient-credits')
  })
})
