import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
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

function mountHistogramPlot(
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
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorHistogramPlot, {
            histogramPath: options.histogramPath ?? 'M 0 100 L 50 0 L 100 100 Z',
            markers: markers.value,
            onUpdateMarker: events.updateMarker,
          })
      },
    }),
  )
  app.mount(host)
  const plot = host.querySelector<HTMLElement>('.image-editor-histogram-plot')
  if (!plot) throw new Error('expected histogram plot')
  Object.defineProperty(plot, 'getBoundingClientRect', {
    value: () => ({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 200,
      bottom: 100,
      width: 200,
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
    markers,
    plot,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function resolveNextAriaValue(marker: PlotMarker, position: number) {
  if (marker.ariaValueNow == null || marker.keyboardStep == null || marker.keyboardStep === 0) {
    return marker.ariaValueNow
  }
  return marker.ariaValueNow + (position - marker.position) / marker.keyboardStep
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

  it('renders marker positions without business semantics', async () => {
    const harness = mountHistogramPlot({
      markers: [
        {
          id: 'main',
          position: 50,
          ariaLabel: '主手柄',
          guideLine: true,
          guideLineClassName: 'is-test-guide',
        },
      ],
    })

    try {
      await nextTick()
      const marker = harness.host.querySelector<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )
      const guideLine = harness.host.querySelector<SVGLineElement>(
        '.image-editor-histogram-plot__guide-line',
      )

      expect(marker?.style.left).toBe('50%')
      expect(marker?.getAttribute('aria-label')).toBe('主手柄')
      expect(guideLine?.getAttribute('x1')).toBe('50')
      expect(guideLine?.getAttribute('x2')).toBe('50')
      expect(guideLine?.classList.contains('is-test-guide')).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('drags the nearest marker using normalized horizontal coordinates', async () => {
    const harness = mountHistogramPlot({
      markers: [
        { id: 'left', position: 12, ariaLabel: '左手柄' },
        { id: 'right', position: 82, ariaLabel: '右手柄' },
      ],
    })

    try {
      dispatchPointerEvent(harness.plot, 'pointerdown', { clientX: 170 })
      await nextTick()
      dispatchPointerEvent(window, 'pointermove', { clientX: 120 })
      const markers = harness.host.querySelectorAll<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )

      expect(harness.events.updateMarker).toHaveBeenCalledWith('right', 85)
      expect(harness.events.updateMarker).toHaveBeenCalledWith('right', 60)
      expect(harness.events.updateMarker).not.toHaveBeenCalledWith('left', expect.any(Number))
      expect(markers[0]?.classList.contains('active')).toBe(false)
      expect(markers[1]?.classList.contains('active')).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('focuses the selected marker after pointer selection', async () => {
    const harness = mountHistogramPlot({
      markers: [
        { id: 'left', position: 12, ariaLabel: '左手柄' },
        { id: 'right', position: 82, ariaLabel: '右手柄' },
      ],
    })

    try {
      dispatchPointerEvent(harness.plot, 'pointerdown', { clientX: 170 })
      await nextTick()
      const markers = harness.host.querySelectorAll<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )

      expect(document.activeElement).toBe(markers[1])
    } finally {
      harness.unmount()
    }
  })

  it('moves focused markers with horizontal keyboard input', async () => {
    const harness = mountHistogramPlot()

    try {
      await nextTick()
      const marker = harness.host.querySelector<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )
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

      marker?.dispatchEvent(arrowEvent)
      await nextTick()
      marker?.dispatchEvent(shiftArrowEvent)

      expect(harness.events.updateMarker).toHaveBeenCalledWith('main', 51)
      expect(harness.events.updateMarker).toHaveBeenCalledWith('main', 41)
      expect(arrowEvent.defaultPrevented).toBe(true)
      expect(shiftArrowEvent.defaultPrevented).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('moves focused markers to their effective min and max with Home and End', async () => {
    const harness = mountHistogramPlot({
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

    try {
      await nextTick()
      const marker = harness.host.querySelector<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )
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

      marker?.dispatchEvent(homeEvent)
      await nextTick()
      marker?.dispatchEvent(endEvent)

      expect(harness.events.updateMarker).toHaveBeenCalledWith(
        'main',
        expect.closeTo((10 / 255) * 100),
      )
      expect(harness.events.updateMarker).toHaveBeenCalledWith(
        'main',
        expect.closeTo((200 / 255) * 100),
      )
      expect(homeEvent.defaultPrevented).toBe(true)
      expect(endEvent.defaultPrevented).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('honors custom marker keyboard steps', async () => {
    const harness = mountHistogramPlot({
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

    try {
      await nextTick()
      const marker = harness.host.querySelector<HTMLButtonElement>(
        '.image-editor-histogram-plot__marker',
      )
      marker?.dispatchEvent(
        new KeyboardEvent('keydown', {
          key: 'ArrowRight',
          bubbles: true,
          cancelable: true,
        }),
      )

      expect(harness.events.updateMarker).toHaveBeenCalledWith('main', 50 + 100 / 255)
    } finally {
      harness.unmount()
    }
  })
})
