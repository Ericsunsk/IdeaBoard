import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import ImageEditorHistogramPlot from './ImageEditorHistogramPlot.vue'

type PlotMarker = {
  id: string
  position: number
  ariaLabel: string
  ariaValueMin?: number
  ariaValueMax?: number
  ariaValueNow?: number
  keyboardStep?: number
  keyboardShiftStep?: number
  guideLine?: boolean
  guideLineClassName?: string
}

function renderHistogramPlot(
  options: {
    markers?: readonly PlotMarker[]
    histogramPath?: string
  } = {},
) {
  const markers = ref<readonly PlotMarker[]>(
    options.markers ?? [{ id: 'main', position: 50, ariaLabel: '主手柄' }],
  )
  const events = {
    updateMarker: vi.fn((markerId: string, position: number) => {
      markers.value = markers.value.map((marker) =>
        marker.id === markerId
          ? { ...marker, position, ariaValueNow: resolveNextAriaValue(marker, position) }
          : marker,
      )
    }),
  }
  const Harness = defineComponent({
    setup() {
      return () =>
        h(ImageEditorHistogramPlot, {
          histogramPath: options.histogramPath ?? 'M 0 100 L 50 0 L 100 100 Z',
          markers: markers.value,
          onUpdateMarker: events.updateMarker,
        })
    },
  })

  const result = renderWithUser(Harness)
  const plot = screen.getByRole('group', { name: '直方图控制' })
  stubPointerTarget(plot, { right: 200, bottom: 100, width: 200, height: 100 })

  return {
    events,
    markers,
    plot,
    ...result,
  }
}

function resolveNextAriaValue(marker: PlotMarker, position: number) {
  if (marker.ariaValueNow == null || marker.keyboardStep == null || marker.keyboardStep === 0) {
    return marker.ariaValueNow
  }
  return marker.ariaValueNow + (position - marker.position) / marker.keyboardStep
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
      clientX: 100,
      clientY: 40,
      ...init,
    }),
  )
}

describe('ImageEditorHistogramPlot', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('drags the nearest marker using normalized horizontal coordinates', async () => {
    const { events, plot } = renderHistogramPlot({
      markers: [
        { id: 'left', position: 12, ariaLabel: '左手柄' },
        { id: 'right', position: 82, ariaLabel: '右手柄' },
      ],
    })

    dispatchPointerEvent(plot, 'pointerdown', { clientX: 170 })
    await nextTick()
    dispatchPointerEvent(window, 'pointermove', { clientX: 120 })

    const markerControls = within(plot).getAllByRole('slider')

    expect(events.updateMarker).toHaveBeenCalledWith('right', 85)
    expect(events.updateMarker).toHaveBeenCalledWith('right', 60)
    expect(events.updateMarker).not.toHaveBeenCalledWith('left', expect.any(Number))
    expect(markerControls[0]).not.toHaveClass('active')
    expect(markerControls[1]).toHaveClass('active')
  })

  it('focuses the selected marker after pointer selection', async () => {
    const { plot } = renderHistogramPlot({
      markers: [
        { id: 'left', position: 12, ariaLabel: '左手柄' },
        { id: 'right', position: 82, ariaLabel: '右手柄' },
      ],
    })

    dispatchPointerEvent(plot, 'pointerdown', { clientX: 170 })

    await waitFor(() => {
      expect(screen.getByRole('slider', { name: '右手柄' })).toHaveFocus()
    })
  })

  it('captures the active pointer and stops dragging after pointer cancel', () => {
    const { events, plot } = renderHistogramPlot()

    dispatchPointerEvent(plot, 'pointerdown', { clientX: 120 })
    expect(plot.setPointerCapture).toHaveBeenCalledWith(1)

    events.updateMarker.mockClear()
    dispatchPointerEvent(window, 'pointercancel')
    dispatchPointerEvent(window, 'pointermove', { clientX: 160 })

    expect(plot.releasePointerCapture).toHaveBeenCalledWith(1)
    expect(events.updateMarker).not.toHaveBeenCalled()
  })

  it('stops dragging when pointer capture is lost', () => {
    const { events, plot } = renderHistogramPlot()

    dispatchPointerEvent(plot, 'pointerdown', { clientX: 120 })
    events.updateMarker.mockClear()

    dispatchPointerEvent(plot, 'lostpointercapture')
    dispatchPointerEvent(window, 'pointermove', { clientX: 160 })

    expect(events.updateMarker).not.toHaveBeenCalled()
  })

  it('moves focused markers with horizontal keyboard input', async () => {
    const { events } = renderHistogramPlot()
    const marker = screen.getByRole('slider', { name: '主手柄' })
    const arrowEvent = new KeyboardEvent('keydown', {
      key: 'ArrowRight',
      bubbles: true,
      cancelable: true,
    })
    const shiftArrowEvent = new KeyboardEvent('keydown', {
      key: 'ArrowLeft',
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })

    marker.focus()
    marker.dispatchEvent(arrowEvent)
    await nextTick()
    marker.dispatchEvent(shiftArrowEvent)

    expect(events.updateMarker).toHaveBeenCalledWith('main', 51)
    expect(events.updateMarker).toHaveBeenCalledWith('main', 41)
    expect(arrowEvent.defaultPrevented).toBe(true)
    expect(shiftArrowEvent.defaultPrevented).toBe(true)
  })

  it('moves focused markers to their effective min and max with Home and End', async () => {
    const { events } = renderHistogramPlot({
      markers: [
        {
          id: 'main',
          position: (100 / 255) * 100,
          ariaLabel: '主手柄',
          ariaValueMin: 10,
          ariaValueMax: 200,
          ariaValueNow: 100,
          keyboardStep: 100 / 255,
          keyboardShiftStep: 1000 / 255,
        },
      ],
    })
    const marker = screen.getByRole('slider', { name: '主手柄' })
    const homeEvent = new KeyboardEvent('keydown', {
      key: 'Home',
      bubbles: true,
      cancelable: true,
    })
    const endEvent = new KeyboardEvent('keydown', {
      key: 'End',
      bubbles: true,
      cancelable: true,
    })

    marker.focus()
    marker.dispatchEvent(homeEvent)
    await nextTick()
    marker.dispatchEvent(endEvent)

    expect(events.updateMarker).toHaveBeenCalledWith('main', expect.closeTo((10 / 255) * 100))
    expect(events.updateMarker).toHaveBeenCalledWith('main', expect.closeTo((200 / 255) * 100))
    expect(homeEvent.defaultPrevented).toBe(true)
    expect(endEvent.defaultPrevented).toBe(true)
  })

  it('honors custom marker keyboard steps', () => {
    const { events } = renderHistogramPlot({
      markers: [
        {
          id: 'main',
          position: 50,
          ariaLabel: '主手柄',
          keyboardStep: 100 / 255,
          keyboardShiftStep: 1000 / 255,
        },
      ],
    })
    const marker = screen.getByRole('slider', { name: '主手柄' })

    marker.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'ArrowRight',
        bubbles: true,
        cancelable: true,
      }),
    )

    expect(events.updateMarker).toHaveBeenCalledWith('main', 50 + 100 / 255)
  })
})
