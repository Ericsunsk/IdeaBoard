import { describe, expect, it, vi } from 'vitest'
import { createCanvasHandleProximityCoordinator } from './canvasHandleProximityCoordinator'

function rect(left: number, top: number) {
  return { left, top, width: 1, height: 1 }
}

describe('Canvas handle proximity coordinator', () => {
  it('activates an output handle from its outward screen-space sector', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator()
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
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
    const coordinator = createCanvasHandleProximityCoordinator()
    coordinator.register({
      direction: 'input',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => true,
      onStateChange: inputState,
    })
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(300, 100),
      isEnabled: () => true,
      onStateChange: outputState,
    })

    coordinator.movePointer({ x: 70, y: 100 })
    expect(inputState).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: -30.5, y: -0.5 },
    })

    coordinator.movePointer({ x: 157, y: 100 })
    expect(inputState).toHaveBeenLastCalledWith({
      isActive: false,
      point: { x: 0, y: 0 },
    })
    expect(outputState).not.toHaveBeenCalledWith(expect.objectContaining({ isActive: true }))
  })

  it('keeps the active handle when equally near candidates overlap', () => {
    const firstState = vi.fn()
    const secondState = vi.fn()
    let secondLeft = 110
    const coordinator = createCanvasHandleProximityCoordinator()
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => true,
      onStateChange: firstState,
    })
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(secondLeft, 100),
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

  it('releases a handle immediately when it becomes disabled', () => {
    let isEnabled = true
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator()
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => isEnabled,
      onStateChange,
    })
    coordinator.movePointer({ x: 130, y: 100 })

    isEnabled = false
    coordinator.refresh()

    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: false,
      point: { x: 0, y: 0 },
    })
  })

  it('hands the current pointer to the next candidate when the winner unregisters', () => {
    const firstState = vi.fn()
    const secondState = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator()
    const unregisterFirst = coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => true,
      onStateChange: firstState,
    })
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
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

  it('removes every registration without notifying detached consumers when the surface clears', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator()
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => true,
      onStateChange,
    })
    coordinator.movePointer({ x: 130, y: 100 })

    coordinator.clear()
    coordinator.movePointer({ x: 135, y: 100 })

    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 29.5, y: -0.5 },
    })
    expect(onStateChange).toHaveBeenCalledTimes(1)
  })

  it('stops notifying a consumer after its final handle unregisters', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator()
    const unregister = coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => true,
      onStateChange,
    })
    coordinator.movePointer({ x: 130, y: 100 })

    unregister()
    coordinator.movePointer({ x: 135, y: 100 })

    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 29.5, y: -0.5 },
    })
    expect(onStateChange).toHaveBeenCalledTimes(1)
  })

  it('suspends the active handle and restores the latest pointer on resume', () => {
    const onStateChange = vi.fn()
    const coordinator = createCanvasHandleProximityCoordinator()
    coordinator.register({
      direction: 'output',
      getAnchorRect: () => rect(100, 100),
      isEnabled: () => true,
      onStateChange,
    })
    coordinator.movePointer({ x: 130, y: 110 })
    coordinator.setSuspended(true)
    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: false,
      point: { x: 0, y: 0 },
    })

    coordinator.movePointer({ x: 140, y: 105 })
    expect(onStateChange).toHaveBeenCalledTimes(2)

    coordinator.setSuspended(false)
    expect(onStateChange).toHaveBeenLastCalledWith({
      isActive: true,
      point: { x: 39.5, y: 4.5 },
    })
  })
})
