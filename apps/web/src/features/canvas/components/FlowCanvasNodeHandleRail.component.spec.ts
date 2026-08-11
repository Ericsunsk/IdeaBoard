import { h, defineComponent, nextTick, reactive, ref } from 'vue'
import { cleanup } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '../../../test/render'
import { createCanvasHandleProximityCoordinator } from '../composables/canvasHandleProximityCoordinator'
import { provideCanvasHandleProximityCoordinator } from '../composables/useCanvasHandleProximity'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import FlowCanvasNodeHandleRail from './FlowCanvasNodeHandleRail.vue'

const vueFlowMock = vi.hoisted(() => ({
  connectionStartHandle: null as { value: unknown } | null,
}))

vi.mock('@vue-flow/core', async () => {
  const { defineComponent: defineVueComponent, h: renderElement, ref: createRef } =
    await import('vue')
  const connectionStartHandle = createRef(null)
  vueFlowMock.connectionStartHandle = connectionStartHandle

  return {
    Handle: defineVueComponent({
      inheritAttrs: false,
      setup(_, { attrs, slots }) {
        return () => renderElement('div', attrs, slots.default?.())
      },
    }),
    Position: { Left: 'left', Right: 'right' },
    useVueFlow: () => ({ connectionStartHandle }),
  }
})

type MockConnectingHandle = {
  id: string
  nodeId: string
  type: 'source' | 'target'
  position: 'left' | 'right'
  x: number
  y: number
}

function setConnectionStartHandle(value: MockConnectingHandle | null) {
  if (!vueFlowMock.connectionStartHandle) throw new Error('Vue Flow mock is not initialized')
  vueFlowMock.connectionStartHandle.value = value
}

function setAnchorRect(element: HTMLElement, left: number) {
  let currentLeft = left
  vi.spyOn(element, 'getBoundingClientRect').mockImplementation(() => ({
    bottom: 101,
    height: 1,
    left: currentLeft,
    right: currentLeft + 1,
    top: 100,
    width: 1,
    x: currentLeft,
    y: 100,
    toJSON: () => undefined,
  }))
  return (nextLeft: number) => {
    currentLeft = nextLeft
  }
}

function renderHandles(props: {
  kind: 'prompt' | 'image' | 'fusion'
  disabled?: boolean
  zoomLevel?: number
}) {
  const coordinator = createCanvasHandleProximityCoordinator()
  const currentProps = reactive<{
    kind: 'prompt' | 'image' | 'fusion'
    disabled: boolean
  }>({
    kind: props.kind,
    disabled: props.disabled ?? false,
  })
  const interactionRoot = ref<HTMLElement | null>(null)
  const zoomLevel = ref(props.zoomLevel ?? 100)
  const onRequestInternalsUpdate = vi.fn()
  const Host = defineComponent({
    setup() {
      provideCanvasHandleProximityCoordinator(coordinator)
      provideCanvasViewportContext({ zoomLevel })
      return () =>
        h('div', { ref: interactionRoot, 'data-testid': 'interaction-root' }, [
          h(FlowCanvasNodeHandleRail, {
            ...currentProps,
            interactionRoot: interactionRoot.value,
            onRequestInternalsUpdate,
          }),
        ])
    },
  })
  const view = render(Host)

  return {
    ...view,
    coordinator,
    interactionRoot: view.getByTestId('interaction-root'),
    onRequestInternalsUpdate,
    zoomLevel,
    async rerender(nextProps: Partial<typeof props>) {
      Object.assign(currentProps, nextProps)
      await nextTick()
    },
  }
}

describe('FlowCanvasNodeHandleRail', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    setConnectionStartHandle(null)
    document.body.innerHTML = ''
  })

  it('keeps handles hidden without node hover or proximity activation', () => {
    const view = renderHandles({
      kind: 'prompt',
    })

    const inputHandle = view.getByLabelText('提示词节点输入连接点，拖拽创建连接')
    const outputHandle = view.getByLabelText('提示词节点输出连接点，拖拽创建连接')

    expect(inputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(outputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(inputHandle).toHaveAttribute('tabindex', '-1')
    expect(outputHandle).toHaveAttribute('tabindex', '-1')
  })

  it('shows handles only while the pointer is over the node', async () => {
    const view = renderHandles({ kind: 'prompt' })

    const inputHandle = view.getByLabelText('提示词节点输入连接点，拖拽创建连接')
    const outputHandle = view.getByLabelText('提示词节点输出连接点，拖拽创建连接')

    await nextTick()
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))
    await nextTick()
    expect(inputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(inputHandle).toHaveAttribute('tabindex', '0')
    expect(outputHandle).toHaveAttribute('tabindex', '0')

    view.interactionRoot.dispatchEvent(new MouseEvent('mouseleave'))
    await nextTick()
    expect(inputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(outputHandle).toHaveAttribute('aria-hidden', 'true')
  })

  it('keeps disabled handles hidden even when the node is hovered or in range', async () => {
    const view = renderHandles({ kind: 'prompt', disabled: true })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

    await nextTick()
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))
    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()

    expect(outputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(outputHandle).toHaveAttribute('tabindex', '-1')
  })

  it('moves only the visual handle when an external pointer enters the output sector', async () => {
    const view = renderHandles({
      kind: 'prompt',
    })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()

    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle).toHaveAttribute('style', expect.stringContaining('--handle-plus-x: 29.5px'))
    expect(outputHandle).toHaveAttribute('style', expect.stringContaining('--handle-plus-y: -0.5px'))
    expect(outputHandle.querySelector('span')).toBeInTheDocument()
    expect(outputHandle).toHaveAttribute('tabindex', '0')
  })

  it('keeps following while node hover visibility changes inside the active sector', async () => {
    const view = renderHandles({
      kind: 'prompt',
    })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()
    await nextTick()
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))

    view.coordinator.movePointer({ x: 136, y: 106 })
    await nextTick()

    expect(outputHandle).toHaveAttribute(
      'style',
      expect.stringContaining('--handle-plus-x: 35.5px'),
    )
    expect(outputHandle).toHaveAttribute(
      'style',
      expect.stringContaining('--handle-plus-y: 5.5px'),
    )
  })

  it('keeps both handles mounted but hides their presentation during a connection drag', async () => {
    const view = renderHandles({ kind: 'prompt' })
    const inputHandle = view.getByLabelText(
      '提示词节点输入连接点，拖拽创建连接',
    ) as HTMLElement
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

    await nextTick()
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))
    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()
    expect(outputHandle.style.getPropertyValue('--handle-plus-x')).toBe('29.5px')
    setConnectionStartHandle({
      id: 'out:prompt',
      nodeId: 'node-1',
      type: 'source',
      position: 'right',
      x: 100,
      y: 100,
    })
    view.coordinator.movePointer({ x: 136, y: 106 })
    await nextTick()

    expect(outputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(outputHandle).toHaveAttribute('tabindex', '-1')
    expect(inputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(inputHandle).toHaveAttribute('tabindex', '-1')
    expect(outputHandle.querySelector('.flow-canvas-node__handle-plus')).toBeInTheDocument()
    expect(inputHandle.querySelector('.flow-canvas-node__handle-plus')).toBeInTheDocument()
    expect(outputHandle.style.getPropertyValue('--handle-plus-x')).toBe('')
    expect(outputHandle.style.getPropertyValue('--handle-plus-y')).toBe('')

    setConnectionStartHandle(null)
    await nextTick()
    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle).toHaveAttribute('tabindex', '0')
    expect(inputHandle).not.toHaveAttribute('aria-hidden')
    expect(inputHandle).toHaveAttribute('tabindex', '0')
  })

  it('keeps the original handle positions while the node itself is hovered', async () => {
    const view = renderHandles({
      kind: 'prompt',
    })
    const inputHandle = view.getByLabelText(
      '提示词节点输入连接点，拖拽创建连接',
    ) as HTMLElement
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(inputHandle, 160)
    setAnchorRect(outputHandle, 240)
    await nextTick()
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))

    // A pointer inside the node lies behind each outward-facing sector.
    view.coordinator.movePointer({ x: 200, y: 100 })
    await nextTick()

    expect(inputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(inputHandle).toHaveAttribute('tabindex', '0')
    expect(outputHandle).toHaveAttribute('tabindex', '0')
    expect(inputHandle).toHaveAttribute('style', expect.not.stringContaining('--handle-plus-x'))
    expect(outputHandle).toHaveAttribute('style', expect.not.stringContaining('--handle-plus-x'))
    expect(inputHandle).toHaveAttribute('style', expect.not.stringContaining('--handle-plus-y'))
    expect(outputHandle).toHaveAttribute('style', expect.not.stringContaining('--handle-plus-y'))
  })

  it.each(['image', 'fusion'] as const)(
    'requests Vue Flow internals refresh when projected ports change to %s',
    async (kind) => {
      const view = renderHandles({ kind: 'prompt' })

      await view.rerender({ kind })

      expect(view.onRequestInternalsUpdate).toHaveBeenCalledTimes(1)
    },
  )

  it.each([
    { zoomLevel: 50, expectedX: 59 },
    { zoomLevel: 200, expectedX: 14.75 },
  ])(
    'keeps the followed visual center screen-aligned at $zoomLevel% zoom',
    async ({ zoomLevel, expectedX }) => {
      const view = renderHandles({ kind: 'prompt', zoomLevel })
      const outputHandle = view.getByLabelText(
        '提示词节点输出连接点，拖拽创建连接',
      ) as HTMLElement
      setAnchorRect(outputHandle, 100)

      view.coordinator.movePointer({ x: 130, y: 100 })
      await nextTick()

      expect(Number.parseFloat(outputHandle.style.getPropertyValue('--handle-plus-x'))).toBeCloseTo(
        expectedX,
      )
      expect(outputHandle.style.right).toBe('0px')
    },
  )

  it('remeasures the fixed anchor when zoom changes under a stationary pointer', async () => {
    const view = renderHandles({ kind: 'prompt' })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    const moveAnchor = setAnchorRect(outputHandle, 100)

    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()
    expect(outputHandle.style.getPropertyValue('--handle-plus-x')).toBe('29.5px')

    moveAnchor(110)
    view.zoomLevel.value = 200
    await nextTick()
    await nextTick()

    expect(Number.parseFloat(outputHandle.style.getPropertyValue('--handle-plus-x'))).toBeCloseTo(
      9.75,
    )
  })

  it('keeps the handle visible until both axes finish returning', async () => {
    vi.useFakeTimers()
    const view = renderHandles({ kind: 'prompt' })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

    view.coordinator.movePointer({ x: 130, y: 110 })
    await nextTick()
    view.coordinator.movePointer({ x: 200, y: 100 })
    await nextTick()

    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle.style.getPropertyValue('--handle-plus-x')).toBe('')
    expect(outputHandle.style.getPropertyValue('--handle-plus-y')).toBe('')

    vi.advanceTimersByTime(160)
    await nextTick()
    expect(outputHandle).not.toHaveAttribute('aria-hidden')

    vi.advanceTimersByTime(20)
    await nextTick()
    expect(outputHandle).toHaveAttribute('aria-hidden', 'true')
  })
})
