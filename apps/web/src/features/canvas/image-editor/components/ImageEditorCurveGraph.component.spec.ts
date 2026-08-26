import { defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import {
  IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID,
  createIdentityCurvePoints,
  normalizeCurveChannelPoints,
  type NormalizedCurveChannelPoint,
} from '../utils/imageEditorCurves'
import ImageEditorCurveGraph from './ImageEditorCurveGraph.vue'

function renderCurveGraph(
  options: {
    points?: readonly NormalizedCurveChannelPoint[]
    selectedPointId?: string
    addPointId?: string
  } = {},
) {
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
  const Harness = defineComponent({
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
  })

  const result = renderWithUser(Harness)
  const graph = screen.getByRole('group', { name: 'Curves graph' })
  stubPointerTarget(graph, { right: 100, bottom: 100, width: 100, height: 100 })

  return {
    events,
    graph,
    points,
    selectedPointId,
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
    const { events, graph } = renderCurveGraph()

    dispatchPointerEvent(graph, 'pointerdown', { clientX: 30.5, clientY: 34.4 })

    expect(events.addPoint).toHaveBeenCalledWith({ input: 31, output: 66 })
    expect(events.movePoint).not.toHaveBeenCalled()
  })

  it('continues dragging a point immediately after creating it', async () => {
    const { events, graph } = renderCurveGraph({ addPointId: 'new-point' })

    dispatchPointerEvent(graph, 'pointerdown', { clientX: 35, clientY: 60 })
    await nextTick()
    dispatchPointerEvent(window, 'pointermove', { clientX: 48, clientY: 42 })

    expect(events.addPoint).toHaveBeenCalledWith({ input: 35, output: 40 })
    expect(events.movePoint).toHaveBeenCalledWith('new-point', {
      input: 48,
      output: 58,
    })
  })

  it('renders endpoint handles centered on the graph edge', () => {
    const { graph } = renderCurveGraph()
    const handles = within(graph).getAllByRole('button')

    expect(handles).toHaveLength(2)
    expect(handles[0]?.style.left).toBe('0%')
    expect(handles[0]?.style.top).toBe('100%')
    expect(handles[1]?.style.left).toBe('100%')
    expect(handles[1]?.style.top).toBe('0%')
  })

  it('selects and drags a nearby endpoint instead of adding another point', () => {
    const { events, graph } = renderCurveGraph()

    dispatchPointerEvent(graph, 'pointerdown', { clientX: 0, clientY: 100 })

    expect(events.selectPoint).toHaveBeenCalledWith(IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID)
    expect(events.addPoint).not.toHaveBeenCalled()
    expect(events.movePoint).toHaveBeenCalledWith(IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID, {
      input: 0,
      output: 0,
    })
  })

  it('selects and drags a nearby point instead of adding another point', () => {
    const { events, graph } = renderCurveGraph({
      points: normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }]),
    })

    dispatchPointerEvent(graph, 'pointerdown', { clientX: 53, clientY: 51 })

    expect(events.addPoint).not.toHaveBeenCalled()
    expect(events.selectPoint).toHaveBeenCalledWith('mid')
    expect(events.movePoint).toHaveBeenCalledWith('mid', { input: 53, output: 49 })
  })

  it('captures the active pointer and stops dragging after pointer cancel', () => {
    const { events, graph } = renderCurveGraph()

    dispatchPointerEvent(graph, 'pointerdown', { clientX: 0, clientY: 100 })
    expect(graph.setPointerCapture).toHaveBeenCalledWith(1)

    events.movePoint.mockClear()
    dispatchPointerEvent(window, 'pointercancel')
    dispatchPointerEvent(window, 'pointermove', { clientX: 40, clientY: 40 })

    expect(graph.releasePointerCapture).toHaveBeenCalledWith(1)
    expect(events.movePoint).not.toHaveBeenCalled()
  })

  it('stops dragging when pointer capture is lost', () => {
    const { events, graph } = renderCurveGraph()

    dispatchPointerEvent(graph, 'pointerdown', { clientX: 0, clientY: 100 })
    events.movePoint.mockClear()

    dispatchPointerEvent(graph, 'lostpointercapture')
    dispatchPointerEvent(window, 'pointermove', { clientX: 40, clientY: 40 })

    expect(events.movePoint).not.toHaveBeenCalled()
  })

  it('emits keyboard move and remove events for focused user points', () => {
    const { events } = renderCurveGraph({
      points: normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }]),
      selectedPointId: 'mid',
    })
    const midPoint = screen.getByRole('button', { name: 'Curve point In 50 Out 50' })
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

    midPoint.focus()
    midPoint.dispatchEvent(arrowEvent)
    midPoint.dispatchEvent(backspaceEvent)

    expect(events.movePoint).toHaveBeenCalledWith('mid', { input: 50, output: 51 })
    expect(events.removePoint).toHaveBeenCalledWith('mid')
    expect(arrowEvent.defaultPrevented).toBe(true)
    expect(backspaceEvent.defaultPrevented).toBe(true)
  })

  it('focuses a clicked user point so Delete removes it without bubbling to node shortcuts', async () => {
    const { events, graph } = renderCurveGraph({
      points: normalizeCurveChannelPoints([{ id: 'mid', input: 50, output: 50 }]),
    })
    const midPoint = screen.getByRole('button', { name: 'Curve point In 50 Out 50' })
    const bubbledKeydown = vi.fn()
    graph.addEventListener('keydown', bubbledKeydown)

    dispatchPointerEvent(midPoint, 'pointerdown')
    events.removePoint.mockClear()
    await waitFor(() => expect(midPoint).toHaveFocus())
    document.activeElement?.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true }),
    )

    expect(events.removePoint).toHaveBeenCalledWith('mid')
    expect(bubbledKeydown).not.toHaveBeenCalled()
  })
})
