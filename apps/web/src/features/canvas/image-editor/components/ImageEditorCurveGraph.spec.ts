import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID,
  createIdentityCurvePoints,
  normalizeCurveChannelPoints,
  type NormalizedCurveChannelPoint,
} from '../utils/imageEditorCurves'
import ImageEditorCurveGraph from './ImageEditorCurveGraph.vue'

function mountCurveGraph(options: {
  points?: readonly NormalizedCurveChannelPoint[]
  selectedPointId?: string
  addPointId?: string
} = {}) {
  const points = ref<readonly NormalizedCurveChannelPoint[]>(
    options.points ?? createIdentityCurvePoints(),
  )
  const selectedPointId = ref(options.selectedPointId ?? IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID)
  const events = {
    addPoint: vi.fn((point: { input: number; output: number }) => {
      if (!options.addPointId) return
      points.value = normalizeCurveChannelPoints([
        ...points.value.map(({ id, input, output }) => ({ id, input, output })),
        {
          id: options.addPointId,
          input: point.input,
          output: point.output,
        },
      ])
      selectedPointId.value = options.addPointId
    }),
    movePoint: vi.fn(),
    removePoint: vi.fn(),
    selectPoint: vi.fn((pointId: string) => {
      selectedPointId.value = pointId
    }),
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(ImageEditorCurveGraph, {
            activeCurvePath: 'M 0 100 L 100 0',
            ghostCurvePaths: [],
            histogramPath: '',
            points: points.value,
            selectedPointId: selectedPointId.value,
            onAddPoint: events.addPoint,
            onMovePoint: events.movePoint,
            onRemovePoint: events.removePoint,
            onSelectPoint: events.selectPoint,
          })
      },
    }),
  )
  app.mount(host)
  const graph = host.querySelector<HTMLElement>('.image-editor-curves__graph')
  if (!graph) throw new Error('expected curve graph')
  Object.defineProperty(graph, 'getBoundingClientRect', {
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
  graph.setPointerCapture = vi.fn()
  graph.releasePointerCapture = vi.fn()
  graph.hasPointerCapture = vi.fn(() => true)

  return {
    app,
    events,
    graph,
    host,
    points,
    selectedPointId,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function dispatchPointerEvent(
  target: EventTarget,
  type: string,
  init: Partial<PointerEvent> = {},
) {
  target.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      pointerId: 1,
      clientX: 50,
      clientY: 50,
      ...init,
    }),
  )
}

describe('ImageEditorCurveGraph', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('adds a point at the clicked graph position', () => {
    const harness = mountCurveGraph()

    try {
      dispatchPointerEvent(harness.graph, 'pointerdown', { clientX: 30.5, clientY: 34.4 })

      expect(harness.events.addPoint).toHaveBeenCalledWith({ input: 31, output: 66 })
      expect(harness.events.movePoint).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('continues dragging a point immediately after creating it', async () => {
    const harness = mountCurveGraph({ addPointId: 'new-point' })

    try {
      dispatchPointerEvent(harness.graph, 'pointerdown', { clientX: 35, clientY: 60 })
      await nextTick()
      dispatchPointerEvent(window, 'pointermove', { clientX: 48, clientY: 42 })

      expect(harness.events.addPoint).toHaveBeenCalledWith({ input: 35, output: 40 })
      expect(harness.events.movePoint).toHaveBeenCalledWith('new-point', {
        input: 48,
        output: 58,
      })
    } finally {
      harness.unmount()
    }
  })

  it('renders endpoint handles centered on the graph edge', async () => {
    const harness = mountCurveGraph()

    try {
      await nextTick()
      const handles = harness.host.querySelectorAll<HTMLButtonElement>('.image-editor-curves__point')

      expect(handles).toHaveLength(2)
      expect(handles[0]?.style.left).toBe('0%')
      expect(handles[0]?.style.top).toBe('100%')
      expect(handles[1]?.style.left).toBe('100%')
      expect(handles[1]?.style.top).toBe('0%')
    } finally {
      harness.unmount()
    }
  })

  it('selects and drags a nearby endpoint instead of adding another point', () => {
    const harness = mountCurveGraph()

    try {
      dispatchPointerEvent(harness.graph, 'pointerdown', { clientX: 0, clientY: 100 })

      expect(harness.events.selectPoint).toHaveBeenCalledWith(IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID)
      expect(harness.events.addPoint).not.toHaveBeenCalled()
      expect(harness.events.movePoint).toHaveBeenCalledWith(IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID, {
        input: 0,
        output: 0,
      })
    } finally {
      harness.unmount()
    }
  })

  it('selects and drags a nearby point instead of adding another point', () => {
    const harness = mountCurveGraph({
      points: normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }]),
    })

    try {
      dispatchPointerEvent(harness.graph, 'pointerdown', { clientX: 53, clientY: 51 })

      expect(harness.events.addPoint).not.toHaveBeenCalled()
      expect(harness.events.selectPoint).toHaveBeenCalledWith('mid')
      expect(harness.events.movePoint).toHaveBeenCalledWith('mid', { input: 53, output: 49 })
    } finally {
      harness.unmount()
    }
  })

  it('captures the active pointer and stops dragging after pointer cancel', () => {
    const harness = mountCurveGraph()

    try {
      dispatchPointerEvent(harness.graph, 'pointerdown', { clientX: 0, clientY: 100 })
      expect(harness.graph.setPointerCapture).toHaveBeenCalledWith(1)

      harness.events.movePoint.mockClear()
      dispatchPointerEvent(window, 'pointercancel')
      dispatchPointerEvent(window, 'pointermove', { clientX: 40, clientY: 40 })

      expect(harness.graph.releasePointerCapture).toHaveBeenCalledWith(1)
      expect(harness.events.movePoint).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('stops dragging when pointer capture is lost', () => {
    const harness = mountCurveGraph()

    try {
      dispatchPointerEvent(harness.graph, 'pointerdown', { clientX: 0, clientY: 100 })
      harness.events.movePoint.mockClear()

      dispatchPointerEvent(harness.graph, 'lostpointercapture')
      dispatchPointerEvent(window, 'pointermove', { clientX: 40, clientY: 40 })

      expect(harness.events.movePoint).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })

  it('emits keyboard move and remove events for focused user points', async () => {
    const harness = mountCurveGraph({
      points: normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }]),
      selectedPointId: 'mid',
    })

    try {
      await nextTick()
      const midPoint = harness.host.querySelectorAll<HTMLButtonElement>('.image-editor-curves__point')[1]
      const arrowEvent = new KeyboardEvent('keydown', {
        key: 'ArrowUp',
        bubbles: true,
        cancelable: true,
      })
      const backspaceEvent = new KeyboardEvent('keydown', {
        key: 'Backspace',
        bubbles: true,
        cancelable: true,
      })
      midPoint?.dispatchEvent(arrowEvent)
      midPoint?.dispatchEvent(backspaceEvent)

      expect(harness.events.movePoint).toHaveBeenCalledWith('mid', { input: 50, output: 51 })
      expect(harness.events.removePoint).toHaveBeenCalledWith('mid')
      expect(arrowEvent.defaultPrevented).toBe(true)
      expect(backspaceEvent.defaultPrevented).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('focuses a clicked user point so Delete removes it without bubbling to node shortcuts', async () => {
    const harness = mountCurveGraph({
      points: normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }]),
    })

    try {
      await nextTick()
      const midPoint =
        harness.host.querySelectorAll<HTMLButtonElement>('.image-editor-curves__point')[1]
      const bubbledKeydown = vi.fn()
      harness.graph.addEventListener('keydown', bubbledKeydown)

      dispatchPointerEvent(midPoint, 'pointerdown')
      harness.events.removePoint.mockClear()
      document.activeElement?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true }),
      )

      expect(document.activeElement).toBe(midPoint)
      expect(harness.events.removePoint).toHaveBeenCalledWith('mid')
      expect(bubbledKeydown).not.toHaveBeenCalled()
    } finally {
      harness.unmount()
    }
  })
})
