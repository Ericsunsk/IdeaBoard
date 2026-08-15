import { describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { ImageStrengthAction } from '../../execution'
import HomeCanvasImageActionMenu from './HomeCanvasImageActionMenu.vue'
import { render, screen, waitFor } from '../../../../test/render'
import { canvasBillingReadinessKey, type CanvasBillingReadinessStore } from '../../execution'

function renderImageActionMenu(
  input: {
    activeStrengthAction?: ImageStrengthAction | null
    showStrengthControl?: boolean
    strengthValue?: number
    billingStore?: CanvasBillingReadinessStore
    outputCount?: number
  } = {},
) {
  const props = {
    outputCount: input.outputCount ?? 1,
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
    ...render(HomeCanvasImageActionMenu, {
      props,
      global: {
        provide: input.billingStore
          ? { [canvasBillingReadinessKey as symbol]: input.billingStore }
          : {},
      },
    }),
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

  it('shows AI prices without disabling cached unavailable billing states, and keeps local editor price-free', () => {
    const billingStore: CanvasBillingReadinessStore = {
      ensureFeature: vi.fn(async () => undefined),
      applyReadiness: vi.fn(),
      getFeatureState: (feature) =>
        feature === IMAGE_NODE_ACTION.removeBg
          ? { feature, status: 'blocked', reason: '价格未配置' }
          : { feature, status: 'ready', unitCredits: 3, availableCredits: 20 },
    }

    renderImageActionMenu({ billingStore, outputCount: 4 })

    expect(screen.getByRole('button', { name: '抠图' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '风格延展 · 12 Ideas' })).toBeEnabled()
    expect(screen.getAllByText('12 Ideas')[0]).toHaveAttribute('data-touch-quote')
    expect(screen.getByRole('button', { name: '图像编辑' })).toBeEnabled()
    expect(screen.getByRole('button', { name: '局部重绘 · 12 Ideas' })).toBeEnabled()
  })

  it('presents explicit zero pricing as free in tooltip and touch quote', () => {
    const billingStore: CanvasBillingReadinessStore = {
      ensureFeature: vi.fn(async () => undefined),
      applyReadiness: vi.fn(),
      getFeatureState: (feature) => ({
        feature,
        status: 'ready',
        unitCredits: 0,
        availableCredits: 0,
        sufficient: true,
      }),
    }

    renderImageActionMenu({ billingStore })

    expect(screen.getByRole('button', { name: '抠图 · 免费' })).toBeEnabled()
    expect(screen.getAllByText('免费')[0]).toHaveAttribute('data-touch-quote')
  })

  it('submits a strength action once when the committed value changes', () => {
    const view = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.styleVariation,
    })
    const range = screen.getByRole('slider', { name: '风格延展强度' })

    Object.defineProperty(range, 'value', { configurable: true, value: '72' })
    range.dispatchEvent(new Event('input', { bubbles: true }))
    range.dispatchEvent(new Event('pointerup', { bubbles: true }))
    range.dispatchEvent(new Event('click', { bubbles: true }))
    range.dispatchEvent(new Event('change', { bubbles: true }))

    expect(view.props.strengthControl.onInput).toHaveBeenCalledOnce()
    expect(view.props.strengthControl.onPointerUp).toHaveBeenCalledOnce()
    expect(view.props.strengthControl.onChange).toHaveBeenCalledOnce()
    expect(view.emitted('action')).toEqual([[IMAGE_NODE_ACTION.styleVariation, 72]])
  })
})
