import { defineComponent, h, nextTick, reactive, ref } from 'vue'
import { cleanup } from '@testing-library/vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '../../../../test/render'
import { createCanvasHandleProximityCoordinator } from '../../session/flow-adapter/handles/canvasHandleProximityCoordinator'
import { provideCanvasHandleProximityCoordinator } from '../../session/flow-adapter/handles/useCanvasHandleProximity'
import { provideCanvasViewportContext } from '../../session/flow-adapter/viewport/useCanvasViewportContext'
import FlowCanvasNodeHandleRail from './FlowCanvasNodeHandleRail.vue'

const vueFlowMock = vi.hoisted(() => ({
  connectionStartHandle: null as { value: unknown } | null,
}))

vi.mock('@vue-flow/core', async () => {
  const {
    defineComponent: defineVueComponent,
    h: renderElement,
    ref: createRef,
  } = await import('vue')
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

type NodeKind = 'prompt' | 'image' | 'fusion'
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

function renderHandles(props: { kind: NodeKind; disabled?: boolean }) {
  const geometry = {
    prepare: () => undefined,
    resolveAnchor: () => ({ x: 100.5, y: 100.5 }),
  }
  const coordinator = createCanvasHandleProximityCoordinator(geometry)
  const currentProps = reactive({
    nodeId: 'node-1',
    kind: props.kind,
    disabled: props.disabled ?? false,
    dragging: false,
  })
  const interactionRoot = ref<HTMLElement | null>(null)
  const zoomLevel = ref(100)
  let proximitySurface: ReturnType<typeof provideCanvasHandleProximityCoordinator> | null = null

  const Host = defineComponent({
    setup() {
      proximitySurface = provideCanvasHandleProximityCoordinator(geometry, coordinator)
      provideCanvasViewportContext({ zoomLevel })
      return () =>
        h('div', { ref: interactionRoot, 'data-testid': 'interaction-root' }, [
          h(FlowCanvasNodeHandleRail, {
            ...currentProps,
            interactionRoot: interactionRoot.value,
          }),
        ])
    },
  })
  const view = render(Host)

  return {
    ...view,
    coordinator,
    interactionRoot: view.getByTestId('interaction-root'),
    setProximitySuspended: (suspended: boolean) => proximitySurface?.setSuspended(suspended),
  }
}

function getAggregateHandles(container: Element) {
  return [...container.querySelectorAll<HTMLElement>('.flow-canvas-node__handle-aggregate')]
}

describe('FlowCanvasNodeHandleRail', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    setConnectionStartHandle(null)
  })

  it('shows handles on node hover and hides them again on leave', async () => {
    const view = renderHandles({ kind: 'prompt' })
    const handles = getAggregateHandles(view.container)

    expect(handles).toHaveLength(2)
    for (const handle of handles) {
      expect(handle).toHaveAttribute('aria-hidden', 'true')
      expect(handle).toHaveAttribute('tabindex', '-1')
    }

    await nextTick()
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))
    await nextTick()
    for (const handle of handles) {
      expect(handle).not.toHaveAttribute('aria-hidden')
      expect(handle).toHaveAttribute('tabindex', '0')
    }

    view.interactionRoot.dispatchEvent(new MouseEvent('mouseleave'))
    await nextTick()
    for (const handle of handles) {
      expect(handle).toHaveAttribute('aria-hidden', 'true')
      expect(handle).toHaveAttribute('tabindex', '-1')
    }
  })

  it('keeps disabled handles hidden and non-connectable', async () => {
    const view = renderHandles({ kind: 'prompt', disabled: true })

    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))
    view.coordinator.movePointer({ x: 130, y: 100 })
    await nextTick()

    for (const handle of getAggregateHandles(view.container)) {
      expect(handle).toHaveAttribute('aria-hidden', 'true')
      expect(handle).toHaveAttribute('tabindex', '-1')
      expect(handle).toHaveAttribute('connectable', 'false')
    }
  })

  it('keeps both handles mounted but hides them during a connection drag', async () => {
    const view = renderHandles({ kind: 'prompt' })
    vi.spyOn(view.interactionRoot, 'matches').mockImplementation(
      (selector: string) => selector === ':hover',
    )
    view.interactionRoot.dispatchEvent(new MouseEvent('mouseenter'))
    await nextTick()

    setConnectionStartHandle({
      id: 'out:prompt',
      nodeId: 'node-1',
      type: 'source',
      position: 'right',
      x: 100,
      y: 100,
    })
    view.setProximitySuspended(true)
    await nextTick()

    const handles = getAggregateHandles(view.container)
    expect(handles).toHaveLength(2)
    const inputHandle = view.container.querySelector<HTMLElement>('#in\\:__auto')
    const outputHandle = view.container.querySelector<HTMLElement>('#out\\:image')

    expect(inputHandle).toBeDefined()
    expect(outputHandle).toBeDefined()
    expect(inputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(inputHandle).toHaveAttribute('tabindex', '-1')
    expect(inputHandle).toHaveAttribute('connectable-end', 'false')
    expect(inputHandle).toHaveStyle({
      left: '0',
      top: '50%',
      transform: 'translateY(-50%)',
    })
    expect(outputHandle).toHaveAttribute('aria-hidden', 'true')
    expect(outputHandle).toHaveAttribute('tabindex', '-1')
    for (const handle of handles) {
      expect(handle.querySelector('.flow-canvas-node__handle-plus')).toBeInTheDocument()
    }

    setConnectionStartHandle(null)
    view.setProximitySuspended(false)
    await nextTick()
    for (const handle of handles) {
      expect(handle).not.toHaveAttribute('aria-hidden')
      expect(handle).toHaveAttribute('tabindex', '0')
    }
  })

  it.each<{
    kind: NodeKind
    expected: Array<{
      id: string
      type: string
      connectable: string | null
      connectableStart: string | null
      connectableEnd: string | null
    }>
  }>([
    {
      kind: 'image',
      expected: [
        {
          id: 'in:image',
          type: 'target',
          connectable: 'false',
          connectableStart: null,
          connectableEnd: null,
        },
        {
          id: 'in:__auto',
          type: 'target',
          connectable: 'true',
          connectableStart: 'false',
          connectableEnd: 'false',
        },
        {
          id: 'out:image',
          type: 'source',
          connectable: 'true',
          connectableStart: 'true',
          connectableEnd: 'false',
        },
      ],
    },
    {
      kind: 'prompt',
      expected: [
        {
          id: 'in:image',
          type: 'target',
          connectable: 'false',
          connectableStart: null,
          connectableEnd: null,
        },
        {
          id: 'out:prompt',
          type: 'source',
          connectable: 'false',
          connectableStart: null,
          connectableEnd: null,
        },
        {
          id: 'in:__auto',
          type: 'target',
          connectable: 'true',
          connectableStart: 'false',
          connectableEnd: 'false',
        },
        {
          id: 'out:image',
          type: 'source',
          connectable: 'true',
          connectableStart: 'true',
          connectableEnd: 'false',
        },
      ],
    },
    {
      kind: 'fusion',
      expected: [
        {
          id: 'in:image',
          type: 'target',
          connectable: 'false',
          connectableStart: null,
          connectableEnd: null,
        },
        {
          id: 'in:prompt',
          type: 'target',
          connectable: 'false',
          connectableStart: null,
          connectableEnd: null,
        },
        {
          id: 'in:__auto',
          type: 'target',
          connectable: 'true',
          connectableStart: 'false',
          connectableEnd: 'false',
        },
        {
          id: 'out:image',
          type: 'source',
          connectable: 'true',
          connectableStart: 'true',
          connectableEnd: 'false',
        },
      ],
    },
  ])('projects the canonical $kind handle set', ({ kind, expected }) => {
    const view = renderHandles({ kind })
    const handles = [...view.container.querySelectorAll<HTMLElement>('[id^="in:"], [id^="out:"]')]

    expect(
      handles.map((handle) => ({
        id: handle.id,
        type: handle.getAttribute('type'),
        connectable: handle.getAttribute('connectable'),
        connectableStart: handle.getAttribute('connectable-start'),
        connectableEnd: handle.getAttribute('connectable-end'),
      })),
    ).toEqual(expected)
  })
})
