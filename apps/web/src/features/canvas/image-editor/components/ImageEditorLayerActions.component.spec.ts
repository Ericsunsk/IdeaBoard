import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageEditorLayerActions from './ImageEditorLayerActions.vue'
import type { ImageAdjustmentFinalApplyProgress } from '../runtime/imageAdjustmentFinalApplyContext'
import { renderWithUser, screen } from '../../../../test/render'

function renderLayerActions(
  options: {
    applying?: boolean
    applyDisabled?: boolean
    applyProgress?: ImageAdjustmentFinalApplyProgress | null
    onCancelApply?: () => void
  } = {},
) {
  return renderWithUser(
    ImageEditorLayerActions,
    {
      props: {
        canCopyLayer: true,
        canMergeLayer: true,
        canRemoveLayer: true,
        applying: options.applying ?? false,
        applyDisabled: options.applyDisabled ?? options.applying ?? false,
        applyProgress: options.applyProgress ?? null,
        'onCancel-apply': options.onCancelApply,
      },
    },
    { advanceTimers: vi.advanceTimersByTime },
  )
}

describe('ImageEditorLayerActions', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows final apply progress after a short disclosure delay and emits cancel', async () => {
    const onCancelApply = vi.fn()
    const { user } = renderLayerActions({
      applying: true,
      applyProgress: { phase: 'render', loaded: 25, total: 100 },
      onCancelApply,
    })

    expect(screen.queryByRole('status')).not.toBeInTheDocument()

    vi.advanceTimersByTime(199)
    expect(screen.queryByRole('status')).not.toBeInTheDocument()

    vi.advanceTimersByTime(1)

    expect(await screen.findByRole('status')).toHaveTextContent('Rendering adjustments')
    expect(screen.getByRole('status')).toHaveTextContent('25%')

    await user.click(screen.getByRole('button', { name: 'Cancel apply' }))

    expect(onCancelApply).toHaveBeenCalledTimes(1)
  })

  it('does not flash final apply progress for quick apply tasks', async () => {
    const { rerender } = renderLayerActions({
      applying: true,
      applyProgress: { phase: 'render', loaded: 25, total: 100 },
    })

    vi.advanceTimersByTime(199)
    await rerender({ applying: false, applyDisabled: false })
    vi.advanceTimersByTime(1)

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('hides final apply progress when idle', () => {
    renderLayerActions()

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Cancel apply' })).not.toBeInTheDocument()
  })
})
