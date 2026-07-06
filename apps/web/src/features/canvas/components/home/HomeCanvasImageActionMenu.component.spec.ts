import { describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { ImageStrengthAction } from '../../execution'
import HomeCanvasImageActionMenu from './HomeCanvasImageActionMenu.vue'
import { render, screen, waitFor } from '../../../../test/render'

function renderImageActionMenu(
  input: {
    activeStrengthAction?: ImageStrengthAction | null
    showStrengthControl?: boolean
    strengthValue?: number
  } = {},
) {
  const props = {
    directGenerating: false,
    strengthControl: {
      visible: input.showStrengthControl ?? false,
      activeAction: input.activeStrengthAction ?? null,
      getValue: () => input.strengthValue ?? 50,
      setRangeRef: vi.fn(),
      setTriggerRef: vi.fn(),
      onRangePointerDown: vi.fn(),
      onInput: vi.fn(),
      onChange: vi.fn(),
      onPointerUp: vi.fn(),
      onClick: vi.fn(),
    },
  }

  return {
    props,
    ...render(HomeCanvasImageActionMenu, { props }),
  }
}

function findActionControl(element: HTMLElement) {
  const control = element.closest<HTMLElement>('.image-action-control')
  if (!control) throw new Error('expected image action control')
  return control
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
      expect(props.strengthControl.setTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.styleVariation,
        expect.anything(),
      )
      expect(props.strengthControl.setTriggerRef).toHaveBeenCalledWith(
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
    const activeStyleRange = styleRender.getByRole('slider', { name: '风格延展强度' })
    const hiddenHdRange = styleRender.getByLabelText('HD强度')

    expect(activeStyleRange).not.toHaveAttribute('aria-hidden')
    expect(findActionControl(activeStyleRange)).toHaveClass('is-strength-active')
    expect(hiddenHdRange).toHaveAttribute('aria-hidden', 'true')
    expect(findActionControl(hiddenHdRange)).toHaveClass('is-strength-hidden')

    styleRender.unmount()

    const hdRender = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.hdUpscale,
    })
    const activeHdRange = hdRender.getByRole('slider', { name: 'HD强度' })

    expect(activeHdRange).not.toHaveAttribute('aria-hidden')
    expect(findActionControl(activeHdRange)).toHaveClass('is-strength-active')
  })
})
