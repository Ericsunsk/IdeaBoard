import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorThresholdParams } from '../types'
import ImageEditorThresholdControl from './ImageEditorThresholdControl.vue'

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

function mountThresholdControl(
  initialParams: ImageEditorThresholdParams,
  histogramSource: ImageData | null = null,
) {
  const params = ref(initialParams)
  const events = {
    updateParams: vi.fn((nextParams: ImageEditorThresholdParams) => {
      params.value = nextParams
    }),
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorThresholdControl, {
            params: params.value,
            histogramSource,
            'onUpdate:params': events.updateParams,
          })
      },
    }),
  )
  app.mount(host)
  const plot = host.querySelector<HTMLElement>('.image-editor-histogram-plot')
  if (!plot) throw new Error('expected threshold histogram plot')
  Object.defineProperty(plot, 'getBoundingClientRect', {
    value: () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 255,
      bottom: 100,
      width: 255,
      height: 100,
      toJSON: () => ({}),
    }),
  })
  plot.setPointerCapture = vi.fn()
  plot.releasePointerCapture = vi.fn()
  plot.hasPointerCapture = vi.fn(() => true)

  return {
    app,
    events,
    host,
    params,
    plot,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

describe('ImageEditorThresholdControl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders luminance histogram and a histogram threshold divider', async () => {
    const harness = mountThresholdControl(
      { threshold: 64, invert: true },
      makeImageData([0, 0, 0, 255, 255, 255, 255, 255]),
    )

    try {
      await nextTick()
      const histogram = harness.host.querySelector<SVGPathElement>(
        '.image-editor-histogram-plot__histogram',
      )
      const marker = harness.host.querySelector<HTMLElement>(
        '.image-editor-histogram-plot__marker',
      )
      const slider = harness.host.querySelector<HTMLInputElement>('input[type="range"]')
      const guideLine = harness.host.querySelector<SVGLineElement>(
        '.image-editor-histogram-plot__guide-line',
      )

      expect(histogram?.getAttribute('d')).toContain('L 0 0')
      expect(marker?.style.left).toBe(`${(64 / 255) * 100}%`)
      expect(marker?.classList.contains('is-threshold')).toBe(true)
      expect(marker?.getAttribute('aria-hidden')).toBe('true')
      expect(slider?.getAttribute('aria-label')).toBe('阈值 64')
      expect(slider?.getAttribute('aria-valuenow')).toBe('64')
      expect(slider?.min).toBe('0')
      expect(slider?.max).toBe('255')
      expect(slider?.step).toBe('1')
      expect(guideLine?.getAttribute('x1')).toBe(`${(64 / 255) * 100}`)
      expect(guideLine?.getAttribute('x2')).toBe(`${(64 / 255) * 100}`)
      expect(guideLine?.classList.contains('is-threshold')).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('updates threshold params from native histogram range and numeric input', async () => {
    const harness = mountThresholdControl({ threshold: 64, invert: true })

    try {
      const slider = harness.host.querySelector<HTMLInputElement>('input[type="range"]')
      slider!.value = '192'
      slider?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        threshold: 192,
        invert: true,
      })

      await nextTick()
      const numberInput = harness.host.querySelector<HTMLInputElement>('input[type="number"]')
      expect(numberInput?.getAttribute('inputmode')).toBe('numeric')
      numberInput!.value = '300'
      numberInput?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        threshold: 255,
        invert: true,
      })
    } finally {
      harness.unmount()
    }
  })

  it('updates threshold params from native range input in channel units', async () => {
    const harness = mountThresholdControl({ threshold: 64, invert: false })

    try {
      await nextTick()
      const slider = harness.host.querySelector<HTMLInputElement>('input[type="range"]')

      slider!.value = '65'
      slider?.dispatchEvent(new Event('input', { bubbles: true }))
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        threshold: 65,
        invert: false,
      })

      await nextTick()
      slider!.value = '55'
      slider?.dispatchEvent(new Event('input', { bubbles: true }))
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        threshold: 55,
        invert: false,
      })
    } finally {
      harness.unmount()
    }
  })

  it('updates invert params without changing the normalized threshold', async () => {
    const harness = mountThresholdControl({ threshold: Number.NaN, invert: false })

    try {
      await nextTick()
      const invertButton = harness.host.querySelector<HTMLButtonElement>(
        '.image-editor-histogram-control__toggle-button',
      )

      expect(invertButton?.getAttribute('aria-pressed')).toBe('false')
      expect(invertButton?.getAttribute('aria-label')).toBe('反转阈值输出')
      expect(invertButton?.querySelector('.node-tool-tooltip')?.textContent?.trim()).toBe('反转')
      invertButton?.click()

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        threshold: 128,
        invert: true,
      })
    } finally {
      harness.unmount()
    }
  })
})
