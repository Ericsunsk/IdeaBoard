import { createApp, defineComponent, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlowCanvasNodeHandleRail } from './useFlowCanvasNodeHandleRail'

type HandleRailHarness = {
  rail: ReturnType<typeof useFlowCanvasNodeHandleRail>
  flowNodeRef: ReturnType<typeof ref<HTMLElement | null>>
  unmount: () => void
}

function mountHandleRail(initialRoot: HTMLElement | null): HandleRailHarness {
  const flowNodeRef = ref<HTMLElement | null>(initialRoot)
  let rail!: ReturnType<typeof useFlowCanvasNodeHandleRail>
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        rail = useFlowCanvasNodeHandleRail(flowNodeRef)
        return () => null
      },
    }),
  )
  app.mount(host)

  return {
    rail,
    flowNodeRef,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function createRootWithFocusableHandle() {
  const root = document.createElement('div')
  const handle = document.createElement('button')
  handle.type = 'button'
  handle.className = 'flow-canvas-node__handle-aggregate'
  root.append(handle)
  document.body.append(root)
  return { root, handle }
}

describe('useFlowCanvasNodeHandleRail', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('shows handles while the pointer is over the node', () => {
    const { root } = createRootWithFocusableHandle()
    const harness = mountHandleRail(root)

    try {
      expect(harness.rail.showHandleControls.value).toBe(false)

      harness.rail.onNodeMouseEnter()
      expect(harness.rail.showHandleControls.value).toBe(true)

      harness.rail.onNodeMouseLeave()
      expect(harness.rail.showHandleControls.value).toBe(false)
    } finally {
      harness.unmount()
    }
  })

  it('keeps handles visible while keyboard focus remains inside the node', () => {
    const { root, handle } = createRootWithFocusableHandle()
    const harness = mountHandleRail(root)

    try {
      vi.spyOn(root, 'querySelector').mockImplementation((selector: string) =>
        selector === ':focus-visible' ? handle : null,
      )

      harness.rail.onNodeFocusIn()

      expect(harness.rail.showHandleControls.value).toBe(true)

      harness.rail.onNodeFocusOut(new FocusEvent('focusout', { relatedTarget: document.body }))
      expect(harness.rail.showHandleControls.value).toBe(false)
    } finally {
      harness.unmount()
    }
  })

  it('resyncs focus when focus moves within the same node', () => {
    const { root, handle } = createRootWithFocusableHandle()
    const nextFocusedElement = document.createElement('button')
    root.append(nextFocusedElement)
    const harness = mountHandleRail(root)

    try {
      vi.spyOn(root, 'querySelector').mockImplementation((selector: string) =>
        selector === ':focus-visible' ? handle : null,
      )

      harness.rail.onNodeFocusOut(new FocusEvent('focusout', { relatedTarget: nextFocusedElement }))
      expect(harness.rail.showHandleControls.value).toBe(true)
    } finally {
      harness.unmount()
    }
  })

  it('shows touch handles transiently and clears the timer on unmount', () => {
    vi.useFakeTimers()
    const { root } = createRootWithFocusableHandle()
    const harness = mountHandleRail(root)

    harness.rail.onNodePointerDown({ pointerType: 'touch' } as PointerEvent)

    expect(harness.rail.showHandleControls.value).toBe(true)
    expect(vi.getTimerCount()).toBe(1)

    harness.unmount()

    expect(harness.rail.showHandleControls.value).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
    vi.runOnlyPendingTimers()
    expect(harness.rail.showHandleControls.value).toBe(false)
  })

  it('refreshes transient visibility and blurs a focused aggregate handle', () => {
    const { root, handle } = createRootWithFocusableHandle()
    const harness = mountHandleRail(root)

    try {
      vi.spyOn(root, 'matches').mockImplementation((selector: string) => selector === ':hover')
      vi.spyOn(root, 'querySelector').mockImplementation(() => null)
      const blur = vi.spyOn(handle, 'blur')
      handle.focus()

      harness.rail.refreshTransientHandleVisibility()
      expect(blur).toHaveBeenCalledTimes(1)

      expect(harness.rail.showHandleControls.value).toBe(true)
    } finally {
      harness.unmount()
    }
  })
})
