import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../test/render'
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

function renderLevelsControl(
  initialParams: ImageEditorLevelsParams,
  histogramSource: ImageData | null = null,
) {
  const params = ref(initialParams)
  const events = {
    updateParams: vi.fn((nextParams: ImageEditorLevelsParams) => {
      params.value = nextParams
    }),
  }
  const Harness = defineComponent({
    setup() {
      return () =>
        h(ImageEditorLevelsControl, {
          params: params.value,
          histogramSource,
          'onUpdate:params': events.updateParams,
        })
    },
  })

  const result = renderWithUser(Harness)
  const plot = screen.getByRole('group', { name: 'Levels histogram' })
  const outputSlider = screen.getByRole('group', { name: 'Output levels range' })

  stubPointerTarget(plot, { right: 100, bottom: 100, width: 100, height: 100 })
  stubPointerTarget(outputSlider, { right: 255, bottom: 22, width: 255, height: 22 })

  return {
    events,
    outputSlider,
    params,
    plot,
    ...result,
  }
}

function stubPointerTarget(
  target: HTMLElement,
  rect: Pick<DOMRect, 'right' | 'bottom' | 'width' | 'height'>,
) {
  Object.defineProperty(target, 'getBoundingClientRect', {
    value: () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      toJSON: () => ({}),
      ...rect,
    }),
  })
  target.setPointerCapture = vi.fn()
  target.releasePointerCapture = vi.fn()
  target.hasPointerCapture = vi.fn(() => true)
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

function updateInputValue(target: HTMLElement, value: string) {
  const input = target as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('ImageEditorLevelsControl', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders RGB histogram, level markers, and guide lines', () => {
    const { plot } = renderLevelsControl(
      { shadows: 64, midtones: 128, highlights: 191, outputBlack: 0, outputWhite: 255 },
      makeImageData([0, 0, 0, 255, 255, 255, 255, 255]),
    )

    const histogram = plot.querySelector<SVGPathElement>('.image-editor-histogram-plot__histogram')
    const markers = within(plot).getAllByRole('slider')
    const guideLines = plot.querySelectorAll<SVGLineElement>(
      '.image-editor-histogram-plot__guide-line',
    )

    expect(histogram?.getAttribute('d')).toContain('L 0 0')
    expect(markers).toHaveLength(3)
    expect(guideLines).toHaveLength(3)
    expect(markers[0]?.style.left).toBe(`${(64 / 255) * 100}%`)
    expect(markers[1]?.style.left).toBe(`${(128 / 255) * 100}%`)
    expect(markers[2]?.style.left).toBe(`${(191 / 255) * 100}%`)
    expect(markers[0]).toHaveAttribute('aria-label', 'Blacks 64')
    expect(markers[0]).toHaveAttribute('aria-valuemin', '0')
    expect(markers[0]).toHaveAttribute('aria-valuemax', '190')
    expect(markers[0]).toHaveAttribute('aria-valuenow', '64')
    expect(markers[1]).toHaveClass('is-midtones')
    expect(markers[1]).toHaveAttribute('aria-valuemin', '65')
    expect(markers[1]).toHaveAttribute('aria-valuemax', '190')
    expect(markers[2]).toHaveAttribute('aria-label', 'Whites 191')
    expect(markers[2]).toHaveAttribute('aria-valuemin', '65')
    expect(markers[2]).toHaveAttribute('aria-valuemax', '255')
    expect(guideLines[0]?.getAttribute('x1')).toBe(`${(64 / 255) * 100}`)
    expect(guideLines[1]?.getAttribute('x1')).toBe(`${(128 / 255) * 100}`)
    expect(guideLines[2]?.getAttribute('x1')).toBe(`${(191 / 255) * 100}`)
    expect(guideLines[0]).toHaveClass('is-shadows')
    expect(guideLines[1]).toHaveClass('is-midtones')
    expect(guideLines[2]).toHaveClass('is-highlights')
  })

  it('updates full levels params from marker drag with handle constraints', async () => {
    const { events } = renderLevelsControl({
      shadows: 10,
      midtones: 128,
      highlights: 153,
      outputBlack: 0,
      outputWhite: 255,
    })

    dispatchPointerEvent(screen.getByRole('slider', { name: 'Blacks 10' }), 'pointerdown', {
      clientX: 90,
    })
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 152,
      midtones: 152,
      highlights: 153,
      outputBlack: 0,
      outputWhite: 255,
    })

    await nextTick()
    dispatchPointerEvent(screen.getByRole('slider', { name: 'Midtones 152' }), 'pointerdown', {
      clientX: 30,
    })
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 152,
      midtones: 152,
      highlights: 153,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('updates full levels params from integer channel numeric inputs', async () => {
    const { events } = renderLevelsControl({
      shadows: 10,
      midtones: 128,
      highlights: 230,
      outputBlack: 0,
      outputWhite: 255,
    })

    updateInputValue(screen.getByLabelText('Whites'), '30')

    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 10,
      midtones: 29,
      highlights: 30,
      outputBlack: 0,
      outputWhite: 255,
    })

    await nextTick()
    updateInputValue(screen.getByLabelText('Midtones'), '0')

    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 10,
      midtones: 11,
      highlights: 30,
      outputBlack: 0,
      outputWhite: 255,
    })
  })

  it('updates output levels from the global range controls', async () => {
    const { events, outputSlider } = renderLevelsControl({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 20,
      outputWhite: 220,
    })
    const outputHandles = within(outputSlider).getAllByRole('slider')
    const outputNumbers = [
      screen.getByRole('spinbutton', { name: 'Output Blacks' }) as HTMLInputElement,
      screen.getByRole('spinbutton', { name: 'Output Whites' }) as HTMLInputElement,
    ]

    expect(outputHandles).toHaveLength(2)
    expect(outputNumbers).toHaveLength(2)
    expect(outputHandles[0]).toHaveAttribute('role', 'slider')
    expect(outputHandles[1]).toHaveAttribute('role', 'slider')
    expect(
      outputNumbers.every((input) => input.classList.contains('editing-controls-number-input')),
    ).toBe(true)
    expect(outputNumbers[0]?.value).toBe('20')
    expect(outputNumbers[1]?.value).toBe('220')
    expect(outputHandles.map((handle) => handle.getAttribute('aria-label'))).toEqual([
      'Output Blacks',
      'Output Whites',
    ])
    expect(outputNumbers.map((input) => [input.min, input.max])).toEqual([
      ['0', '219'],
      ['21', '255'],
    ])
    expect(outputHandles.map((handle) => handle.getAttribute('aria-valuemin'))).toEqual(['0', '21'])
    expect(outputHandles.map((handle) => handle.getAttribute('aria-valuemax'))).toEqual([
      '219',
      '255',
    ])
    expect(outputNumbers.map((input) => input.getAttribute('aria-valuemin'))).toEqual(['0', '21'])
    expect(outputNumbers.map((input) => input.getAttribute('aria-valuemax'))).toEqual([
      '219',
      '255',
    ])

    events.updateParams.mockClear()
    dispatchPointerEvent(outputSlider, 'pointerdown', { clientX: 40 })
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 40,
      outputWhite: 220,
    })
    dispatchPointerEvent(window, 'pointerup', { clientX: 40 })

    await nextTick()
    const updatedOutputHandles = within(outputSlider).getAllByRole('slider')
    events.updateParams.mockClear()
    dispatchPointerEvent(updatedOutputHandles[1]!, 'pointerdown', { clientX: 220 })
    expect(events.updateParams).not.toHaveBeenCalled()
    dispatchPointerEvent(window, 'pointermove', { clientX: 10 })
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 40,
      outputWhite: 41,
    })
    dispatchPointerEvent(window, 'pointerup', { clientX: 10 })

    await nextTick()
    const boundaryOutputHandles = within(outputSlider).getAllByRole('slider')
    events.updateParams.mockClear()
    dispatchPointerEvent(boundaryOutputHandles[0]!, 'pointerdown', { clientX: 40 })
    dispatchPointerEvent(window, 'pointermove', { clientX: 240 })
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 40,
      outputWhite: 41,
    })
    dispatchPointerEvent(window, 'pointerup', { clientX: 240 })

    await nextTick()
    updateInputValue(outputNumbers[0]!, '30')
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 30,
      outputWhite: 41,
    })

    await nextTick()
    updateInputValue(outputNumbers[1]!, '20')
    expect(events.updateParams).toHaveBeenCalledWith({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 30,
      outputWhite: 31,
    })
  })
})
