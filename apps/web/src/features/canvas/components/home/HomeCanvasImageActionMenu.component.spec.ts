import { describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
import type { OutputResolution } from '../../../../shared/domain/generationContract'
import type { WorkflowFeature } from '../../../../shared/domain/workflowFeatures'
import type { SubmitTaskReadinessData } from '../../../../shared/api/task'
import type { ImageStrengthAction } from '../../execution'
import HomeCanvasImageActionMenu from './HomeCanvasImageActionMenu.vue'
import { render, screen, waitFor } from '../../../../test/render'
import { canvasBillingReadinessKey, type CanvasBillingReadinessStore } from '../../execution'
import { createCanvasBillingReadinessStore } from '../../execution/billing/useCanvasBillingReadiness'

function createDeferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })
  return { promise, resolve }
}

function makeReadiness(
  feature: WorkflowFeature,
  outputResolution: OutputResolution,
  unitCredits: number,
): SubmitTaskReadinessData {
  return {
    ready: true,
    feature,
    billing: {
      sku: `ai-image:${feature}`,
      output_resolution: outputResolution,
      display_name: feature,
      price_version_id: 1,
      price_version: 1,
      unit_credits: unitCredits,
      quantity: 1,
      total_credits: unitCredits,
      quoted_at: '2026-08-19T00:00:00.000Z',
      available_credits: 100,
      reserved_credits: 0,
      sufficient: true,
    },
  }
}

function renderImageActionMenu(
  input: {
    activeStrengthAction?: ImageStrengthAction | null
    showStrengthControl?: boolean
    strengthValue?: number
    billingStore?: CanvasBillingReadinessStore
    outputCount?: number
    outputResolution?: OutputResolution
  } = {},
) {
  const props = {
    outputCount: input.outputCount ?? 1,
    outputResolution: input.outputResolution ?? '1k',
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

    const variationsRange = screen.getByLabelText('Variations strength')
    const hdRange = screen.getByLabelText('Upscale strength')

    expect(variationsRange).toHaveAttribute('tabindex', '-1')
    expect(variationsRange).toHaveAttribute('aria-hidden', 'true')
    expect(hdRange).toHaveAttribute('tabindex', '-1')
    expect(hdRange).toHaveAttribute('aria-hidden', 'true')
  })

  it('binds long press triggers to the visible strength buttons', async () => {
    const { props } = renderImageActionMenu()

    await waitFor(() => {
      expect(props.strengthControl.setTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.variations,
        expect.anything(),
      )
      expect(props.strengthControl.setTriggerRef).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.hdUpscale,
        expect.anything(),
      )
    })
  })

  it('shows the active strength range for style variation and HD actions', () => {
    const variationsRender = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.variations,
    })
    const activeStyleRange = variationsRender.getByRole('slider', { name: 'Variations strength' })
    const hiddenHdRange = variationsRender.getByLabelText('Upscale strength')

    expect(activeStyleRange).not.toHaveAttribute('aria-hidden')
    expect(findActionControl(activeStyleRange)).toHaveClass('is-strength-active')
    expect(hiddenHdRange).toHaveAttribute('aria-hidden', 'true')
    expect(findActionControl(hiddenHdRange)).toHaveClass('is-strength-hidden')

    variationsRender.unmount()

    const hdRender = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.hdUpscale,
    })
    const activeHdRange = hdRender.getByRole('slider', { name: 'Upscale strength' })

    expect(activeHdRange).not.toHaveAttribute('aria-hidden')
    expect(activeHdRange).toHaveAttribute('min', '1')
    expect(activeHdRange).toHaveAttribute('max', '4')
    expect(activeHdRange).toHaveAttribute('step', '0.1')
    expect(findActionControl(activeHdRange)).toHaveClass('is-strength-active')
  })

  it('shows AI prices without disabling cached unavailable billing states, and keeps local editor price-free', () => {
    const billingStore: CanvasBillingReadinessStore = {
      ensureFeature: vi.fn(async () => undefined),
      applyReadiness: vi.fn(),
      getFeatureState: (feature) =>
        feature === IMAGE_NODE_ACTION.cutout
          ? { feature, status: 'blocked', reason: '价格未配置' }
          : { feature, status: 'ready', unitCredits: 3, totalCredits: 7, availableCredits: 20 },
    }

    renderImageActionMenu({ billingStore, outputCount: 4 })

    expect(screen.getByRole('button', { name: 'Cutout' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Variations · 7 Ideas' })).toBeEnabled()
    expect(screen.getAllByText('7 Ideas')[0]).toHaveAttribute('data-touch-quote')
    expect(screen.getByRole('button', { name: 'Retouch' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Inpaint · 7 Ideas' })).toBeEnabled()
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

    expect(screen.getByRole('button', { name: 'Cutout · Free' })).toBeEnabled()
    expect(screen.getAllByText('Free')[0]).toHaveAttribute('data-touch-quote')
  })

  it('requests and displays the 4K quote when resolution changes during a 2K request', async () => {
    const pending2K = createDeferred<SubmitTaskReadinessData>()
    const fetchReadiness = vi.fn(
      async (feature: WorkflowFeature, request?: { params?: unknown }) => {
        const params = request?.params as { output_resolution?: OutputResolution } | undefined
        if (feature === IMAGE_NODE_ACTION.variations && params?.output_resolution === '2k') {
          return pending2K.promise
        }
        return makeReadiness(
          feature,
          params?.output_resolution ?? '1k',
          params?.output_resolution === '4k' ? 12 : 4,
        )
      },
    )
    const billingStore = createCanvasBillingReadinessStore({ fetchReadiness })
    const view = renderImageActionMenu({ billingStore, outputResolution: '2k' })

    await waitFor(() => {
      expect(fetchReadiness).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.variations,
        expect.objectContaining({ params: expect.objectContaining({ output_resolution: '2k' }) }),
      )
    })

    await view.rerender({ outputResolution: '4k' })

    await waitFor(() => {
      expect(fetchReadiness).toHaveBeenCalledWith(
        IMAGE_NODE_ACTION.variations,
        expect.objectContaining({ params: expect.objectContaining({ output_resolution: '4k' }) }),
      )
      expect(screen.getByRole('button', { name: 'Variations · 12 Ideas' })).toBeEnabled()
    })

    pending2K.resolve(makeReadiness(IMAGE_NODE_ACTION.variations, '2k', 8))
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Variations · 12 Ideas' })).toBeEnabled()
    })
  })

  it('submits a strength action once when the committed value changes', () => {
    const view = renderImageActionMenu({
      showStrengthControl: true,
      activeStrengthAction: IMAGE_NODE_ACTION.variations,
    })
    const range = screen.getByRole('slider', { name: 'Variations strength' })

    Object.defineProperty(range, 'value', { configurable: true, value: '72' })
    range.dispatchEvent(new Event('input', { bubbles: true }))
    range.dispatchEvent(new Event('pointerup', { bubbles: true }))
    range.dispatchEvent(new Event('click', { bubbles: true }))
    range.dispatchEvent(new Event('change', { bubbles: true }))

    expect(view.props.strengthControl.onInput).toHaveBeenCalledOnce()
    expect(view.props.strengthControl.onPointerUp).toHaveBeenCalledOnce()
    expect(view.props.strengthControl.onChange).toHaveBeenCalledOnce()
    expect(view.emitted('action')).toEqual([[IMAGE_NODE_ACTION.variations, 72]])
  })
})
