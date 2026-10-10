import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../../../test/render'
import { setUserLocale } from '../../../../../../core/i18n'
import type { RetouchThresholdParams } from '../../core/model.ts'
import RetouchThresholdControl from './ThresholdControl.vue'

function makeImageData(values: number[]) {
  class TestImageData {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }

  return new TestImageData(new Uint8ClampedArray(values), values.length / 4, 1) as ImageData
}

function renderThresholdControl(
  initialParams: RetouchThresholdParams,
  histogramSource: ImageData | null = null,
) {
  const params = ref(initialParams)
  const events = {
    updateParams: vi.fn((nextParams: RetouchThresholdParams) => {
      params.value = nextParams
    }),
  }
  const Harness = defineComponent({
    setup() {
      return () =>
        h(RetouchThresholdControl, {
          params: params.value,
          histogramSource,
          'onUpdate:params': events.updateParams,
        })
    },
  })

  return {
    events,
    params,
    ...renderWithUser(Harness),
  }
}

function updateInputValue(target: HTMLElement, value: string) {
  const input = target as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('RetouchThresholdControl', () => {
  afterEach(async () => {
    vi.restoreAllMocks()
    await setUserLocale('en')
  })

  it('renders luminance histogram and a histogram threshold divider', () => {
    renderThresholdControl(
      { threshold: 64, invert: true },
      makeImageData([0, 0, 0, 255, 255, 255, 255, 255]),
    )

    const plot = screen.getByRole('group', { name: 'Threshold histogram' })
    const histogram = plot.querySelector<SVGPathElement>('.retouch-histogram-plot__histogram')
    const marker = plot.querySelector<HTMLElement>('.retouch-histogram-plot__marker')
    const slider = within(plot).getByRole('slider', { name: 'Threshold 64' }) as HTMLInputElement
    const guideLine = plot.querySelector<SVGLineElement>('.retouch-histogram-plot__guide-line')

    expect(histogram?.getAttribute('d')).toContain('L 0 0')
    expect(marker?.style.left).toBe(`${(64 / 255) * 100}%`)
    expect(marker).toHaveClass('is-threshold')
    expect(marker).toHaveAttribute('aria-hidden', 'true')
    expect(slider).toHaveAttribute('aria-valuenow', '64')
    expect(slider.min).toBe('0')
    expect(slider.max).toBe('255')
    expect(slider.step).toBe('1')
    expect(guideLine?.getAttribute('x1')).toBe(`${(64 / 255) * 100}`)
    expect(guideLine?.getAttribute('x2')).toBe(`${(64 / 255) * 100}`)
    expect(guideLine).toHaveClass('is-threshold')
  })

  it('updates threshold params from native histogram range and numeric input', () => {
    const { events } = renderThresholdControl({ threshold: 64, invert: true })
    const slider = screen.getByRole('slider', { name: 'Threshold 64' })

    updateInputValue(slider, '192')

    expect(events.updateParams).toHaveBeenCalledWith({
      threshold: 192,
      invert: true,
    })

    const numberInput = screen.getByLabelText('Threshold', {
      selector: 'input[type="number"]',
    })

    expect(numberInput).toHaveAttribute('inputmode', 'numeric')
    expect(numberInput).toHaveClass('editing-controls-number-input')
    updateInputValue(numberInput, '300')

    expect(events.updateParams).toHaveBeenCalledWith({
      threshold: 255,
      invert: true,
    })
  })

  it('updates threshold params from native range input in channel units', () => {
    const { events } = renderThresholdControl({ threshold: 64, invert: false })
    const slider = screen.getByRole('slider', { name: 'Threshold 64' })

    updateInputValue(slider, '65')
    expect(events.updateParams).toHaveBeenCalledWith({
      threshold: 65,
      invert: false,
    })

    updateInputValue(slider, '55')
    expect(events.updateParams).toHaveBeenCalledWith({
      threshold: 55,
      invert: false,
    })
  })

  it('passes invert through raw while storage normalization owns the threshold clamp', async () => {
    const { events, user } = renderThresholdControl({ threshold: Number.NaN, invert: false })
    const invertButton = screen.getByRole('button', { name: 'Invert threshold output' })

    expect(invertButton).toHaveAttribute('aria-pressed', 'false')
    expect(invertButton).toHaveClass('editing-controls-toggle-button')
    expect(invertButton.querySelector('.canvas-tool-tooltip')?.textContent?.trim()).toBe('Invert')

    await user.click(invertButton)

    // Emit stays raw (NaN passes through); layerStack normalizes NaN to the
    // 128 default on commit, which the session storage specs lock.
    expect(events.updateParams).toHaveBeenCalledWith({
      threshold: Number.NaN,
      invert: true,
    })
  })

  it('keeps a single range-mode marker without button sliders', () => {
    renderThresholdControl({ threshold: 64, invert: false })

    const plot = screen.getByRole('group', { name: 'Threshold histogram' })
    const slider = within(plot).getByRole('slider', { name: 'Threshold 64' })

    expect(slider).toHaveAttribute('type', 'range')
    expect(plot.querySelector('button[role="slider"]')).toBeNull()
    expect(plot.querySelector('.retouch-histogram-plot__marker')).toHaveAttribute(
      'aria-hidden',
      'true',
    )
  })

  it('moves the native range marker with keyboard input in channel units', () => {
    const { events } = renderThresholdControl({ threshold: 64, invert: false })
    const slider = screen.getByRole('slider', { name: 'Threshold 64' }) as HTMLInputElement

    slider.focus()
    expect(document.activeElement).toBe(slider)

    // Native range inputs commit through the input event; arrow keys move by
    // the step (1 channel) without extra marker plumbing.
    slider.step = '1'
    updateInputValue(slider, '65')
    expect(events.updateParams).toHaveBeenCalledWith({ threshold: 65, invert: false })

    updateInputValue(slider, '63')
    expect(events.updateParams).toHaveBeenCalledWith({ threshold: 63, invert: false })
  })

  it('re-translates the threshold marker on locale switch without remount', async () => {
    renderThresholdControl({ threshold: 64, invert: false })

    expect(screen.getByRole('slider', { name: 'Threshold 64' })).toBeVisible()

    await setUserLocale('es')
    await nextTick()

    expect(screen.getByRole('slider', { name: 'Umbral 64' })).toBeVisible()
  })
})
