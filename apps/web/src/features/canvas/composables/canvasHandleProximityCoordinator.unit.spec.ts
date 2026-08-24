import { describe, expect, it, vi } from 'vitest'
import {
  createCanvasHandleProximityCoordinator,
  type CanvasHandleProximityGeometry,
  type CanvasHandleProximityPoint,
} from './canvasHandleProximityCoordinator'

function point(left: number, top: number): CanvasHandleProximityPoint {
  return { x: left + 0.5, y: top + 0.5 }
}

function geometry(
  resolveAnchor: (nodeId: string) => CanvasHandleProximityPoint | null,
): CanvasHandleProximityGeometry {
  return {
    prepare: vi.fn(),
    resolveAnchor: (nodeId) => resolveAnchor(nodeId),
  }
}

describe('Canvas handle proximity coordinator', () => {
  it('activates an output handle from its outward screen-space sector', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator(geometry(() => point(100, 100)))
    coordinator.register({
      nodeId: 'node-1',
      direction: 'output',
      isEnabled: () => true,
      onStateChange,
    })

    coordinator.movePointer({ x: 130, y: 100 })

    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 29.5, y: -0.5 },
    })
  })

  it('mirrors the outward sector for input handles and rejects points outside the radius', () => {
    const inputState = vi.fn()
    const outputState = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator(
      geometry((nodeId) => (nodeId === 'input' ? point(100, 100) : point(300, 100))),
    )
    coordinator.register({
      nodeId: 'input',
      direction: 'input',
      isEnabled: () => true,
      onStateChange: inputState,
    })
    coordinator.register({
      nodeId: 'output',
      direction: 'output',
      isEnabled: () => true,
      onStateChange: outputState,
    })

    coordinator.movePointer({ x: 70, y: 100 })
    expect(inputState).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: -30.5, y: -0.5 },
    })

    coordinator.movePointer({ x: 157, y: 100 })
    expect(inputState).toHaveBeenLastCalledWith({ isActive: false, point: { x: 0, y: 0 } })
    expect(outputState).not.toHaveBeenCalledWith(expect.objectContaining({ isActive: true }))
  })

  it('keeps the active handle when equally near candidates overlap', () => {
    const firstState = vi.fn()
    const secondState = vi.fn()
    let secondLeft = 110
    const coordinator = createCanvasHandleProximityCoordinator(
      geometry((nodeId) => (nodeId === 'first' ? point(100, 100) : point(secondLeft, 100))),
    )
    coordinator.register({
      nodeId: 'first',
      direction: 'output',
      isEnabled: () => true,
      onStateChange: firstState,
    })
    coordinator.register({
      nodeId: 'second',
      direction: 'output',
      isEnabled: () => true,
      onStateChange: secondState,
    })

    coordinator.movePointer({ x: 130, y: 100 })
    expect(secondState).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 19.5, y: -0.5 },
    })

    secondLeft = 100
    coordinator.refresh()

    expect(secondState).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 29.5, y: -0.5 },
    })
    expect(firstState).not.toHaveBeenCalledWith(expect.objectContaining({ isActive: true }))
  })

  it('skips nodes without usable Vue Flow geometry', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator(geometry(() => null))
    coordinator.register({
      nodeId: 'unmeasured-node',
      direction: 'output',
      isEnabled: () => true,
      onStateChange,
    })

    coordinator.movePointer({ x: 130, y: 100 })

    expect(onStateChange).not.toHaveBeenCalled()
  })

  it('hands the current pointer to the next candidate when the winner unregisters', () => {
    const firstState = vi.fn()
    const secondState = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator(geometry(() => point(100, 100)))
    const unregisterFirst = coordinator.register({
      nodeId: 'first',
      direction: 'output',
      isEnabled: () => true,
      onStateChange: firstState,
    })
    coordinator.register({
      nodeId: 'second',
      direction: 'output',
      isEnabled: () => true,
      onStateChange: secondState,
    })
    coordinator.movePointer({ x: 130, y: 100 })

    unregisterFirst()

    expect(firstState).toHaveBeenCalledTimes(1)
    expect(secondState).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 29.5, y: -0.5 },
    })
  })

  it('suspends the active handle and restores the latest pointer on resume', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator(geometry(() => point(100, 100)))
    coordinator.register({
      nodeId: 'node-1',
      direction: 'output',
      isEnabled: () => true,
      onStateChange,
    })
    coordinator.movePointer({ x: 130, y: 110 })
    coordinator.setSuspended(true)
    expect(onStateChange).toHaveBeenLastCalledWith({ isActive: false, point: { x: 0, y: 0 } })

    coordinator.movePointer({ x: 140, y: 105 })
    expect(onStateChange).toHaveBeenCalledTimes(2)

    coordinator.setSuspended(false)
    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 39.5, y: 4.5 },
    })
  })
})
