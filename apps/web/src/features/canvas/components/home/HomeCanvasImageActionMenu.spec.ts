import { describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { ImageStrengthAction } from '../../execution'
import HomeCanvasImageActionMenu from './HomeCanvasImageActionMenu.vue'
import { render, screen, waitFor, within } from '../../../../test/render'

function renderImageActionMenu(
  input: {
    activeStrengthAction?: ImageStrengthAction | null
    showStrengthControl?: boolean
    strengthValue?: number
  } = {},
) {
  const props = {
    showStrengthControl: input.showStrengthControl ?? false,
    directGenerating: false,
    activeStrengthAction: input.activeStrengthAction ?? null,
    getStrengthValue: () => input.strengthValue ?? 50,
    setStrengthRangeRef: vi.fn(),
    setStrengthTriggerRef: vi.fn(),
    onStrengthRangePointerDown: vi.fn(),
    onStrengthInput: vi.fn(),
    onStrengthChange: vi.fn(),
    onStrengthPointerUp: vi.fn(),
    onStrengthClick: vi.fn(),
  }

  return {
    props,
    ...render(HomeCanvasImageActionMenu, { props }),
  }
}

describe('HomeCanvasImageActionMenu', () => {
  it('keeps inactive strength ranges outside keyboard navigation', () => {
    renderImageActionMenu()

    const styleVariationRange = screen.getByLabelText('风格延展强度')
    const hdRange = screen.getByLabelText('HD强度')

    expect(styleVariationRange).toHaveAttribute('tabindex', '-1')
    expect(styleVariationRange).toHaveAttribute('aria-hidden', 'true')
    expect(hdRange).toHaveAttribute('tabindex', '-1')
    expect(hdRange).toHaveAttribute('aria-hidden', 'true')
  })

  it('binds long press triggers to the visible strength buttons', async () => {
    const { props } = renderImageActionMenu()

    await waitFor(() => {
      expect(props.setStrengthTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.styleVariation,
        expect.anything(),
      )
      expect(props.setStrengthTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.hdUpscale,
        expect.anything(),
      )
    })
  })

  it('shows the active strength range for style variation and HD actions', () => {
    const styleRender = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.styleVariation,
    })
    const styleMenu = within(styleRender.container)
    const activeStyleRange = styleMenu.getByRole('slider', { name: '风格延展强度' })
    const hiddenHdRange = styleMenu.getByLabelText('HD强度')

    expect(activeStyleRange).not.toHaveAttribute('aria-hidden')
    expect(activeStyleRange.closest('.image-action-control')).toHaveClass('is-strength-active')
    expect(hiddenHdRange).toHaveAttribute('aria-hidden', 'true')
    expect(hiddenHdRange.closest('.image-action-control')).toHaveClass('is-strength-hidden')

    styleRender.unmount()

    const hdRender = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.hdUpscale,
    })
    const activeHdRange = within(hdRender.container).getByRole('slider', { name: 'HD强度' })

    expect(activeHdRange).not.toHaveAttribute('aria-hidden')
    expect(activeHdRange.closest('.image-action-control')).toHaveClass('is-strength-active')
  })
})
