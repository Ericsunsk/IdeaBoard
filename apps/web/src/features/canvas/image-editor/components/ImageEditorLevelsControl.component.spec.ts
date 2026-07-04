import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ImageEditorLevelsParams } from '../types'
import ImageEditorLevelsControl from './ImageEditorLevelsControl.vue'

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

function mountLevelsControl(
  initialParams: ImageEditorLevelsParams,
  histogramSource: ImageData | null = null,
) {
  const params = ref(initialParams)
  const events = {
    updateParams: vi.fn((nextParams: ImageEditorLevelsParams) => {
      params.value = nextParams
    }),
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorLevelsControl, {
            params: params.value,
            histogramSource,
            'onUpdate:params': events.updateParams,
          })
      },
    }),
  )
  app.mount(host)
  const plot = host.querySelector<HTMLElement>('.image-editor-histogram-plot')
  if (!plot) throw new Error('expected levels histogram plot')
  Object.defineProperty(plot, 'getBoundingClientRect', {
    value: () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100,
      toJSON: () => ({}),
    }),
  })
  plot.setPointerCapture = vi.fn()
  plot.releasePointerCapture = vi.fn()
  plot.hasPointerCapture = vi.fn(() => true)
  const outputSlider = host.querySelector<HTMLElement>('.image-editor-levels-output__slider')
  if (outputSlider) {
    Object.defineProperty(outputSlider, 'getBoundingClientRect', {
      value: () => ({
        x: 0,
        y: 0,
        left: 0,
        top: 0,
        right: 255,
        bottom: 22,
        width: 255,
        height: 22,
        toJSON: () => ({}),
      }),
    })
    outputSlider.setPointerCapture = vi.fn()
    outputSlider.releasePointerCapture = vi.fn()
    outputSlider.hasPointerCapture = vi.fn(() => true)
  }

  return {
    app,
    events,
    host,
    outputSlider,
    params,
    plot,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function dispatchPointerEvent(target: EventTarget, type: string, init: Partial<PointerEvent> = {}) {
  target.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      pointerId: 1,
      clientX: 50,
      clientY: 40,
      ...init,
    }),
  )
}

describe('ImageEditorLevelsControl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders RGB histogram, level markers, and guide lines', async () => {
    const harness = mountLevelsControl(
      { shadows: 64, midtones: 128, highlights: 191, outputBlack: 0, outputWhite: 255 },
      makeImageData([0, 0, 0, 255, 255, 255, 255, 255]),
    )

    try {
      await nextTick()
      const histogram = harness.host.querySelector<SVGPathElement>(
        '.image-editor-histogram-plot__histogram',
      )
      const markers = harness.host.querySelectorAll<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )
      const guideLines = harness.host.querySelectorAll<SVGLineElement>(
        '.image-editor-histogram-plot__guide-line',
      )

      expect(histogram?.getAttribute('d')).toContain('L 0 0')
      expect(markers).toHaveLength(3)
      expect(guideLines).toHaveLength(3)
      expect(markers[0]?.style.left).toBe(`${(64 / 255) * 100}%`)
      expect(markers[1]?.style.left).toBe(`${(128 / 255) * 100}%`)
      expect(markers[2]?.style.left).toBe(`${(191 / 255) * 100}%`)
      expect(markers[0]?.getAttribute('aria-label')).toBe('黑场 64')
      expect(markers[0]?.getAttribute('aria-valuemin')).toBe('0')
      expect(markers[0]?.getAttribute('aria-valuemax')).toBe('190')
      expect(markers[0]?.getAttribute('aria-valuenow')).toBe('64')
      expect(markers[1]?.classList.contains('is-midtones')).toBe(true)
      expect(markers[1]?.getAttribute('aria-valuemin')).toBe('65')
      expect(markers[1]?.getAttribute('aria-valuemax')).toBe('190')
      expect(markers[2]?.getAttribute('aria-label')).toBe('白场 191')
      expect(markers[2]?.getAttribute('aria-valuemin')).toBe('65')
      expect(markers[2]?.getAttribute('aria-valuemax')).toBe('255')
      expect(guideLines[0]?.getAttribute('x1')).toBe(`${(64 / 255) * 100}`)
      expect(guideLines[1]?.getAttribute('x1')).toBe(`${(128 / 255) * 100}`)
      expect(guideLines[2]?.getAttribute('x1')).toBe(`${(191 / 255) * 100}`)
      expect(guideLines[0]?.classList.contains('is-shadows')).toBe(true)
      expect(guideLines[1]?.classList.contains('is-midtones')).toBe(true)
      expect(guideLines[2]?.classList.contains('is-highlights')).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('updates full levels params from marker drag with handle constraints', async () => {
    const harness = mountLevelsControl({
      shadows: 10,
      midtones: 128,
      highlights: 153,
      outputBlack: 0,
      outputWhite: 255,
    })

    try {
      await nextTick()
      const markers = harness.host.querySelectorAll<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )

      dispatchPointerEvent(markers[0]!, 'pointerdown', { clientX: 90 })
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 152,
        midtones: 152,
        highlights: 153,
        outputBlack: 0,
        outputWhite: 255,
      })

      await nextTick()
      dispatchPointerEvent(markers[1]!, 'pointerdown', { clientX: 30 })
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 152,
        midtones: 152,
        highlights: 153,
        outputBlack: 0,
        outputWhite: 255,
      })
    } finally {
      harness.unmount()
    }
  })

  it('updates full levels params from integer channel numeric inputs', async () => {
    const harness = mountLevelsControl({
      shadows: 10,
      midtones: 128,
      highlights: 230,
      outputBlack: 0,
      outputWhite: 255,
    })

    try {
      await nextTick()
      const numberInputs = harness.host.querySelectorAll<HTMLInputElement>(
        '.image-editor-levels-fields input[type="number"]',
      )

      numberInputs[2]!.value = '30'
      numberInputs[2]?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 10,
        midtones: 29,
        highlights: 30,
        outputBlack: 0,
        outputWhite: 255,
      })

      await nextTick()
      numberInputs[1]!.value = '0'
      numberInputs[1]?.dispatchEvent(new Event('input', { bubbles: true }))

      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 10,
        midtones: 11,
        highlights: 30,
        outputBlack: 0,
        outputWhite: 255,
      })
    } finally {
      harness.unmount()
    }
  })

  it('updates output levels from the global range controls', async () => {
    const harness = mountLevelsControl({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 20,
      outputWhite: 220,
    })

    try {
      await nextTick()
      const outputHandles = harness.host.querySelectorAll<HTMLButtonElement>(
        '.editing-two-handle-slider__handle',
      )
      const outputNumbers = harness.host.querySelectorAll<HTMLInputElement>(
        '.image-editor-levels-output__field input[type="number"]',
      )

      expect(outputHandles).toHaveLength(2)
      expect(outputNumbers).toHaveLength(2)
      expect(outputHandles[0]?.getAttribute('role')).toBe('slider')
      expect(outputHandles[1]?.getAttribute('role')).toBe('slider')
      expect(
        [...outputNumbers].every((input) =>
          input.classList.contains('editing-controls-number-input'),
        ),
      ).toBe(true)
      expect(outputNumbers[0]?.value).toBe('20')
      expect(outputNumbers[0]?.getAttribute('aria-label')).toBe('输出黑场')
      expect(outputNumbers[1]?.value).toBe('220')
      expect(outputNumbers[1]?.getAttribute('aria-label')).toBe('输出白场')
      expect([...outputHandles].map((handle) => handle.getAttribute('aria-label'))).toEqual([
        '输出黑场',
        '输出白场',
      ])
      expect([...outputNumbers].map((input) => [input.min, input.max])).toEqual([
        ['0', '219'],
        ['21', '255'],
      ])
      expect([...outputHandles].map((handle) => handle.getAttribute('aria-valuemin'))).toEqual([
        '0',
        '21',
      ])
      expect([...outputHandles].map((handle) => handle.getAttribute('aria-valuemax'))).toEqual([
        '219',
        '255',
      ])
      expect([...outputNumbers].map((input) => input.getAttribute('aria-valuemin'))).toEqual([
        '0',
        '21',
      ])
      expect([...outputNumbers].map((input) => input.getAttribute('aria-valuemax'))).toEqual([
        '219',
        '255',
      ])

      harness.events.updateParams.mockClear()
      dispatchPointerEvent(harness.outputSlider!, 'pointerdown', { clientX: 40 })
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 40,
        outputWhite: 220,
      })
      dispatchPointerEvent(window, 'pointerup', { clientX: 40 })

      await nextTick()
      const updatedOutputHandles = harness.host.querySelectorAll<HTMLButtonElement>(
        '.editing-two-handle-slider__handle',
      )
      harness.events.updateParams.mockClear()
      dispatchPointerEvent(updatedOutputHandles[1]!, 'pointerdown', { clientX: 220 })
      expect(harness.events.updateParams).not.toHaveBeenCalled()
      dispatchPointerEvent(window, 'pointermove', { clientX: 10 })
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 40,
        outputWhite: 41,
      })
      dispatchPointerEvent(window, 'pointerup', { clientX: 10 })

      await nextTick()
      const boundaryOutputHandles = harness.host.querySelectorAll<HTMLButtonElement>(
        '.editing-two-handle-slider__handle',
      )
      harness.events.updateParams.mockClear()
      dispatchPointerEvent(boundaryOutputHandles[0]!, 'pointerdown', { clientX: 40 })
      dispatchPointerEvent(window, 'pointermove', { clientX: 240 })
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 40,
        outputWhite: 41,
      })
      dispatchPointerEvent(window, 'pointerup', { clientX: 240 })

      await nextTick()
      outputNumbers[0]!.value = '30'
      outputNumbers[0]?.dispatchEvent(new Event('input', { bubbles: true }))
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 30,
        outputWhite: 41,
      })

      await nextTick()
      outputNumbers[1]!.value = '20'
      outputNumbers[1]?.dispatchEvent(new Event('input', { bubbles: true }))
      expect(harness.events.updateParams).toHaveBeenCalledWith({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 30,
        outputWhite: 31,
      })
    } finally {
      harness.unmount()
    }
  })
})
