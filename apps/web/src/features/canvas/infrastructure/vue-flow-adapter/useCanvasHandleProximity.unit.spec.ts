/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  provideCanvasHandleProximityCoordinator,
  useCanvasHandleProximity,
} from './useCanvasHandleProximity'
import {
  createCanvasHandleProximityCoordinator,
  type CanvasHandleProximityGeometry,
} from './canvasHandleProximityCoordinator'

function createGeometry(): CanvasHandleProximityGeometry {
  return {
    prepare: vi.fn(),
    resolveAnchor: () => ({ x: 100.5, y: 100.5 }),
  }
}

function mountProximity(
  direction: 'input' | 'output' = 'output',
  enabled = ref(true),
  geometry = createGeometry(),
) {
  let proximity!: ReturnType<typeof useCanvasHandleProximity>
  let surface!: ReturnType<typeof provideCanvasHandleProximityCoordinator>
  const coordinator = createCanvasHandleProximityCoordinator(geometry)
  const host = document.createElement('div')
  document.body.append(host)
  const Consumer = defineComponent({
    setup() {
      proximity = useCanvasHandleProximity({ nodeId: 'node-1', direction, enabled })
      return () => null
    },
  })
  const Provider = defineComponent({
    setup() {
      surface = provideCanvasHandleProximityCoordinator(geometry, coordinator)
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
    delete window.__canvasInteractionBenchmarkCounters
    frameCallback = null
  })

  function installFrameScheduler() {
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        frameCallback = callback
        return 1
      }),
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
  }

  function flushFrame() {
    const pendingFrame = frameCallback
    frameCallback = null
    pendingFrame?.(0)
  }

  function movePointer(clientX: number, clientY: number, pointerType = 'mouse') {
    window.dispatchEvent(new PointerEvent('pointermove', { clientX, clientY, pointerType }))
    flushFrame()
  }

  it('activates an outward sector from a node identity and direction', () => {
    installFrameScheduler()
    window.__canvasInteractionBenchmarkCounters = {}
    const harness = mountProximity()

    movePointer(130, 100)

    expect(harness.proximity.isActive.value).toBe(true)
    expect(harness.proximity.point.value).toEqual({ x: 29.5, y: -0.5 })
    expect(window.__canvasInteractionBenchmarkCounters.handleGeometryLayoutReadCount ?? 0).toBe(0)
    harness.unmount()
  })

  it('coalesces repeated surface refresh requests into one frame', () => {
    const requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      frameCallback = callback
      return 1
    })
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const harness = mountProximity()

    harness.surface.requestRefresh()
    harness.surface.requestRefresh()
    harness.surface.requestRefresh()

    expect(requestAnimationFrame).toHaveBeenCalledTimes(1)
    flushFrame()
    harness.unmount()
  })

  it('refreshes stationary-pointer geometry after resize and scroll in one frame', () => {
    const requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      frameCallback = callback
      return 1
    })
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    const geometry = createGeometry()
    const harness = mountProximity('output', ref(true), geometry)

    movePointer(130, 100)
    vi.mocked(geometry.prepare).mockClear()
    requestAnimationFrame.mockClear()

    window.dispatchEvent(new Event('resize'))
    window.dispatchEvent(new Event('scroll'))

    expect(requestAnimationFrame).toHaveBeenCalledTimes(1)
    flushFrame()
    expect(geometry.prepare).toHaveBeenCalledTimes(1)
    harness.unmount()
  })

  it('releases the active handle when its enabled state changes', () => {
    installFrameScheduler()
    const harness = mountProximity()

    movePointer(130, 100)
    harness.enabled.value = false
    flushFrame()

    expect(harness.proximity.isActive.value).toBe(false)
    harness.unmount()
  })

  it('ignores touch pointer movement and cleans up surface listeners', () => {
    installFrameScheduler()
    const removeEventListener = vi.spyOn(window, 'removeEventListener')
    const harness = mountProximity()

    movePointer(130, 100, 'touch')
    expect(harness.proximity.isActive.value).toBe(false)

    harness.unmount()
    expect(removeEventListener).toHaveBeenCalledWith('pointermove', expect.any(Function), true)
    expect(removeEventListener).toHaveBeenCalledWith('pointerleave', expect.any(Function), true)
    expect(removeEventListener).toHaveBeenCalledWith('blur', expect.any(Function))
    expect(removeEventListener).toHaveBeenCalledWith('resize', expect.any(Function))
    expect(removeEventListener).toHaveBeenCalledWith('scroll', expect.any(Function), true)
  })

  it('cancels a pending geometry refresh when the Canvas scope is disposed', () => {
    installFrameScheduler()
    const cancelAnimationFrame = vi.mocked(window.cancelAnimationFrame)
    const harness = mountProximity()

    harness.surface.requestRefresh()
    harness.unmount()

    expect(cancelAnimationFrame).toHaveBeenCalledWith(1)
  })
})
