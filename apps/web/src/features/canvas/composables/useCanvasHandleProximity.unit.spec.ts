/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  provideCanvasHandleProximityCoordinator,
  useCanvasHandleProximity,
} from './useCanvasHandleProximity'
import { createCanvasHandleProximityCoordinator } from './canvasHandleProximityCoordinator'

function createAnchor(left = 100, top = 100) {
  const anchor = document.createElement('div')
  vi.spyOn(anchor, 'getBoundingClientRect').mockReturnValue({
    bottom: top + 1,
    height: 1,
    left,
    right: left + 1,
    top,
    width: 1,
    x: left,
    y: top,
    toJSON: () => undefined,
  })
  document.body.append(anchor)
  return anchor
}

function mountProximity(
  anchor: HTMLElement,
  direction: 'input' | 'output' = 'output',
  enabled = ref(true),
  coordinator = createCanvasHandleProximityCoordinator(),
) {
  let proximity!: ReturnType<typeof useCanvasHandleProximity>
  let surface!: ReturnType<typeof provideCanvasHandleProximityCoordinator>
  const host = document.createElement('div')
  document.body.append(host)
  const Consumer = defineComponent({
    setup() {
      proximity = useCanvasHandleProximity({
        direction,
        getAnchorRect: () => anchor.getBoundingClientRect(),
        enabled,
      })
      return () => null
    },
  })
  const Provider = defineComponent({
    setup() {
      surface = provideCanvasHandleProximityCoordinator(coordinator)
      return () => h(Consumer)
    },
  })
  const app = createApp(Provider)
  app.mount(host)

  return {
    enabled,
    proximity,
    surface,
    unmount: () => {
      app.unmount()
      host.remove()
      anchor.remove()
    },
  }
}

describe('useCanvasHandleProximity', () => {
  let frameCallback: FrameRequestCallback | null = null

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    frameCallback = null
  })

  function movePointer(clientX: number, clientY: number, pointerType = 'mouse') {
    window.dispatchEvent(new PointerEvent('pointermove', { clientX, clientY, pointerType }))
    flushFrame()
  }

  function flushFrame() {
    const pendingFrame = frameCallback as FrameRequestCallback | null
    frameCallback = null
    pendingFrame?.(0)
  }

  it('activates an outward sector from an external pointer position', () => {
    frameCallback = null
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity(createAnchor())

    movePointer(130, 100)

    expect(harness.proximity.isActive.value).toBe(true)
    expect(harness.proximity.point.value).toEqual({ x: 29.5, y: -0.5 })

    harness.unmount()
  })

  it('rejects pointer positions outside the outward 120 degree sector', () => {
    frameCallback = null
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity(createAnchor())

    movePointer(100, 130)

    expect(harness.proximity.isActive.value).toBe(false)
    harness.unmount()
  })

  it('coalesces repeated surface refresh requests into one frame', () => {
    frameCallback = null
    const requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      frameCallback = callback
      return 1
    })
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const coordinator = createCanvasHandleProximityCoordinator()
    const refresh = vi.spyOn(coordinator, 'refresh')
    const harness = mountProximity(createAnchor(), 'output', ref(true), coordinator)

    harness.surface.requestRefresh()
    harness.surface.requestRefresh()
    harness.surface.requestRefresh()

    expect(requestAnimationFrame).toHaveBeenCalledTimes(1)
    flushFrame()
    expect(refresh).toHaveBeenCalledTimes(1)

    harness.unmount()
  })

  it('releases the active handle when its enabled state changes', () => {
    frameCallback = null
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity(createAnchor())

    movePointer(130, 100)
    harness.enabled.value = false
    flushFrame()

    expect(harness.proximity.isActive.value).toBe(false)
    harness.unmount()
  })

  it('exposes one surface suspension control for all registrations', () => {
    frameCallback = null
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity(createAnchor())

    movePointer(130, 100)
    harness.surface.setSuspended(true)
    expect(harness.proximity.isActive.value).toBe(false)

    harness.surface.setSuspended(false)
    expect(harness.proximity.isActive.value).toBe(true)
    harness.unmount()
  })

  it('ignores touch pointer movement', () => {
    frameCallback = null
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity(createAnchor())

    movePointer(130, 100, 'touch')

    expect(harness.proximity.isActive.value).toBe(false)
    harness.unmount()
  })

  it('supports pen input and evaluates only the latest pointer in a frame', () => {
    frameCallback = null
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity(createAnchor())

    window.dispatchEvent(
      new PointerEvent('pointermove', { clientX: 120, clientY: 100, pointerType: 'pen' }),
    )
    window.dispatchEvent(
      new PointerEvent('pointermove', { clientX: 135, clientY: 105, pointerType: 'pen' }),
    )
    flushFrame()

    expect(harness.proximity.point.value).toEqual({ x: 34.5, y: 4.5 })
    harness.unmount()
  })

  it('removes the Canvas surface pointer listeners when its provider unmounts', () => {
    const removeEventListener = vi.spyOn(window, 'removeEventListener')
    const harness = mountProximity(createAnchor())

    harness.unmount()

    expect(removeEventListener).toHaveBeenCalledWith('pointermove', expect.any(Function), true)
    expect(removeEventListener).toHaveBeenCalledWith('pointerleave', expect.any(Function), true)
    expect(removeEventListener).toHaveBeenCalledWith('blur', expect.any(Function))
  })
})
