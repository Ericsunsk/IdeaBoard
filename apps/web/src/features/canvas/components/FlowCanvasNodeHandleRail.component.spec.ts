/* eslint-disable vue/one-component-per-file */
import { h, defineComponent, nextTick, reactive, ref } from 'vue'
import { cleanup } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '../../../test/render'
import { createCanvasHandleProximityCoordinator } from '../composables/canvasHandleProximityCoordinator'
import { provideCanvasHandleProximityCoordinator } from '../composables/useCanvasHandleProximity'
import { provideCanvasViewportContext } from '../composables/useCanvasViewportContext'
import FlowCanvasNodeHandleRail from './FlowCanvasNodeHandleRail.vue'

vi.mock('@vue-flow/core', () => ({
  Handle: defineComponent({
    inheritAttrs: false,
    setup(_, { attrs, slots }) {
      return () => h('div', attrs, slots.default?.())
    },
  }),
  Position: { Left: 'left', Right: 'right' },
}))

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
  selected?: boolean
  zoomLevel?: number
}) {
  const coordinator = createCanvasHandleProximityCoordinator()
  const currentProps = reactive<{
    kind: 'prompt' | 'image' | 'fusion'
    disabled: boolean
    selected: boolean
  }>({
    kind: props.kind,
    disabled: props.disabled ?? false,
    selected: props.selected ?? false,
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
    document.body.innerHTML = ''
  })

  it('keeps handles hidden without node hover, selection, or proximity activation', () => {
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

  it('keeps handles visible for a selected node after the pointer leaves', () => {
    const view = renderHandles({ kind: 'prompt', selected: true })

    const inputHandle = view.getByLabelText('提示词节点输入连接点，拖拽创建连接')
    const outputHandle = view.getByLabelText('提示词节点输出连接点，拖拽创建连接')

    expect(inputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(inputHandle).toHaveAttribute('tabindex', '0')
    expect(outputHandle).toHaveAttribute('tabindex', '0')
  })

  it('keeps disabled handles hidden even when the node is selected or in range', async () => {
    const view = renderHandles({ kind: 'prompt', selected: true, disabled: true })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

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

  it('freezes follow movement while Vue Flow owns a connection drag', async () => {
    const view = renderHandles({ kind: 'prompt' })
    const outputHandle = view.getByLabelText(
      '提示词节点输出连接点，拖拽创建连接',
    ) as HTMLElement
    setAnchorRect(outputHandle, 100)

    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()
    expect(outputHandle.style.getPropertyValue('--handle-plus-x')).toBe('29.5px')

    outputHandle.dispatchEvent(
      new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerType: 'mouse' }),
    )
    view.coordinator.movePointer({ x: 136, y: 106 })
    await nextTick()

    expect(outputHandle).not.toHaveAttribute('aria-hidden')
    expect(outputHandle.style.getPropertyValue('--handle-plus-x')).toBe('')
    expect(outputHandle.style.getPropertyValue('--handle-plus-y')).toBe('')

    window.dispatchEvent(new PointerEvent('pointerup', { pointerType: 'mouse' }))
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
    { zoomLevel: 50, expectedX: 59, expectedLocalHitSize: 104 },
    { zoomLevel: 200, expectedX: 14.75, expectedLocalHitSize: 26 },
  ])(
    'keeps the visual center and hit area screen-sized at $zoomLevel% zoom',
    async ({ zoomLevel, expectedX, expectedLocalHitSize }) => {
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
      const localHitSize = Number.parseFloat(
        outputHandle.style.getPropertyValue('--handle-hit-size'),
      )
      expect(localHitSize).toBeCloseTo(expectedLocalHitSize)
      expect(localHitSize * (zoomLevel / 100)).toBeCloseTo(52)
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
