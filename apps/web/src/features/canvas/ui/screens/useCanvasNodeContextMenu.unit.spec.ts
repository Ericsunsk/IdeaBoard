import { describe, expect, it, vi } from 'vitest'
import { useCanvasNodeContextMenu } from './useCanvasNodeContextMenu'

describe('useCanvasNodeContextMenu', () => {
  it('keeps the surface and flow coordinates from the opening interaction', () => {
    const contextMenu = useCanvasNodeContextMenu()

    contextMenu.open({ x: 32, y: 44, flowX: 142, flowY: 264 })

    expect(contextMenu.nodeContextMenu.value).toEqual({
      isVisible: true,
      x: 32,
      y: 44,
      flowX: 142,
      flowY: 264,
    })
  })

  it('creates through the captured flow intent and closes its surface', () => {
    const contextMenu = useCanvasNodeContextMenu()
    const createNodeAtPosition = vi.fn()
    contextMenu.open({ x: 32, y: 44, flowX: 142, flowY: 264 })

    contextMenu.createNode('prompt', createNodeAtPosition)

    expect(createNodeAtPosition).toHaveBeenCalledWith('prompt', 142, 264)
    expect(contextMenu.nodeContextMenu.value.isVisible).toBe(false)
  })

  it('closes without discarding the last creation snapshot', () => {
    const contextMenu = useCanvasNodeContextMenu()
    contextMenu.open({ x: 32, y: 44, flowX: 142, flowY: 264 })

    contextMenu.close()

    expect(contextMenu.nodeContextMenu.value).toEqual({
      isVisible: false,
      x: 32,
      y: 44,
      flowX: 142,
      flowY: 264,
    })
  })
})
