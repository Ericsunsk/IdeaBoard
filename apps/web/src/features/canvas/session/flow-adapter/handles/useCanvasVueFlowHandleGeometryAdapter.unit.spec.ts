import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createCanvasHandleProximityCoordinator } from './canvasHandleProximityCoordinator'

const vueFlowMock = vi.hoisted(() => ({
  findNode: vi.fn(),
  viewport: { value: { x: 20, y: 30, zoom: 2 } },
  vueFlowRef: { value: null as HTMLElement | null },
}))

vi.mock('@vue-flow/core', () => ({
  useVueFlow: () => vueFlowMock,
}))

import { useCanvasVueFlowHandleGeometryAdapter } from './useCanvasVueFlowHandleGeometryAdapter'

function createSurface() {
  const surface = document.createElement('div')
  vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
    bottom: 220,
    height: 200,
    left: 10,
    right: 410,
    top: 20,
    width: 400,
    x: 10,
    y: 20,
    toJSON: () => undefined,
  })
  return surface
}

describe('useCanvasVueFlowHandleGeometryAdapter', () => {
  beforeEach(() => {
    vueFlowMock.findNode.mockReset()
    vueFlowMock.viewport.value = { x: 20, y: 30, zoom: 2 }
    vueFlowMock.vueFlowRef.value = createSurface()
  })

  it('projects aggregate anchors from Vue Flow node geometry and the active viewport', () => {
    vueFlowMock.findNode.mockReturnValue({
      computedPosition: { x: 100, y: 50, z: 0 },
      dimensions: { width: 80, height: 40 },
    })
    const adapter = useCanvasVueFlowHandleGeometryAdapter()

    adapter.prepare()

    expect(adapter.resolveAnchor('node-1', 'input')).toEqual({ x: 230, y: 190 })
    expect(adapter.resolveAnchor('node-1', 'output')).toEqual({ x: 390, y: 190 })
  })

  it('reads the Vue Flow surface once per evaluation, not once per Handle', () => {
    vueFlowMock.findNode.mockReturnValue({
      computedPosition: { x: 0, y: 0, z: 0 },
      dimensions: { width: 80, height: 40 },
    })
    const adapter = useCanvasVueFlowHandleGeometryAdapter()
    const surface = vueFlowMock.vueFlowRef.value as HTMLElement
    const getBoundingClientRect = vi.mocked(surface.getBoundingClientRect)

    adapter.prepare()
    adapter.resolveAnchor('first', 'input')
    adapter.resolveAnchor('second', 'output')

    expect(getBoundingClientRect).toHaveBeenCalledTimes(1)
  })

  it('prepares one shared surface snapshot for multiple coordinator registrations', () => {
    vueFlowMock.findNode.mockReturnValue({
      computedPosition: { x: 0, y: 0, z: 0 },
      dimensions: { width: 80, height: 40 },
    })
    const adapter = useCanvasVueFlowHandleGeometryAdapter()
    const prepare = vi.spyOn(adapter, 'prepare')
    const surface = vueFlowMock.vueFlowRef.value as HTMLElement
    const getBoundingClientRect = vi.mocked(surface.getBoundingClientRect)
    const coordinator = createCanvasHandleProximityCoordinator(adapter)
    coordinator.register({
      nodeId: 'first',
      direction: 'input',
      isEnabled: () => true,
      onStateChange: vi.fn(),
    })
    coordinator.register({
      nodeId: 'second',
      direction: 'output',
      isEnabled: () => true,
      onStateChange: vi.fn(),
    })

    coordinator.movePointer({ x: 190, y: 90 })

    expect(prepare).toHaveBeenCalledTimes(1)
    expect(getBoundingClientRect).toHaveBeenCalledTimes(1)
  })

  it('does not expose an anchor for unmeasured nodes', () => {
    vueFlowMock.findNode.mockReturnValue({
      computedPosition: { x: 0, y: 0, z: 0 },
      dimensions: { width: 0, height: 0 },
    })
    const adapter = useCanvasVueFlowHandleGeometryAdapter()

    adapter.prepare()

    expect(adapter.resolveAnchor('unmeasured', 'output')).toBeNull()
  })
})
